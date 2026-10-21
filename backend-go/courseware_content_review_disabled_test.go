package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestPausedContentReviewResumesSavedPageHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	in := TaskInput{Type: "courseware", Topic: "浮力", Subject: "物理", Detail: "concise", LessonMinutes: 45, Theme: "ocean", UseKB: true, PreviewFirst: true, IdempotencyKey: newID()}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	route := "/api/tasks/" + id
	defer worksCall(t, s, "POST", route+"/cancel", token, nil, 200)
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	plan, err := s.planCourseware(context.Background(), a, id, in, nil)
	if err != nil {
		t.Fatal(err)
	}
	in.Pages = len(plan.Pages)
	generated, reviewed, contentCalls := 0, 0, 0
	outage := true
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model    string
			Messages []struct{ Content any }
		}
		json.NewDecoder(r.Body).Decode(&req)
		raw := ""
		if req.Model == "generation" {
			generated++
			var page, end int
			fmt.Sscanf(regexp.MustCompile(`本批只返回第\d+至第\d+页`).FindString(req.Messages[len(req.Messages)-1].Content.(string)), "本批只返回第%d至第%d页", &page, &end)
			if page < 1 || page > in.Pages || page != end {
				t.Error("nonlocal generation")
				w.WriteHeader(400)
				return
			}
			d := directHTTPFixtureDeck(sampleDeck(in))
			d.Slides = d.Slides[page-1 : page]
			raw = string(asJSON(d))
		} else {
			reviewed++
			if req.Model == "review" {
				contentCalls++
			}
			if outage && req.Model == "review" {
				raw = `{}`
			} else {
				raw = `{"issues":[]}`
				// Simulated successful review contracts; content accuracy is tested
				// separately by the explicitly enabled real-model corpus.
				if prompt, ok := req.Messages[len(req.Messages)-1].Content.(string); ok {
					if strings.Contains(prompt, "你是独立解题员") {
						raw = `{"solution":"测试模拟：已根据学生可见条件完成本页题目，保留条件、计算和结论供后续核对。"}`
					} else if strings.Contains(prompt, "你是教学风险复核员") {
						checks := []map[string]any{}
						for i := 0; i < strings.Count(prompt, "\n复核项"); i++ {
							checks = append(checks, map[string]any{"index": i + 1, "evidence": "测试模拟：已独立检查本项具体教学条件及计算说明，当前对象没有模拟错误。", "issues": []PageQualityIssue{}})
						}
						raw = string(asJSON(map[string]any{"checks": checks}))
					}
				}
			}
		}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": raw}}}})
	}))
	defer provider.Close()
	s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "fixture", "generation"
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = provider.URL, "fixture", "review"
	s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = provider.URL, "fixture", "vision"
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), "UPDATE tasks SET mode='live',result=$1 WHERE id=$2", asJSON(Deck{Plan: plan, Generation: &CoursewareCheckpoint{Phase: "preview"}}), id)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	v := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	var saved Deck
	json.Unmarshal(asJSON(v["result"]), &saved)
	if v["status"] != "pending" || v["charge_state"] != "reserved" || len(saved.Slides) != 1 || generated != 1 {
		t.Fatal("review outage requires click or loses generated page", v["status"], v["error"], generated, v["events"])
	}
	hash := slideContentHash(saved, saved.Slides[0])
	if pageCheckPassed(saved.PageChecks[saved.Slides[0].ID], false) {
		t.Fatal("unavailable review passed")
	}
	s.cfg.CoursewareContentReviewDisabled = true
	beforeContentCalls := contentCalls
	makeAutomaticRetryReady(t, s, id)
	if err = s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	v = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	for round := 0; round < 8 && v["status"] == "pending"; round++ {
		makeAutomaticRetryReady(t, s, id)
		if err = s.workOne(context.Background()); err != nil {
			t.Fatal(err)
		}
		v = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	}
	var done Deck
	json.Unmarshal(asJSON(v["result"]), &done)
	if v["status"] != "awaiting_preview" || generated != 3 || hash != slideContentHash(done, done.Slides[0]) {
		t.Fatal("review recovery regenerated saved page or failed teacher preview", v["status"], v["error"], generated, reviewed, v["events"])
	}
	if contentCalls != beforeContentCalls {
		t.Fatalf("disabled content reviewer was called: %d -> %d", beforeContentCalls, contentCalls)
	}
	for _, sl := range done.Slides {
		check := done.PageChecks[sl.ID]
		if check.Content != "disabled" || check.Visual != "checked" {
			t.Fatalf("disabled reviewer must be explicit and visual must still pass: %+v", check)
		}
	}
	if !strings.Contains(done.Notice, "内容模型审核已停用") {
		t.Fatal("teacher must see content reviewer disabled")
	}
	report := s.assessTeaching(context.Background(), a, id, "courseware", done, nil)
	if report["status"] != "unavailable" || !strings.Contains(fmt.Sprint(report["reason"]), "全课") {
		t.Fatal("disabled review presented as fully reviewed", report)
	}
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = "", "", ""
	health := worksObject(t, worksCall(t, s, "GET", "/api/health", "", nil, 200))
	if health["courseware_content_review_enabled"] != false || health["courseware_quality_configured"] != true {
		t.Fatal("disabled mode still blocks generation", health)
	}
	newInput := in
	newInput.IdempotencyKey = newID()
	freshID := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, newInput, 202))["id"].(string)
	worksCall(t, s, "POST", "/api/tasks/"+freshID+"/cancel", token, nil, 200)

	s.cfg.CoursewareContentReviewDisabled = false
	meta := v["slide_reviews"].([]any)[0].(map[string]any)
	worksCall(t, s, "POST", route+"/slides/"+meta["id"].(string)+"/review", token, map[string]any{"revision": v["revision"], "hash": meta["hash"], "approved": true}, 409)
	// Rechecking an unchanged legacy draft uses the existing save path and must
	// neither regenerate teaching content nor approve or charge the teacher.
	outage = false
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = provider.URL, "fixture", "review"
	beforeBilling := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	beforeGenerated, beforeContent := generated, contentCalls
	other := worksLogin(t, s, "other")
	request := map[string]any{"revision": v["revision"], "result": v["result"]}
	worksCall(t, s, "POST", route+"/draft", other, request, 404)
	worksCall(t, s, "POST", route+"/draft", token, request, 200)
	v = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	var rechecked Deck
	json.Unmarshal(asJSON(v["result"]), &rechecked)
	if v["status"] != "awaiting_preview" || generated != beforeGenerated || contentCalls <= beforeContent {
		t.Fatal("unchanged draft failed to rerun enabled content checks without regeneration", v["status"], generated, contentCalls)
	}
	for i, sl := range rechecked.Slides {
		if slideContentHash(rechecked, sl) != slideContentHash(done, done.Slides[i]) || !s.pageCheckPassed(rechecked.PageChecks[sl.ID], false, sl) {
			t.Fatal("recheck changed teaching content or left a disabled check", sl.ID, rechecked.PageChecks[sl.ID])
		}
	}
	for _, row := range v["slide_reviews"].([]any) {
		if row.(map[string]any)["approved"] == true {
			t.Fatal("recheck cannot approve on behalf of the teacher")
		}
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != beforeBilling {
		t.Fatal("recheck changed teacher points")
	}
	meta = v["slide_reviews"].([]any)[0].(map[string]any)
	s.cfg.CoursewareContentReviewDisabled = true
	worksCall(t, s, "POST", route+"/slides/"+meta["id"].(string)+"/review", other, map[string]any{"revision": v["revision"], "hash": meta["hash"], "approved": true}, 404)
	worksCall(t, s, "POST", route+"/slides/"+meta["id"].(string)+"/review", token, map[string]any{"revision": v["revision"], "hash": meta["hash"], "approved": true}, 200)

}
