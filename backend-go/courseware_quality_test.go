package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCoursewareDraftQualityChecks(t *testing.T) {
	in := TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "forest"}
	d := sampleDeck(in)
	if issues := coursewareQualityIssues(d, in.Pages); len(issues) != 0 {
		t.Fatalf("valid varied sample failed: %v", issues)
	}
	d.Slides[1].Title = d.Slides[0].Title
	d.Slides[2].Bullets = append([]string(nil), d.Slides[1].Bullets...)
	d.Slides[3].Notes = ""
	for i := range d.Slides {
		d.Slides[i].Layout = "concept"
	}
	issues := strings.Join(coursewareQualityIssues(d, in.Pages), "；")
	for _, item := range []string{"重复标题", "重复正文", "版式", "讲解备注"} {
		if !strings.Contains(issues, item) {
			t.Errorf("missing %s: %s", item, issues)
		}
	}
}

func TestValidTablePageIsNotRegeneratedForBulletCount(t *testing.T) {
	d := sampleDeck(TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "ocean"})
	d.Slides[4].Bullets = []string{"只改深度，V排和液体不变，观察F示", "下移前后示数不变，说明什么？", "部分浸入时，深度和V排同时变；如何区分？"}
	d.Slides[4].Table = &TeachingTable{Headers: []string{"深度", "F示/N"}, Rows: [][]string{{"5cm", "3"}, {"10cm", "3"}}}
	if issues := coursewareQualityIssues(d, 8); len(issues) != 0 {
		t.Fatal("valid short questions would regenerate a reviewed page", issues)
	}
}

func TestCoursewarePreflightAndSingleRepair(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	in := TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "forest"}
	modelCalls, renderCalls, firstRenderFails, invalidMedia := 0, 0, false, false
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelCalls++
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
			ResponseFormat map[string]string `json:"response_format"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.ResponseFormat["type"] != "json_object" {
			t.Error("missing JSON mode")
		}
		if modelCalls%2 == 0 && !strings.Contains(req.Messages[len(req.Messages)-1].Content, "重复标题") && !firstRenderFails {
			t.Error("repair missed draft quality issue")
		}
		d := sampleDeck(in)
		if modelCalls%2 == 1 && !firstRenderFails {
			d.Slides[1].Title = d.Slides[0].Title
			if invalidMedia {
				d.Slides[6].Table.Rows = [][]string{{"missing column"}}
			}
		}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(asJSON(d))}}}})
	}))
	defer provider.Close()
	renderer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			respond(w, 200, map[string]string{"courseware_media_version": "media-v1"})
			return
		}
		renderCalls++
		if firstRenderFails && renderCalls == 1 {
			respond(w, 422, map[string]any{"detail": map[string]any{"code": "text_overflow", "page": 2, "field": "bullets"}})
			return
		}
		w.Write([]byte("valid-pptx"))
	}))
	defer renderer.Close()
	s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "test", "test"
	s.cfg.RenderURL = renderer.URL
	d, err := s.generate(context.Background(), a, "courseware-quality-test", in, nil)
	if err != nil || modelCalls != 2 || renderCalls != 2 || len(coursewareQualityIssues(d, in.Pages)) != 0 || !d.NeedsReview || !strings.Contains(d.Notice, "未找到可引用资料") {
		t.Fatal("failed to repair before teacher confirmation", err, modelCalls, renderCalls)
	}
	modelCalls, renderCalls, firstRenderFails = 0, 0, true
	if _, err = s.generate(context.Background(), a, "courseware-quality-test", in, nil); err != nil || modelCalls != 2 || renderCalls != 2 {
		t.Fatal("failed to repair PPTX overflow before confirmation", err, modelCalls, renderCalls)
	}
	modelCalls, renderCalls, firstRenderFails, invalidMedia = 0, 0, false, true
	// The malformed row is now normalized before the first render; duplicate
	// teaching titles still require the single bounded model repair.
	if _, err = s.generate(context.Background(), a, "courseware-media-repair", in, nil); err != nil || modelCalls != 2 || renderCalls != 2 {
		t.Fatal("malformed media did not reach bounded repair", err, modelCalls, renderCalls)
	}
}

func TestCoursewareRepairFailuresAreBounded(t *testing.T) {
	for _, mode := range []string{"still-invalid", "fabricated-source", "renderer-unavailable"} {
		t.Run(mode, func(t *testing.T) {
			s := worksServer(t)
			token := worksLogin(t, s, "teacher")
			var a Actor
			json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
			in := TaskInput{Type: "courseware", Topic: "勾股定理", Pages: 8, Theme: "forest"}
			calls := 0
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				d := sampleDeck(in)
				if mode != "renderer-unavailable" && (calls == 1 || mode == "still-invalid") {
					d.Slides[1].Title = d.Slides[0].Title
				}
				if calls == 2 && mode == "fabricated-source" {
					d.Slides[1].Notes += "[S999]"
				}
				respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(asJSON(d))}}}})
			}))
			defer provider.Close()
			renderer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/health" {
					respond(w, 200, map[string]string{"courseware_media_version": "media-v1"})
					return
				}
				if mode == "renderer-unavailable" {
					w.WriteHeader(503)
				} else {
					w.Write([]byte("valid-pptx"))
				}
			}))
			defer renderer.Close()
			s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel, s.cfg.RenderURL = "live", provider.URL, "test", "test", renderer.URL
			before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))["balance"]
			_, err := s.generate(context.Background(), a, "courseware-failure-test", in, nil)
			want := 2
			if mode == "renderer-unavailable" {
				want = 1
			}
			if err == nil || calls != want {
				t.Fatal("failure escaped or caused unbounded repair", err, calls)
			}
			if after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))["balance"]; after != before {
				t.Fatal("draft repair charged business credits")
			}
		})
	}
}
