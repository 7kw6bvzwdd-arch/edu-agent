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
			id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
			route := "/api/tasks/" + id
			t.Cleanup(func() { worksCall(t, s, "POST", route+"/cancel", token, nil, 200) })
			if err := s.workOne(context.Background()); err != nil {
				t.Fatal(err)
			}
			preview := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
			locked := preview["result"].(map[string]any)["slides"]
			for _, entry := range preview["slide_reviews"].([]any) {
				meta := entry.(map[string]any)
				worksCall(t, s, "POST", route+"/slides/"+meta["id"].(string)+"/review", token, map[string]any{"revision": preview["revision"], "hash": meta["hash"], "approved": true}, 200)
			}
			calls, firstBudget := 0, 0
			ranges := regexp.MustCompile(`本批只返回第(\d+)至第(\d+)页`)
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var request struct {
					Messages  []struct{ Content string }
					MaxTokens int `json:"max_tokens"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				prompt := request.Messages[len(request.Messages)-1].Content
				match := ranges.FindStringSubmatch(prompt)
				if match == nil {
					t.Error("expansion still requests all remaining pages in one response")
					respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "length"}}})
					return
				}
				var start, end int
				fmt.Sscanf(match[0], "本批只返回第%d至第%d页", &start, &end)
				if end-start+1 > 3 || start < 4 || end > pages {
					t.Errorf("unsafe batch range %d-%d", start, end)
				}
				if calls == 1 {
					firstBudget = request.MaxTokens
				}
				if calls == 2 && (mode == "repair-truncated" || mode == "split-to-single" || mode == "persistent-truncated") && request.MaxTokens <= firstBudget {
					t.Error("repair reused the original token limit")
				}
				if mode == "permanent-http" {
					w.WriteHeader(401)
					return
				}
				finish, raw := "stop", ""
				if mode == "persistent-truncated" || mode == "split-to-single" && end > start || mode == "repair-truncated" && calls == 1 || mode == "quality-repair-truncated" && calls == 10 {
					finish, raw = "length", `{"slides":[`
				} else if mode == "split-to-single" && calls == 1 {
					raw = `{"slides":"invalid"}`
				} else {
					d := sampleDeck(in)
					d.Slides = d.Slides[start-1 : end]
					if mode == "quality-repair-truncated" && calls <= 9 {
						for i := range d.Slides {
							if d.Slides[i].Layout == "example" {
								d.Slides[i].Layout = "concept"
							}
						}
					}
					d.Lesson = fmt.Sprintf("第%d至%d页教案", start, end)
					raw = string(asJSON(d))
				}
				if mode == "cancel-between-batches" && calls == 1 {
					worksCall(t, s, "POST", route+"/cancel", token, nil, 200)
				}
				respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": finish, "message": map[string]string{"content": raw}}}})
			}))
			defer provider.Close()
			worksCall(t, s, "POST", route+"/expand", token, map[string]any{"revision": preview["revision"]}, 202)
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
			if err = s.workOne(context.Background()); err != nil {
				t.Fatal(err)
			}
			task := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
			if mode == "12-pages" || mode == "20-pages" {
				var stages []string
				for _, row := range task["events"].([]any) {
					stage := row.(map[string]any)["stage"].(string)
					if strings.HasPrefix(stage, "batch_") {
						stages = append(stages, stage)
					}
				}
				var want []string
				for start := 1; start <= pages; start++ {
					end := start
					want = append(want, fmt.Sprintf("batch_start_%d_%d", start, end), fmt.Sprintf("batch_complete_%d", end))
				}
				if !reflect.DeepEqual(stages, want) {
					t.Fatalf("HTTP progress events: got %v, want %v", stages, want)
				}
			}
			result := task["result"].(map[string]any)["slides"].([]any)
			if !reflect.DeepEqual(locked, result[:3]) {
				t.Fatal("approved first three slides changed")
			}
			if mode == "persistent-truncated" {
				if task["status"] != "expand_pending" || task["stage"] != "auto_recovering" || task["charge_state"] != "reserved" || len(result) != 3 || calls > 6 {
					t.Fatal("truncation lost saved preview, requires manual retry, or exceeds local call limit", task["status"], task["stage"], calls)
				}
				// Recoverable truncation now waits on the automatic queue. Explicit
				// cancellation must still stop it and release the same reservation.
				worksCall(t, s, "POST", route+"/cancel", token, nil, 200)
				task = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
			}
			if mode == "persistent-truncated" || mode == "permanent-http" || mode == "cancel-between-batches" {
				want := "failed"
				if mode == "cancel-between-batches" || mode == "persistent-truncated" {
					want = "cancelled"
				}
				if task["status"] != want || task["charge_state"] != "released" || len(result) != 3 || calls > 6 {
					t.Fatal("terminal failure did not preserve preview/release once/bound calls", task["status"], calls)
				}
				if mode != "persistent-truncated" && calls != 1 {
					t.Fatal("cancelled or rejected task continued calling model", calls)
				}
			} else {
				if task["status"] != "awaiting_confirmation" || len(result) != pages || task["charge_state"] != "reserved" {
					t.Fatal("length recovery failed", task["status"], task["error"], len(result))
				}
				var deck Deck
				json.Unmarshal(asJSON(task["result"]), &deck)
				if issues := coursewareQualityIssues(deck, pages); len(issues) != 0 {
					t.Fatal("batched deck failed global teaching quality", issues)
				}
				artifact := worksCall(t, s, "GET", route+"/preview-pptx", token, nil, 200).Body.Bytes()
				if len(artifact) < 1000 || string(artifact[:2]) != "PK" {
					t.Fatal("recovery did not produce a real PPTX")
				}
				worksCall(t, s, "POST", route+"/cancel", token, nil, 200)
			}
			billing := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
			if billing["balance"] != before || billing["reserved"] != float64(0) {
				t.Fatal("batch recovery charged extra or retained reservation", billing)
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
		if !strings.Contains(req.Messages[1].Content, "只生成前三页") {
			t.Error("preview gate lost")
		}
		finish := "stop"
		if calls == 1 {
			firstBudget, finish = req.MaxTokens, "length"
		} else if calls == 2 && req.MaxTokens <= firstBudget {
			t.Error("preview retry did not raise output budget")
		}
		d := sampleDeck(TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "classroom"})
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
