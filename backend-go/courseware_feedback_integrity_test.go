package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestTeacherRevisionRetainsCanvasAndReviewsHTTP(t *testing.T) {
	for _, scenario := range []string{"succeeded", "succeeded-unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			stage := strings.TrimSuffix(scenario, "-unavailable")
			unavailable := scenario != stage
			s := worksServer(t)
			token := worksLogin(t, s, "teacher")
			in := TaskInput{Subject: "数学", Topic: "一次函数", Grade: "八年级", Detail: "default"}
			id := worksFixture(t, s, token, "courseware", stage, in.Topic, in)
			d := sampleDeck(TaskInput{Topic: in.Topic, Pages: 8, Theme: "ocean"})
			assignSlideIDs(&d)
			d.Plan = &TeachingPlan{Summary: "明确课堂顺序"}
			d.EditableFormulas = true
			d.PageChecks = map[string]PageQualityCheck{}
			for i := range d.Slides {
				d.Slides[i].Order = i + 1
				d.Plan.Pages = append(d.Plan.Pages, TeachingPage{Position: i + 1, Goal: d.Slides[i].Title, Task: d.Slides[i].Layout, Minutes: 3})
			}
			d.Slides[0].CanvasPositions = map[string]CanvasOffset{"title": {DX: .01, DY: .01}}
			d.Slides[0].CanvasObjects = []CanvasObject{{ID: "object-teacher", Kind: "text", X: .2, Y: .7, Width: .5, Height: .1, Text: "教师保留的提示", FontSize: 24}}
			d.Slides[0].CanvasOrder = []string{"title", "body", "object-teacher"}
			for _, sl := range d.Slides {
				d.PageChecks[sl.ID] = PageQualityCheck{Hash: slideContentHash(d, sl), Content: "checked", Visual: "checked", Issues: []PageQualityIssue{}}
			}
			tx, err := s.tx(context.Background(), 1)
			if err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(context.Background(), "UPDATE tasks SET result=$1,mode='live' WHERE id=$2", asJSON(d), id)
			if err != nil {
				t.Fatal(err)
			}
			if stage != "succeeded" {
				if _, err = tx.Exec(context.Background(), "UPDATE tasks SET charge_state='reserved' WHERE id=$1", id); err != nil {
					t.Fatal(err)
				}
				if _, err = tx.Exec(context.Background(), "UPDATE members SET reserved=reserved+20 WHERE id=1"); err != nil {
					t.Fatal(err)
				}
			}
			if err = tx.Commit(context.Background()); err != nil {
				t.Fatal(err)
			}
			reviews := 0
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct{ Model string }
				json.NewDecoder(r.Body).Decode(&req)
				raw := `{"issues":[]}`
				if req.Model == "generation" {
					sl := d.Slides[0]
					sl.Title = "反馈已落实"
					sl.ID, sl.Order, sl.CanvasPositions, sl.CanvasObjects, sl.CanvasOrder = "model-id", 0, nil, nil, nil
					if stage == "succeeded" {
						raw = string(asJSON(sl))
					} else {
						raw = string(asJSON(map[string]any{"reply": "已修改当前页标题，其他内容保留。", "slide": sl}))
					}
				} else {
					reviews++
					if unavailable {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
				}
				respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": raw}}}})
			}))
			defer provider.Close()
			s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "fixture", "generation"
			s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = provider.URL, "fixture", "review"
			s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = provider.URL, "fixture", "vision"
			one := d
			one.Slides = d.Slides[:1]
			if _, err = s.renderCoursewareAvailable(context.Background(), one); err != nil {
				t.Fatal("invalid original render fixture", err)
			}
			route := "/api/tasks/" + id
			before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
			if stage == "succeeded" {
				child := worksObject(t, worksCall(t, s, "POST", route+"/rewrite", token, map[string]any{"revision": 0, "slide": 0, "instruction": "改进本页标题，保留教师画布", "idempotency_key": newID()}, 202))["id"].(string)
				if err = s.workOne(context.Background()); err != nil {
					t.Fatal(err)
				}
				v := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+child, token, nil, 200))
				if unavailable {
					if v["status"] != "failed" || v["charge_state"] != "released" {
						t.Fatal("unavailable review overwrote or charged the original deck", v["status"], v["charge_state"])
					}
					parent := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
					var retained Deck
					json.Unmarshal(asJSON(parent["result"]), &retained)
					after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
					if !reflect.DeepEqual(retained, d) || after["available"] != before["available"] || after["reserved"] != before["reserved"] {
						t.Fatal("review outage changed saved original or business points")
					}
					return
				}
				if v["status"] != "succeeded" {
					t.Fatal("revision did not complete", v["error"])
				}
			} else {
				worksCall(t, s, "POST", route+"/conversation", token, map[string]any{"revision": 0, "mode": "revise", "slide_id": d.Slides[0].ID, "content": "改进本页标题，保留教师画布", "idempotency_key": newID()}, 200)
			}
			v := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
			var done Deck
			json.Unmarshal(asJSON(v["result"]), &done)
			sl := done.Slides[0]
			if sl.Title != "反馈已落实" || sl.ID != d.Slides[0].ID || sl.Order != 1 || !reflect.DeepEqual(sl.CanvasObjects, d.Slides[0].CanvasObjects) || !reflect.DeepEqual(sl.CanvasPositions, d.Slides[0].CanvasPositions) || !reflect.DeepEqual(sl.CanvasOrder, d.Slides[0].CanvasOrder) {
				t.Fatal("teacher revision lost page identity or canvas", sl.Order, sl.CanvasObjects)
			}
			if !reflect.DeepEqual(done.Slides[1:], d.Slides[1:]) {
				t.Fatal("revision changed neighboring pages")
			}
			check := done.PageChecks[sl.ID]
			if scenario == "succeeded" && (check.Hash != slideContentHash(done, sl) || !pageCheckPassed(check, false)) {
				t.Fatal("modified planned page was delivered without independent content and screenshot checks", reviews, check)
			}
		})
	}
}
