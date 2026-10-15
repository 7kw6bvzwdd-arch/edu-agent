package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestCoursewareThreeSlideGateHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "other")
	var input map[string]any
	json.Unmarshal(asJSON(TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "classroom", UseKB: true, IdempotencyKey: newID()}), &input)
	input["preview_first"] = true
	input["requirements"] = "45分钟，实验探究优先，基础弱，练习由浅入深"
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, input, 202))["id"].(string)
	route := "/api/tasks/" + id
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	task := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if task["input"].(map[string]any)["requirements"] != input["requirements"] {
		t.Fatal("confirmed requirements lost before preview")
	}
	if task["status"] != "awaiting_preview" {
		t.Fatal("full generation occurred before preview approval", task["status"], task["error"])
	}
	slides := task["result"].(map[string]any)["slides"].([]any)
	if len(slides) != 3 {
		t.Fatal("preview must contain exactly three slides")
	}
	previewFile := worksCall(t, s, "GET", route+"/preview-pptx", token, nil, 200).Body.Bytes()
	if len(previewFile) < 1000 || string(previewFile[:2]) != "PK" {
		t.Fatal("preview isn't a real PPTX")
	}
	worksCall(t, s, "GET", route+"/preview-pptx", other, nil, 404)
	metaFirst := task["slide_reviews"].([]any)[0].(map[string]any)
	if os.Getenv("TEST_REQUIRE_OFFICE_PREVIEW") == "1" && task["preview_images_available"] != true {
		t.Fatal("Office preview required but backend snapshot unavailable")
	}
	if task["preview_images_available"] == true {
		png := worksCall(t, s, "GET", route+"/slides/"+metaFirst["id"].(string)+"/preview?revision="+fmt.Sprint(task["revision"]), token, nil, 200).Body.Bytes()
		if len(png) < 1000 || string(png[1:4]) != "PNG" {
			t.Fatal("actual preview missing")
		}
		worksCall(t, s, "GET", route+"/slides/"+metaFirst["id"].(string)+"/preview?revision="+fmt.Sprint(task["revision"]), other, nil, 404)
	}
	worksCall(t, s, "POST", route+"/expand", other, map[string]any{"revision": task["revision"]}, 404)
	worksCall(t, s, "POST", route+"/expand", token, map[string]any{"revision": task["revision"]}, 409)
	before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))["balance"]
	reviews := task["slide_reviews"].([]any)
	for i, entry := range reviews {
		meta := entry.(map[string]any)
		worksCall(t, s, "POST", route+"/slides/"+meta["id"].(string)+"/review", token, map[string]any{"revision": task["revision"], "hash": meta["hash"], "approved": true}, 200)
		if i == 0 {
			worksCall(t, s, "POST", route+"/slides/"+meta["id"].(string)+"/feedback", token, map[string]any{"revision": task["revision"], "hash": meta["hash"], "vote": -1, "reason": "说明不够清楚"}, 200)
			worksCall(t, s, "POST", route+"/expand", token, map[string]any{"revision": task["revision"]}, 409)
			worksCall(t, s, "POST", route+"/slides/"+meta["id"].(string)+"/feedback", token, map[string]any{"revision": task["revision"], "hash": meta["hash"], "vote": 0}, 200)
			worksCall(t, s, "POST", route+"/slides/"+meta["id"].(string)+"/review", token, map[string]any{"revision": task["revision"], "hash": meta["hash"], "approved": true}, 200)
		}
	}
	worksCall(t, s, "POST", route+"/expand", token, map[string]any{"revision": task["revision"]}, 202)
	worksCall(t, s, "POST", route+"/expand", token, map[string]any{"revision": task["revision"]}, 202)
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	full := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if full["status"] != "awaiting_confirmation" {
		t.Fatal("expansion failed", full["error"])
	}
	all := full["result"].(map[string]any)["slides"].([]any)
	if len(all) < 8 || !reflect.DeepEqual(slides, all[:3]) {
		t.Fatal("approved preview slides were overwritten")
	}
	worksCall(t, s, "POST", route+"/confirm", token, map[string]any{"revision": full["revision"], "result": full["result"], "accept_review": true}, 409)
	for _, entry := range full["slide_reviews"].([]any) {
		meta := entry.(map[string]any)
		worksCall(t, s, "POST", route+"/slides/"+meta["id"].(string)+"/review", token, map[string]any{"revision": full["revision"], "hash": meta["hash"], "approved": true}, 200)
	}
	worksCall(t, s, "POST", route+"/confirm", token, map[string]any{"revision": full["revision"], "result": full["result"], "accept_review": true}, 202)
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	pptx := worksCall(t, s, "GET", route+"/download/pptx", token, nil, 200).Body.Bytes()
	if len(pptx) < 1000 {
		t.Fatal("no real artifact")
	}
	after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))["balance"].(float64)
	if after != before.(float64)-20 {
		t.Fatal("preview/expansion charged more than one generation")
	}
	// Delivered single-page AI edits must preserve stable identity and other pages.
	done := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	originalFull := done["result"].(map[string]any)["slides"].([]any)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sl := sampleDeck(TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "classroom"}).Slides[0]
		sl.Notes += "\n已交付后的单页修订。"
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(asJSON(sl))}}}})
	}))
	defer provider.Close()
	s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "test", "test"
	tx, err := s.tx(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), "UPDATE tasks SET mode='live' WHERE id=$1", id)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	rewrite := worksObject(t, worksCall(t, s, "POST", route+"/conversation", token, map[string]any{"revision": done["revision"], "mode": "revise", "content": "补充观察问题", "slide_id": originalFull[0].(map[string]any)["id"], "idempotency_key": newID()}, 202))
	worksCall(t, s, "POST", route+"/conversation", token, map[string]any{"revision": done["revision"], "mode": "discuss", "content": "同时讨论", "idempotency_key": newID()}, 409)
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	child := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+rewrite["id"].(string), token, nil, 200))
	if child["status"] != "succeeded" {
		t.Fatal("delivered rewrite failed", child["error"])
	}
	edited := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))["result"].(map[string]any)["slides"].([]any)
	if edited[0].(map[string]any)["id"] != originalFull[0].(map[string]any)["id"] || !reflect.DeepEqual(edited[1:], originalFull[1:]) {
		t.Fatal("delivered single-page rewrite lost identity or changed other pages")
	}
}

func TestCoursewareConversationAndFeedbackHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "other")
	calls := 0
	revisionCall := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request struct{ Messages []struct{ Content string } }
		json.NewDecoder(r.Body).Decode(&request)
		prompt := request.Messages[len(request.Messages)-1].Content
		if strings.Contains(prompt, "你是课件协作助手") {
			if strings.Contains(prompt, "模式：revise") {
				revisionCall++
				if !strings.Contains(prompt, "说明不够具体") {
					t.Error("downvote reason was not passed to revision")
				}
				d := sampleDeck(TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "classroom"})
				sl := d.Slides[0]
				sl.Bullets = []string{"从木块浮在水面引入浮力", "区分浮力方向与浮沉状态"}
				sl.Notes += "\n先观察木块，再区分运动状态与受力。"
				reply := string(asJSON(map[string]any{"reply": "已更新当前页，补充具体观察问题；其他页不变。", "slide": sl}))
				respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": reply}}}})
			} else {
				respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"reply":"先区分浮力方向与物体浮沉，课堂需要核对条件。"}`}}}})
			}
			return
		}
		d := sampleDeck(TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "classroom"})
		m := regexp.MustCompile(`本批只返回第(\d+)至第(\d+)页`).FindString(prompt)
		var start, end int
		fmt.Sscanf(m, "本批只返回第%d至第%d页", &start, &end)
		if start < 1 || end > 3 || start != end {
			t.Error("invalid preview range")
			w.WriteHeader(400)
			return
		}
		d.Slides = d.Slides[start-1 : end]
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(asJSON(d))}}}})
	}))
	defer provider.Close()
	s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "test", "test"
	in := map[string]any{"type": "courseware", "topic": "浮力", "pages": 8, "theme": "classroom", "use_kb": true, "preview_first": true, "idempotency_key": newID()}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	route := "/api/tasks/" + id
	t.Cleanup(func() { worksCall(t, s, "POST", route+"/cancel", token, nil, 200) })
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	task := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	original := task["result"].(map[string]any)["slides"].([]any)
	m := task["slide_reviews"].([]any)[0].(map[string]any)
	slideID := m["id"].(string)
	before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))["balance"]
	worksCall(t, s, "POST", route+"/slides/"+slideID+"/feedback", token, map[string]any{"revision": task["revision"], "hash": m["hash"], "vote": -1, "reason": "说明不够具体"}, 200)
	worksCall(t, s, "POST", route+"/slides/"+slideID+"/review", token, map[string]any{"revision": task["revision"], "hash": m["hash"], "approved": true}, 409)
	key := newID()
	request := map[string]any{"mode": "revise", "content": "加入具体的观察问题", "slide_id": slideID, "revision": task["revision"], "idempotency_key": key}
	worksCall(t, s, "POST", route+"/conversation", other, request, 404)
	response := worksObject(t, worksCall(t, s, "POST", route+"/conversation", token, request, 200))
	worksCall(t, s, "POST", route+"/conversation", token, request, 202)
	if calls != 4 || revisionCall != 1 {
		t.Fatal("chat idempotency called model twice", calls)
	}
	updated := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	newSlides := updated["result"].(map[string]any)["slides"].([]any)
	if !reflect.DeepEqual(original[1:], newSlides[1:]) || newSlides[0].(map[string]any)["id"] != slideID {
		t.Fatal("single page chat changed other pages or identity")
	}
	worksCall(t, s, "POST", route+"/slides/"+slideID+"/review", token, map[string]any{"revision": task["revision"], "hash": m["hash"], "approved": true}, 409)
	worksCall(t, s, "POST", route+"/conversation/"+response["id"].(string)+"/feedback", token, map[string]any{"revision": updated["revision"], "vote": -1, "reason": "解释还需更明确"}, 200)
	worksCall(t, s, "GET", route+"/conversation", other, nil, 404)
	rows := worksCall(t, s, "GET", route+"/conversation", token, nil, 200).Body.Bytes()
	if !strings.Contains(string(rows), "解释还需更明确") {
		t.Fatal("message feedback not persisted")
	}
	after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))["balance"]
	if before != after {
		t.Fatal("draft chat charged additional business credits")
	}
	// Refresh must unlock an abandoned synchronous request, not a legitimate child job.
	ctx := context.Background()
	tx, err := s.tx(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, "INSERT INTO courseware_messages(id,tenant_id,task_id,request_key,revision,mode,content,created_at) VALUES($1,1,$2,$3,1,'discuss','interrupted',now()-interval '5 minutes')", newID(), id, newID())
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var messages []map[string]any
	json.Unmarshal(worksCall(t, s, "GET", route+"/conversation", token, nil, 200).Body.Bytes(), &messages)
	for _, m := range messages {
		if m["content"] == "interrupted" && m["status"] != "failed" {
			t.Fatal("refresh leaves abandoned chat locked")
		}
	}
}

func TestCoursewarePreviewModelDoesNotGenerateFullDeck(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request struct{ Messages []struct{ Content string } }
		json.NewDecoder(r.Body).Decode(&request)
		prompt := request.Messages[len(request.Messages)-1].Content
		if !strings.Contains(prompt, "只生成前三页") {
			t.Error("initial prompt requested whole deck")
		}
		d := sampleDeck(TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "classroom"})
		m := regexp.MustCompile(`本批只返回第(\d+)至第(\d+)页`).FindString(prompt)
		var start, end int
		fmt.Sscanf(m, "本批只返回第%d至第%d页", &start, &end)
		if start < 1 || end > 3 || start != end {
			t.Error("invalid preview range")
			w.WriteHeader(400)
			return
		}
		d.Slides = d.Slides[start-1 : end]
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(asJSON(d))}}}})
	}))
	defer provider.Close()
	s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "test", "test"
	body := map[string]any{"type": "courseware", "topic": "浮力", "pages": 8, "theme": "classroom", "use_kb": true, "preview_first": true, "idempotency_key": newID()}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, body, 202))["id"].(string)
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	task := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
	if task["status"] != "awaiting_preview" || calls != 3 {
		t.Fatal("three page model preview was rejected", task["error"], calls)
	}
	var actor Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &actor)
	if err := s.finish(context.Background(), actor.TenantID, id, "failed", "模拟后续服务中断", "", nil); err != nil {
		t.Fatal(err)
	}
	retry := worksObject(t, worksCall(t, s, "POST", "/api/tasks/"+id+"/retry", token, nil, 202))["id"].(string)
	recovered := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+retry, token, nil, 200))
	if recovered["status"] != "awaiting_preview" || !reflect.DeepEqual(task["result"].(map[string]any)["slides"], recovered["result"].(map[string]any)["slides"]) || calls != 3 {
		t.Fatal("retry failed to preserve actual three-page preview without a new model call")
	}
	pptx := worksCall(t, s, "GET", "/api/tasks/"+retry+"/preview-pptx", token, nil, 200).Body.Bytes()
	if len(pptx) < 1000 || string(pptx[:2]) != "PK" {
		t.Fatal("recovered preview missing real artifact")
	}
	worksCall(t, s, "POST", "/api/tasks/"+retry+"/cancel", token, nil, 200)
}

func TestCoursewareConversationCancellationHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	input := TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "classroom", UseKB: true, PreviewFirst: true, IdempotencyKey: newID()}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, input, 202))["id"].(string)
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	route := "/api/tasks/" + id
	task := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	started, release := make(chan struct{}), make(chan struct{})
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		sl := sampleDeck(input).Slides[0]
		sl.Notes += "\n迟到的修改"
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(asJSON(map[string]any{"reply": "更新完成", "slide": sl}))}}}})
	}))
	defer provider.Close()
	s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "test", "test"
	tx, err := s.tx(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(context.Background(), "UPDATE tasks SET mode='live' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"revision": task["revision"], "mode": "revise", "slide_id": task["slide_reviews"].([]any)[0].(map[string]any)["id"], "content": "等待期间取消", "idempotency_key": newID()}
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		r := httptest.NewRequest("POST", route+"/conversation", strings.NewReader(string(asJSON(body))))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		result <- w
	}()
	<-started
	worksCall(t, s, "POST", route+"/cancel", token, nil, 200)
	close(release)
	w := <-result
	if w.Code != 409 {
		t.Fatal("late conversation overwrote cancelled task", w.Code, w.Body.String())
	}
	after := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if after["status"] != "cancelled" || after["charge_state"] != "released" || !reflect.DeepEqual(after["result"], task["result"]) {
		t.Fatal("cancellation failed to preserve original preview and release quota")
	}
}
