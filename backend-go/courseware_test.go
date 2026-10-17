package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCoursewarePageCountThemesAndLayoutsHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	for _, theme := range []string{"forest", "violet", "ocean", "midnight", "paper", "coral"} {
		id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, TaskInput{Type: "courseware", Topic: "浮力", Pages: 20, Theme: theme, UseKB: true, IdempotencyKey: newID()}, 202))["id"].(string)
		v := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
		in := v["input"].(map[string]any)
		if in["pages"] != float64(20) || in["theme"] != theme {
			t.Fatal("request silently changed", in)
		}
		worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
	}
	// Populate a reviewable 20-page outline, then validate edits through confirmation.
	id := worksFixture(t, s, token, "courseware", "awaiting_confirmation", "浮力", TaskInput{})
	d := sampleDeck(TaskInput{Topic: "浮力", Pages: 20, Theme: "coral"})
	d.Slides[0].Layout = "unsupported"
	worksCall(t, s, "POST", "/api/tasks/"+id+"/confirm", token, map[string]any{"result": d, "revision": 0, "accept_review": true}, 400)
	d.Slides[0].Layout = "cover"
	d.Slides[0].Section = strings.Repeat("字", 81)
	worksCall(t, s, "POST", "/api/tasks/"+id+"/confirm", token, map[string]any{"result": d, "revision": 0, "accept_review": true}, 400)
	d.Slides[0].Section = "学习目标"
	worksCall(t, s, "POST", "/api/tasks/"+id+"/confirm", token, map[string]any{"result": d, "revision": 0, "accept_review": true}, 202)
	result := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))["result"].(map[string]any)
	if result["slides"].([]any)[0].(map[string]any)["section"] != "学习目标" {
		t.Fatal("section lost on confirmation")
	}
	animation := worksFixture(t, s, token, "animation", "awaiting_confirmation", "浮力", TaskInput{})
	worksCall(t, s, "POST", "/api/tasks/"+animation+"/confirm", token, map[string]any{"result": d, "revision": 0, "accept_review": true}, 503)
}

func TestGenerationExactCountAndBoundedProviderContract(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	var actor Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &actor)
	returnedPages := 8
	finish := "stop"
	var request struct {
		MaxTokens int `json:"max_tokens"`
		Messages  []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		d := sampleDeck(TaskInput{Type: "courseware", Topic: "浮力", Pages: returnedPages, Theme: "forest"})
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": finish, "message": map[string]string{"content": string(asJSON(d))}}}})
	}))
	defer provider.Close()
	s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMModel = "live", provider.URL, "local-test"
	in := TaskInput{Type: "courseware", Topic: "浮力", Pages: 12, Theme: "forest"}
	if _, err := s.generate(context.Background(), actor, "count-mismatch", in, nil); err == nil {
		t.Fatal("accepted 8 pages when teacher requested 12")
	}
	for _, count := range []int{8, 12, 20} {
		in.Pages, returnedPages = count, count
		d, err := s.generate(context.Background(), actor, "correct-count", in, nil)
		if err != nil || len(d.Slides) != count {
			t.Fatalf("%d pages: %v", count, err)
		}
		if request.MaxTokens < count*500 || request.MaxTokens > 12000 {
			t.Fatal("inappropriate output budget", count, request.MaxTokens)
		}
		prompt := request.Messages[1].Content
		for _, word := range []string{"视觉", "实验", "步骤", "答案", "误区", "layout", "cover", "exercise"} {
			if !strings.Contains(prompt, word) {
				t.Errorf("generation brief missing %s", word)
			}
		}
	}
	finish = "length"
	if _, err := s.generate(context.Background(), actor, "truncated", in, nil); err == nil {
		t.Fatal("accepted token-truncated provider output")
	}
}

func TestSampleCoursewareVariesPagesAndKeepsThemeVisual(t *testing.T) {
	for _, topic := range []string{"浮力", "勾股定理", "一次函数", "欧姆定律", "光的反射"} {
		for _, count := range []int{8, 12, 20} {
			d := sampleDeck(TaskInput{Topic: topic, Pages: count, Theme: "forest"})
			if len(d.Slides) != count {
				t.Fatal("wrong sample page count")
			}
			seen := map[string]bool{}
			for _, slide := range d.Slides {
				if seen[slide.Title] || slide.Notes == "" || slide.Layout == "" {
					t.Fatalf("repetitive or empty sample page: %s %+v", topic, slide)
				}
				seen[slide.Title] = true
			}
			other := sampleDeck(TaskInput{Topic: topic, Pages: count, Theme: "coral"})
			if string(asJSON(d.Slides)) != string(asJSON(other.Slides)) || d.Lesson != other.Lesson {
				t.Fatal("visual theme changed subject matter")
			}
		}
	}
}
