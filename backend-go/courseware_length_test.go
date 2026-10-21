package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestCoursewareBatchedLengthRecoveryHTTP(t *testing.T) {
	for _, mode := range []string{"12-pages", "20-pages", "repair-truncated", "quality-repair-truncated", "split-to-single", "persistent-truncated", "permanent-http", "cancel-between-batches"} {
		t.Run(mode, func(t *testing.T) {
			s := worksServer(t)
			token := worksLogin(t, s, "teacher")
			pages := 12
			if mode == "20-pages" {
				pages = 20
			}
			in := TaskInput{Type: "courseware", Topic: "浮力", Pages: pages, Theme: "classroom", UseKB: true, PreviewFirst: true, IdempotencyKey: newID()}
			before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))["balance"]
			calls := 0
			active := false
			route := ""
			attempts := map[int]int{}
			firstBudget := 0
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Messages  []struct{ Content string }
					MaxTokens int `json:"max_tokens"`
				}
				json.NewDecoder(r.Body).Decode(&req)
				var start, end int
				fmt.Sscanf(regexp.MustCompile(`本批只返回第\d+至第\d+页`).FindString(req.Messages[len(req.Messages)-1].Content), "本批只返回第%d至第%d页", &start, &end)
				if start < 1 || start != end || end > pages {
					t.Errorf("unsafe direct range %d-%d", start, end)
					w.WriteHeader(400)
					return
				}
				d := directHTTPFixtureDeck(sampleDeck(in))
				d.Slides = d.Slides[start-1 : end]
				finish := "stop"
				raw := ""
				if active {
					calls++
					attempts[start]++
					if start < 4 {
						t.Error("regenerated approved page")
					}
					if calls == 1 {
						firstBudget = req.MaxTokens
					}
					if calls == 2 && (mode == "repair-truncated" || mode == "split-to-single" || mode == "persistent-truncated") && req.MaxTokens <= firstBudget {
						t.Error("repair budget did not increase")
					}
					if mode == "permanent-http" {
						w.WriteHeader(401)
						return
					}
					if mode == "persistent-truncated" || mode == "repair-truncated" && calls == 1 || mode == "split-to-single" && calls == 2 || mode == "quality-repair-truncated" && start == 5 && attempts[start] == 2 {
						finish = "length"
					}
					if mode == "split-to-single" && calls == 1 {
						raw = `{"slides":"invalid"}`
					}
					if mode == "quality-repair-truncated" && start == 5 && attempts[start] == 1 {
						d.Slides[0].Title = sampleDeck(in).Slides[3].Title
					}
					if mode == "cancel-between-batches" && calls == 1 {
						worksCall(t, s, "POST", route+"/cancel", token, nil, 200)
					}
				}
				if raw == "" {
					raw = string(asJSON(d))
				}
				respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": finish, "message": map[string]string{"content": raw}}}})
			}))
			defer provider.Close()
			s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "fixture", "fixture"
			configureDirectHTTPFixture(t, s, in, provider.URL)
			id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
			route = "/api/tasks/" + id
			t.Cleanup(func() { worksCall(t, s, "POST", route+"/cancel", token, nil, 200) })
			if err := s.workOne(context.Background()); err != nil {
				t.Fatal(err)
			}
			preview := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
			for round := 0; round < 8 && preview["status"] == "pending"; round++ {
				makeAutomaticRetryReady(t, s, id)
				if err := s.workOne(context.Background()); err != nil {
					t.Fatal(err)
				}
				preview = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
			}
			if preview["status"] != "awaiting_preview" {
				t.Fatal("preview failed", preview["error"])
			}
			locked := preview["result"].(map[string]any)["slides"]
			for _, entry := range preview["slide_reviews"].([]any) {
				m := entry.(map[string]any)
				worksCall(t, s, "POST", route+"/slides/"+m["id"].(string)+"/review", token, map[string]any{"revision": preview["revision"], "hash": m["hash"], "approved": true}, 200)
			}
			active = true
			worksCall(t, s, "POST", route+"/expand", token, map[string]any{"revision": preview["revision"]}, 202)
			if err := s.workOne(context.Background()); err != nil {
				t.Fatal(err)
			}
			task := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
			for round := 0; round < 8 && task["status"] == "expand_pending"; round++ {
				makeAutomaticRetryReady(t, s, id)
				if err := s.workOne(context.Background()); err != nil {
					t.Fatal(err)
				}
				task = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
			}
			result := task["result"].(map[string]any)["slides"].([]any)
			if len(result) < 3 || !reflect.DeepEqual(locked, result[:3]) {
				t.Fatal("approved preview changed")
			}
			terminal := mode == "persistent-truncated" || mode == "permanent-http" || mode == "cancel-between-batches"
			if terminal {
				want := "failed"
				if mode == "cancel-between-batches" {
					want = "cancelled"
				}
				if task["status"] != want || task["charge_state"] != "released" || len(result) != 3 || calls > 6 {
					t.Fatal("terminal boundary", task["status"], len(result), calls)
				}
				if mode != "persistent-truncated" && calls != 1 {
					t.Fatal("continued rejected/cancelled task", calls)
				}
			} else {
				if task["status"] != "awaiting_confirmation" || len(result) != pages || task["charge_state"] != "reserved" {
					t.Fatal("recovery failed", task["status"], task["error"], len(result), calls)
				}
				var d Deck
				json.Unmarshal(asJSON(task["result"]), &d)
				if issues := coursewareQualityIssues(d, pages); len(issues) > 0 {
					t.Fatal(issues)
				}
				b := worksCall(t, s, "GET", route+"/preview-pptx", token, nil, 200).Body.Bytes()
				if len(b) < 1000 || string(b[:2]) != "PK" {
					t.Fatal("not real PPTX")
				}
				if mode == "12-pages" || mode == "20-pages" {
					stages := []string{}
					for _, entry := range task["events"].([]any) {
						stage := entry.(map[string]any)["stage"].(string)
						if strings.HasPrefix(stage, "batch_") {
							stages = append(stages, stage)
						}
					}
					want := []string{}
					for i := 1; i <= pages; i++ {
						want = append(want, fmt.Sprintf("batch_start_%d_%d", i, i), fmt.Sprintf("batch_complete_%d", i))
					}
					if !reflect.DeepEqual(stages, want) {
						t.Fatal("progress", stages, want)
					}
				}
				worksCall(t, s, "POST", route+"/cancel", token, nil, 200)
			}
			bill := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
			if bill["balance"] != before || bill["reserved"] != float64(0) {
				t.Fatal("changed credits", bill)
			}
		})
	}
}

