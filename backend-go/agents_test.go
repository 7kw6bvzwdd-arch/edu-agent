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

func TestAgentModelFallbackAndGradePrompts(t *testing.T) {
	s := &Server{cfg: Config{LLMURL: "https://legacy.invalid/v3", LLMKey: "legacy-test", LLMModel: "legacy-model", CoursewareLLMModel: "course-model", AnimationLLMURL: "https://animation.invalid/v3", AnimationLLMKey: "animation-test"}}
	url, key, model := s.agentModel("courseware")
	if url != s.cfg.LLMURL || key != s.cfg.LLMKey || model != "course-model" {
		t.Fatal("courseware must fall back per field")
	}
	url, key, model = s.agentModel("animation")
	if url != s.cfg.AnimationLLMURL || key != s.cfg.AnimationLLMKey || model != "legacy-model" {
		t.Fatal("animation fallback must be independent")
	}
	in := TaskInput{Type: "courseware", Topic: "一次函数", Grade: "七年级", Pages: 12}
	prompt := coursewarePrompt(in, nil)
	for _, phrase := range []string{"七年级", "前置知识", "超纲", "教材索引"} {
		if !strings.Contains(prompt, phrase) {
			t.Errorf("missing grade guidance: %s", phrase)
		}
	}
}

func TestAgentProviderRoutingAndUsage(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "admin")
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	called := map[string]int{}
	provider := func(label, expectedModel, expectedKey string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Model string `json:"model"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			if req.Model != expectedModel || r.Header.Get("Authorization") != "Bearer "+expectedKey {
				t.Error("wrong provider credentials or model", label)
			}
			called[label]++
			respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "{}"}}}})
		}))
	}
	course := provider("course", "course-model", "course-test")
	defer course.Close()
	animation := provider("animation", "animation-model", "animation-test")
	defer animation.Close()
	legacy := provider("eval", "eval-model", "eval-test")
	defer legacy.Close()
	s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = course.URL, "course-test", "course-model"
	s.cfg.AnimationLLMURL, s.cfg.AnimationLLMKey, s.cfg.AnimationLLMModel = animation.URL, "animation-test", "animation-model"
	s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.EvalModel = legacy.URL, "eval-test", "eval-model"
	for _, entry := range []struct{ kind, operation string }{{"courseware", "courseware.generate"}, {"courseware", "slide.rewrite"}, {"animation", "animation.storyboard"}, {"animation", "animation.html.plan"}, {"animation", "animation.html.revise"}} {
		if _, err := s.agentChat(context.Background(), a, "agent-route", entry.kind, entry.operation, "test"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.chat(context.Background(), a, "agent-route", s.cfg.EvalModel, "courseware.review", "test"); err != nil {
		t.Fatal(err)
	}
	if called["course"] != 2 || called["animation"] != 3 || called["eval"] != 1 {
		t.Fatal("agent requests crossed providers", called)
	}
	dashboard := worksObject(t, worksCall(t, s, "GET", "/api/dashboard", token, nil, 200))
	models := map[string]string{}
	for _, row := range dashboard["usage"].([]any) {
		v := row.(map[string]any)
		if _, ok := models[v["operation"].(string)]; !ok {
			models[v["operation"].(string)] = v["model"].(string)
		}
	}
	if models["slide.rewrite"] != "course-model" || models["animation.html.revise"] != "animation-model" || models["courseware.review"] != "eval-model" {
		t.Fatal("usage did not record actual routed model", models)
	}
}

func TestAgentHealthWithoutHTMLConfiguration(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	s.cfg.Mode = "live"
	s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = "http://127.0.0.1:1", "test", "course-model"
	s.cfg.AnimationLLMURL, s.cfg.AnimationLLMKey, s.cfg.AnimationLLMModel = "http://127.0.0.1:1", "test", "animation-model"
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	health := worksObject(t, worksCall(t, s, "GET", "/api/health", "", nil, 200))
	if health["animation_geometry_version"] != "geometry-v1" || !strings.Contains(string(asJSON(health["animation_archetypes"])), "geo.transform@1") {
		t.Fatal("geometry capabilities missing from health")
	}
	if health["courseware_configured"] != true || health["animation_configured"] != false || health["animation_video_configured"] != false || health["animation_service_status"] != "unconfigured" {
		t.Fatal("disabled animation advertised as available", health)
	}
	for _, engine := range []string{"ark", "manim"} {
		in := TaskInput{Type: "animation", AnimationEngine: engine, Topic: "一次函数", Pages: 8, UseKB: true, IdempotencyKey: newID()}
		w := worksCall(t, s, "POST", "/api/tasks", token, in, 503)
		if !strings.Contains(w.Body.String(), "旧版动画生成已移除") {
			t.Fatal("rebuild rejection missing", w.Body.String())
		}
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before {
		t.Fatal("disabled animation changed billing")
	}
}

func TestAgentWorkflowsReachConfirmationWithOwnProviders(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "admin")
	makeProvider := func(kind, expectedModel string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Model    string `json:"model"`
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
			}
			if req.Model != expectedModel || len(req.Messages) < 2 {
				t.Error("wrong workflow provider", kind)
				return
			}
			prompt := req.Messages[1].Content
			if !strings.Contains(prompt, "八年级") {
				t.Error("grade not passed to workflow", kind)
			}
			d := sampleDeck(TaskInput{Type: kind, Topic: "一次函数", Theme: "forest", Pages: 8})
			if strings.Contains(prompt, "不生成页面全文") {
				plan := TeachingPlan{Summary: "显式规划 fixture"}
				plan.LearningGoals = []LearningGoal{{ID: "g1", Text: "理解函数并完成任务", Core: true}}
				for i, sl := range d.Slides {
					plan.Pages = append(plan.Pages, TeachingPage{Position: i + 1, Goal: sl.Title, Task: sl.Layout, Minutes: 3, Preview: i == 1 || i == 3 || i == 4})
					plan.Pages[i].GoalRefs = []string{"g1"}
					if sl.Layout == "exercise" {
						plan.Pages[i].ChecksGoals = []string{"g1"}
					}
				}
				respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(asJSON(plan))}}}})
				return
			}
			var start, end int
			fmt.Sscanf(regexp.MustCompile(`本批只返回第\d+至第\d+页`).FindString(prompt), "本批只返回第%d至第%d页", &start, &end)
			if start < 1 || start != end || end > len(d.Slides) {
				t.Error("provider must receive a single-page request")
				w.WriteHeader(400)
				return
			}
			d.Slides = d.Slides[start-1 : end]
			respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(asJSON(d))}}}})
		}))
	}
	course := makeProvider("courseware", "course-model")
	defer course.Close()
	s.cfg.Mode = "live"
	s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = course.URL, "test", "course-model"
	reviewer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"issues":[]}`}}}})
	}))
	defer reviewer.Close()
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = reviewer.URL, "review-fixture", "review-model"
	s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = reviewer.URL, "vision-fixture", "vision-model"
	for _, kind := range []string{"courseware"} {
		in := TaskInput{Type: kind, Grade: "八年级", Topic: "一次函数", Theme: "forest", Pages: 8, UseKB: true, IdempotencyKey: newID()}
		seed := in
		seed.Type, seed.AnimationEngine = "courseware", ""
		id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, seed, 202))["id"].(string)
		t.Cleanup(func() { worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200) })
		if err := s.workOne(context.Background()); err != nil {
			t.Fatal(err)
		}
		result := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
		worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
		if result["status"] != "awaiting_preview" {
			t.Fatal("agent did not produce a confirmable outline", kind, result["status"], result["error"])
		}
	}
}
