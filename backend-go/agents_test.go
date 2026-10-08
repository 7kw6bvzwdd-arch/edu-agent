package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	manim := animationPrompt(in, nil, "5")
	if !strings.Contains(manim, "20–45秒") {
		t.Fatal("Manim duration lost")
	}
	in.AnimationEngine = "ark"
	ark := animationPrompt(in, nil, "5")
	if !strings.Contains(ark, "5秒") || strings.Contains(ark, "20–45秒") || strings.Contains(ark, "480p") {
		t.Fatal("cloud storyboard claims Manim duration or resolution")
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
	for _, entry := range []struct{ kind, operation string }{{"courseware", "courseware.generate"}, {"courseware", "slide.rewrite"}, {"animation", "animation.storyboard"}, {"animation", "animation.code"}, {"animation", "animation.repair"}} {
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
	if models["slide.rewrite"] != "course-model" || models["animation.repair"] != "animation-model" || models["courseware.review"] != "eval-model" {
		t.Fatal("usage did not record actual routed model", models)
	}
}

func TestAgentHealthAndArkPreflight(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	s.cfg.Mode = "live"
	s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = "http://127.0.0.1:1", "test", "course-model"
	s.cfg.AnimationLLMURL, s.cfg.AnimationLLMKey, s.cfg.AnimationLLMModel = "http://127.0.0.1:1", "test", "animation-model"
	health := worksObject(t, worksCall(t, s, "GET", "/api/health", "", nil, 200))
	if health["courseware_media_version"] != "media-v1" {
		t.Fatal("missing courseware capability")
	}
	if health["courseware_configured"] != true || health["animation_configured"] != true || health["animation_video_configured"] != false {
		t.Fatal("agent health does not reflect independent configuration", health)
	}
	if health["animation_video_duration"] != float64(5) {
		t.Fatal("video duration must default to integer seconds", health)
	}
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	in := TaskInput{Type: "animation", AnimationEngine: "ark", Topic: "一次函数", Pages: 8, UseKB: true, IdempotencyKey: newID()}
	w := worksCall(t, s, "POST", "/api/tasks", token, in, 503)
	if !strings.Contains(w.Body.String(), "ARK_VIDEO") {
		t.Fatal("missing actionable cloud configuration error")
	}
	s.cfg.VideoURL = "https://ark.cn-beijing.volces.com/api/v3"
	s.cfg.VideoKey, s.cfg.VideoModel = "test", "configured-video-model"
	for _, duration := range []string{"1", "16", "0", "-1", "2.5", "invalid"} {
		s.cfg.VideoDuration = duration
		w := worksCall(t, s, "POST", "/api/tasks", token, in, 503)
		if !strings.Contains(w.Body.String(), "ARK_VIDEO_DURATION") {
			t.Fatal("invalid duration did not fail during configuration preflight", duration)
		}
		health := worksObject(t, worksCall(t, s, "GET", "/api/health", "", nil, 200))
		if health["animation_video_configured"] != false {
			t.Fatal("health advertised an invalid video duration", duration)
		}
		if after := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String(); after != before {
			t.Fatal("invalid duration reserved credits or changed ledger", duration)
		}
	}
	for _, duration := range []string{"2", "5", "15"} {
		s.cfg.VideoDuration = duration
		health := worksObject(t, worksCall(t, s, "GET", "/api/health", "", nil, 200))
		if health["animation_video_configured"] != true {
			t.Fatal("valid boundary duration was rejected", duration)
		}
	}
	s.cfg.VideoDuration = "5"
	s.cfg.Mode = "sample"
	worksCall(t, s, "POST", "/api/tasks", token, in, 503)
	if after := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String(); after != before {
		t.Fatal("unconfigured video reserved credits")
	}
	in.AnimationEngine = "invalid"
	worksCall(t, s, "POST", "/api/tasks", token, in, 400)
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
			if !strings.Contains(prompt, "八年级") || !strings.Contains(prompt, "前置知识") {
				t.Error("grade not passed to workflow", kind)
			}
			if kind == "animation" && (!strings.Contains(prompt, "5秒") || !strings.Contains(prompt, "动作节点") || strings.Contains(prompt, "20–45秒")) {
				t.Error("cloud storyboard duration mismatch")
			}
			d := sampleDeck(TaskInput{Type: kind, AnimationEngine: "ark", Topic: "一次函数", Theme: "forest", Pages: 8})
			respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(asJSON(d))}}}})
		}))
	}
	course := makeProvider("courseware", "course-model")
	defer course.Close()
	animation := makeProvider("animation", "animation-model")
	defer animation.Close()
	s.cfg.Mode = "live"
	s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = course.URL, "test", "course-model"
	s.cfg.AnimationLLMURL, s.cfg.AnimationLLMKey, s.cfg.AnimationLLMModel = animation.URL, "test", "animation-model"
	s.cfg.VideoURL, s.cfg.VideoKey, s.cfg.VideoModel, s.cfg.VideoDuration = "http://127.0.0.1:1", "test", "never-called", "5"
	for _, kind := range []string{"courseware", "animation"} {
		in := TaskInput{Type: kind, Grade: "八年级", Topic: "一次函数", Theme: "forest", Pages: 8, UseKB: true, IdempotencyKey: newID()}
		if kind == "animation" {
			in.AnimationEngine = "ark"
		}
		// Seed through the real reservation API, then switch only the animation fixture.
		// This test exercises provider routing and confirmation, not Docker readiness.
		seed := in
		seed.Type, seed.AnimationEngine = "courseware", ""
		id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, seed, 202))["id"].(string)
		t.Cleanup(func() { worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200) })
		if kind == "animation" {
			var a Actor
			if err := json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			tx, err := s.tx(ctx, a.TenantID)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			changed, err := tx.Exec(ctx, "UPDATE tasks SET type='animation',input=$1 WHERE id=$2 AND member_id=$3 AND status='pending' AND charge_state='reserved'", asJSON(in), id, a.MemberID)
			if err != nil {
				t.Fatal(err)
			}
			if changed.RowsAffected() != 1 {
				t.Fatal("animation fixture was not pending with a reservation")
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.workOne(context.Background()); err != nil {
			t.Fatal(err)
		}
		result := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
		worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
		if result["status"] != "awaiting_confirmation" {
			t.Fatal("agent did not produce a confirmable outline", kind, result["status"], result["error"])
		}
	}
}
