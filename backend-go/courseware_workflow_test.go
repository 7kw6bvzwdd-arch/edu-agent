package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
)

// Advance only the persisted queue delay; never invoke the user's retry route.
func TestWorkflowBlockingReviewPreservesDraftHTTP(t *testing.T) {
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
	generated := map[int]int{}
	var generatedMu sync.Mutex
	var outage atomic.Bool
	outage.Store(true)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model    string
			Messages []struct{ Content any }
		}
		json.NewDecoder(r.Body).Decode(&req)
		raw := ""
		if req.Model == "generation" {
			var page, end int
			fmt.Sscanf(regexp.MustCompile(`本批只返回第\d+至第\d+页`).FindString(req.Messages[len(req.Messages)-1].Content.(string)), "本批只返回第%d至第%d页", &page, &end)
			if page < 1 || page > in.Pages || page != end {
				t.Error("nonlocal generation")
				w.WriteHeader(400)
				return
			}
			generatedMu.Lock()
			generated[page]++
			generatedMu.Unlock()
			d := directHTTPFixtureDeck(sampleDeck(in))
			d.Slides = d.Slides[page-1 : page]
			raw = string(asJSON(d))
		} else {
			if outage.Load() && req.Model == "vision" {
				raw = `{"issues":[{"object":"正文","category":"layout","certainty":"confirmed","location":"右侧正文","observed":"最后一行被遮挡","expected":"完整显示正文","evidence":"可见裁切","severity":"error","repair":"调整正文位置"}]}`
			} else {
				raw = `{"issues":[]}`
			}
		}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": raw}}}})
	}))
	defer provider.Close()
	renderer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			respond(w, 200, map[string]any{"courseware_media_version": "media-v1", "courseware_graph_version": "graph-v1", "courseware_authored_html_version": "authored-html-v2", "isolated_browser_author_code": true})
			return
		}
		w.Write([]byte("workflow-scheduling-pptx"))
	}))
	defer renderer.Close()
	s.cfg.RenderURL = renderer.URL
	ctx := withCoursewareArtifacts(context.Background())
	artifactCachePut(ctx, fmt.Sprintf("office:%x:0", sha256.Sum256([]byte("workflow-scheduling-pptx"))), asJSON(map[string][]byte{"slide-1.png": []byte("synthetic-scheduling-screenshot")}))
	s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "fixture", "generation"
	// Isolate orchestration with the deployed review configuration: content
	// model review is paused, deterministic checks and screenshot review stay on.
	s.cfg.CoursewareContentReviewDisabled = true
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
	if err = s.workOne(ctx); err != nil {
		t.Fatal(err)
	}
	v := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	var saved Deck
	json.Unmarshal(asJSON(v["result"]), &saved)
	if len(saved.Slides) > 0 && saved.Slides[0].HTML == "" {
		t.Fatal("HTTP workflow discarded model-authored source")
	}
	if v["status"] != "pending" || v["charge_state"] != "reserved" || len(saved.Slides) != 1 {
		t.Fatalf("blocking review discarded rendered draft: status=%v saved=%d generated=%v", v["status"], len(saved.Slides), generated)
	}
	if s.pageCheckPassed(saved.PageChecks[saved.Slides[0].ID], false) {
		t.Fatal("failed visual review was accepted")
	}
	worksCall(t, s, "GET", route, worksLogin(t, s, "other"), nil, 404)
	for round := 0; round < 8 && v["status"] != "failed"; round++ {
		makeAutomaticRetryReady(t, s, id)
		if err = s.workOne(ctx); err != nil {
			t.Fatal(err)
		}
		v = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	}
	if v["status"] != "failed" || v["charge_state"] != "released" {
		t.Fatalf("page repair resets forever: status=%v generated=%v", v["status"], generated)
	}
	generatedMu.Lock()
	counts := map[int]int{}
	for page, calls := range generated {
		counts[page] = calls
	}
	generatedMu.Unlock()
	first, next := plannedSchedule(plan)[0].Position, plannedSchedule(plan)[1].Position
	if counts[first] < 1 || counts[first] > 6 {
		t.Fatalf("persistent page repair allowance was reset: %v", counts)
	}
	// Content approval may prefetch the next page before vision fails. Only
	// the initial append round can speculate; subsequent repair rounds cannot.
	for page, calls := range counts {
		if page != first && (page != next || calls > 2) {
			t.Fatalf("unexpected work beyond bounded next-page prefetch: %v", counts)
		}
	}
	json.Unmarshal(asJSON(v["result"]), &saved)
	if len(saved.Slides) != 1 {
		t.Fatal("final failure lost draft")
	}
	t.Logf("bounded visual failure: generated_by_position=%v saved=%d status=%v charge=%v", counts, len(saved.Slides), v["status"], v["charge_state"])
	outage.Store(false)
	worksCall(t, s, "POST", route+"/retry", token, nil, 202)
	if err = s.workOne(ctx); err != nil {
		t.Fatal(err)
	}
	v = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if v["status"] != "awaiting_preview" || v["charge_state"] != "reserved" {
		t.Fatalf("explicit continuation cannot recover saved draft: %v %v", v["status"], v["error"])
	}
}
