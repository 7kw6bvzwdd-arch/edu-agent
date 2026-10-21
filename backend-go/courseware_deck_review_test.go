package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// External reviewer replies are fixtures; requests, isolation, saved drafts,
// confirmation, billing and actual PPTX use the normal HTTP/task pipeline.
func TestCoursewareWholeDeckReviewHTTP(t *testing.T) {
	s := worksServer(t)
	token, other := worksLogin(t, s, "teacher"), worksLogin(t, s, "other")
	in := TaskInput{Type: "courseware", Topic: "浮力", Theme: "classroom", Pages: 8, Detail: "auto", UseKB: true, IdempotencyKey: newID()}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	route := "/api/tasks/" + id
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	d := sampleDeck(in)
	assignSlideIDs(&d)
	d.Plan = &TeachingPlan{Summary: "固定模型接口样例", LearningGoals: []LearningGoal{{ID: "g1", Text: "解释浮力并解决给定问题", Core: true}}}
	d.PageChecks = map[string]PageQualityCheck{}
	for i := range d.Slides {
		d.Slides[i].Order = i + 1
		row := TeachingPage{Position: i + 1, Goal: d.Slides[i].Title, Task: d.Slides[i].Layout, Minutes: 3, Preview: i < 3, GoalRefs: []string{"g1"}}
		if d.Slides[i].Layout == "exercise" {
			row.ChecksGoals = []string{"g1"}
		}
		d.Plan.Pages = append(d.Plan.Pages, row)
		d.PageChecks[d.Slides[i].ID] = PageQualityCheck{Hash: slideContentHash(d, d.Slides[i]), Content: "checked", Visual: "checked"}
	}
	var variant, calls atomic.Int32
	reviewer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model    string
			Messages []struct{ Content string }
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Model == "generation-fixture" {
			sl := directHTTPFixtureDeck(d).Slides[1]
			sl.Notes += "\n固定改写样例，新增需要依据的说明。"
			respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(asJSON(sl))}}}})
			return
		}
		if !strings.Contains(req.Messages[len(req.Messages)-1].Content, "全课事实与跨页一致性") {
			respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": `{"issues":[]}`}}}})
			return
		}
		calls.Add(1)
		if req.Model != "independent-fixture" || r.Header.Get("Authorization") != "Bearer reviewer-fixture" || !strings.Contains(req.Messages[len(req.Messages)-1].Content, "全课事实与跨页一致性") {
			t.Error("whole deck request not routed to independent reviewer")
		}
		pages := []int{}
		for i := range d.Slides {
			pages = append(pages, i+1)
		}
		reply := map[string]any{"summary": "固定审核样例", "pages_checked": pages, "goals_checked": []string{"g1"}, "issues": []any{}}
		if variant.Load() == 0 {
			reply["issues"] = []any{map[string]any{"pages": []int{2, 4}, "object": "史料中的年代", "evidence": "当前来源未提供该年代依据（固定缺证样例）", "severity": "missing_evidence", "repair": "补充可核对的来源或删除具体年代", "source_ids": []string{}}}
		}
		if variant.Load() == 2 {
			reply["pages_checked"] = []int{1}
		}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(asJSON(reply))}}}})
	}))
	defer reviewer.Close()
	s.cfg.Mode, s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = "live", reviewer.URL, "reviewer-fixture", "independent-fixture"
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), "UPDATE tasks SET status='awaiting_confirmation',stage='awaiting_confirmation',mode='live',result=$1 WHERE id=$2", asJSON(d), id)
	if err == nil {
		err = tx.Commit(context.Background())
	} else {
		tx.Rollback(context.Background())
	}
	if err != nil {
		t.Fatal(err)
	}
	task := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	bill := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	save := func(result any) {
		body := map[string]any{"revision": task["revision"], "result": result}
		worksCall(t, s, "POST", route+"/draft", "", body, 401)
		worksCall(t, s, "POST", route+"/draft", other, body, 404)
		worksCall(t, s, "POST", route+"/draft", token, body, 200)
		worksCall(t, s, "POST", route+"/draft", token, body, 409)
		task = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	}
	save(task["result"])
	result := task["result"].(map[string]any)
	if result["deck_check"].(map[string]any)["status"] != "blocked" {
		t.Fatal("missing evidence passed whole-deck gate")
	}
	for _, entry := range task["slide_reviews"].([]any) {
		m := entry.(map[string]any)
		worksCall(t, s, "POST", route+"/slides/"+m["id"].(string)+"/review", token, map[string]any{"revision": task["revision"], "hash": m["hash"], "approved": true}, 200)
	}
	// Forging a success in the payload cannot replace the server's blocked check.
	result["deck_check"].(map[string]any)["status"] = "checked"
	worksCall(t, s, "POST", route+"/confirm", token, map[string]any{"revision": task["revision"], "result": result, "accept_review": true}, 409)
	if calls.Load() != 1 {
		t.Fatal("confirmation unexpectedly called reviewer")
	}
	variant.Store(2)
	result["lesson"] = result["lesson"].(string) + "\n教师补充待核验说明。"
	save(result)
	result = task["result"].(map[string]any)
	if result["deck_check"].(map[string]any)["status"] != "unavailable" || calls.Load() != 3 {
		t.Fatal("incomplete reviewer coverage passed or exceeded retry bound")
	}
	save(result)
	if calls.Load() != 3 {
		t.Fatal("unchanged failed content reset persisted review budget")
	}
	variant.Store(1)
	// An optimistic model verdict cannot override an impossible visible task.
	s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = reviewer.URL, "vision-fixture", "vision-fixture"
	var validResult map[string]any
	json.Unmarshal(asJSON(task["result"]), &validResult)
	var invalidResult map[string]any
	json.Unmarshal(asJSON(validResult), &invalidResult)
	invalidSlide := invalidResult["slides"].([]any)[1].(map[string]any)
	for _, key := range []string{"graph", "chart", "table", "figure", "picture", "diagram", "image_need", "image_search", "canvas_objects"} {
		delete(invalidSlide, key)
	}
	invalidSlide["scientific"] = ScientificDiagram{Kind: "cell", Title: "植物细胞示意", CellType: "plant", VisibleParts: []string{"cell_wall", "cell_membrane", "cytoplasm", "nucleus"}}
	invalidSlide["focus"] = "比较左图和右图，指出两图结构差异。"
	save(invalidResult)
	invalidResult = task["result"].(map[string]any)
	if invalidResult["deck_check"].(map[string]any)["status"] != "blocked" || calls.Load() != 3 {
		t.Fatal("impossible visual task was accepted or sent for unnecessary model review")
	}
	worksCall(t, s, "POST", route+"/confirm", token, map[string]any{"revision": task["revision"], "result": invalidResult, "accept_review": true}, 409)
	// Restoring a usable task can still pass the independent whole-deck review.
	result = validResult
	result["lesson"] = result["lesson"].(string) + "\n教师删除没有依据的具体年代，保留课堂问题。"
	save(result)
	result = task["result"].(map[string]any)
	if result["deck_check"].(map[string]any)["status"] != "checked" {
		t.Fatal("corrected content cannot recover")
	}
	for _, entry := range task["slide_reviews"].([]any) {
		m := entry.(map[string]any)
		worksCall(t, s, "POST", route+"/slides/"+m["id"].(string)+"/review", token, map[string]any{"revision": task["revision"], "hash": m["hash"], "approved": true}, 200)
	}
	after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	if after["balance"] != bill["balance"] || after["reserved"] != bill["reserved"] {
		t.Fatal("draft audits changed business credits")
	}
	worksCall(t, s, "POST", route+"/confirm", other, map[string]any{"revision": task["revision"], "result": result, "accept_review": true}, 404)
	worksCall(t, s, "POST", route+"/confirm", token, map[string]any{"revision": task["revision"], "result": result, "accept_review": true}, 202)
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	task = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if task["status"] != "succeeded" || calls.Load() != 4 {
		t.Fatalf("delivery failed or repeated successful audit: %v, %d calls", task["error"], calls.Load())
	}
	raw := worksCall(t, s, "GET", route+"/download/pptx", token, nil, 200).Body.Bytes()
	if len(raw) < 1000 || string(raw[:2]) != "PK" {
		t.Fatal("missing real PPTX")
	}
	after = worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	if after["balance"].(float64) != bill["balance"].(float64)-20 {
		t.Fatal("delivery did not settle exactly once")
	}
	settledBill := after
	// A free edit must not replace a delivered file with a blocked revision.
	variant.Store(0)
	result = task["result"].(map[string]any)
	result["lesson"] = result["lesson"].(string) + "\n新增需要依据的具体年代。"
	worksCall(t, s, "POST", route+"/edit", token, map[string]any{"revision": task["revision"], "result": result}, 409)
	unchanged := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if unchanged["revision"] != task["revision"] {
		t.Fatal("blocked edit replaced the delivered revision")
	}
	if !bytes.Equal(raw, worksCall(t, s, "GET", route+"/download/pptx", token, nil, 200).Body.Bytes()) {
		t.Fatal("blocked edit replaced the delivered PPTX")
	}
	// AI rewriting must recheck the changed full lesson before replacing it or charging.
	s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = reviewer.URL, "generation-fixture", "generation-fixture"
	s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = reviewer.URL, "vision-fixture", "vision-fixture"
	rewrite := worksObject(t, worksCall(t, s, "POST", route+"/rewrite", token, map[string]any{"revision": task["revision"], "slide": 1, "instruction": "补充课堂说明", "idempotency_key": newID()}, 202))["id"].(string)
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	child := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+rewrite, token, nil, 200))
	if child["status"] != "failed" || child["charge_state"] != "released" {
		t.Fatalf("blocked AI rewrite was delivered or charged: %v, %v", child["status"], child["charge_state"])
	}
	unchanged = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if unchanged["revision"] != task["revision"] || calls.Load() != 6 {
		t.Fatalf("AI rewrite skipped full-deck audit or replaced parent: %d calls", calls.Load())
	}
	after = worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	if after["balance"] != settledBill["balance"] || after["reserved"] != settledBill["reserved"] {
		t.Fatal("blocked rewrite changed settled balance or retained reservation")
	}
	variant.Store(1)
	rewrite = worksObject(t, worksCall(t, s, "POST", route+"/rewrite", token, map[string]any{"revision": task["revision"], "slide": 1, "instruction": "保留依据并补充课堂说明", "idempotency_key": newID()}, 202))["id"].(string)
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	child = worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+rewrite, token, nil, 200))
	if child["status"] != "succeeded" || child["charge_state"] != "settled" {
		t.Fatalf("audited rewrite cannot deliver: %v, %v", child["status"], child["error"])
	}
	task = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if task["result"].(map[string]any)["deck_check"].(map[string]any)["status"] != "checked" || calls.Load() != 7 {
		t.Fatal("successful rewrite lacks current whole-deck audit")
	}
	worksCall(t, s, "GET", route+"/download/pptx", token, nil, 200)
	after = worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	if after["balance"] != settledBill["balance"].(float64)-5 || after["reserved"] != settledBill["reserved"] {
		t.Fatal("successful rewrite did not settle exactly five credits")
	}
}