func TestCoursewarePreviewRaisesRepairBudget(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	calls, firstBudget := 0, 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req struct {
			Messages  []struct{ Content string }
			MaxTokens int `json:"max_tokens"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if !strings.Contains(req.Messages[1].Content, "本批只返回第") || !strings.Contains(req.Messages[1].Content, "slides恰好1项") {
			t.Error("preview gate lost")
		}
		finish := "stop"
		if calls == 1 {
			firstBudget, finish = req.MaxTokens, "length"
		} else if calls == 2 && req.MaxTokens <= firstBudget {
			t.Error("preview retry did not raise output budget")
		}
		d := directHTTPFixtureDeck(sampleDeck(TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "classroom"}))
		match := regexp.MustCompile(`本批只返回第(\d+)至第(\d+)页`).FindString(req.Messages[1].Content)
		var start, end int
		fmt.Sscanf(match, "本批只返回第%d至第%d页", &start, &end)
		if start < 1 || end > 3 || start != end {
			t.Error("invalid preview range")
			w.WriteHeader(400)
			return
		}
		d.Slides = d.Slides[start-1 : end]
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": finish, "message": map[string]string{"content": string(asJSON(d))}}}})
	}))
	defer provider.Close()
	s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "test", "test"
	configureDirectHTTPFixture(t, s, TaskInput{Topic: "浮力", Pages: 8}, provider.URL)
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "classroom", UseKB: true, PreviewFirst: true, IdempotencyKey: newID()}, 202))["id"].(string)
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	task := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
	if calls != 4 || task["status"] != "awaiting_preview" {
		t.Fatal("preview truncation did not recover", calls, task["error"])
	}
	worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
}
