package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestCoursewareGLMSpeedLive(t *testing.T) {
	if os.Getenv("TEST_COURSEWARE_GLM_SPEED_LIVE") != "1" {
		t.Skip("explicit isolated live test opt-in required")
	}
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	in := TaskInput{Type: "courseware", Subject: "数学", Grade: "八年级", Textbook: "人教版", Topic: "一次函数的图像与性质", Detail: "concise", LessonMinutes: 45, Theme: "ocean", PreviewFirst: true, UseKB: true, IdempotencyKey: newID(), Requirements: "教师讲授；学生已学正比例函数。以y=2x+1和y=-x+2说明斜率与截距，图像要有实际数值和单位比例。三页预览包含核心图像、例题、独立练习，正确答案保留备注。"}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	defer worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	s.cfg.Mode = "live"
	target, _ := url.Parse(s.cfg.RenderURL)
	proxy := httputil.NewSingleHostReverseProxy(target)
	var calls atomic.Int32
	renderer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/render/pptx" {
			body, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(body))
			os.WriteFile(fmt.Sprintf("../work/courseware-speed/live-render-%d.json", calls.Add(1)), body, 0600)
		}
		proxy.ServeHTTP(w, r)
	}))
	defer renderer.Close()
	s.cfg.RenderURL = renderer.URL
	s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = os.Getenv("COURSEWARE_LLM_BASE_URL"), os.Getenv("COURSEWARE_LLM_API_KEY"), os.Getenv("COURSEWARE_LLM_MODEL")
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = os.Getenv("EVAL_LLM_BASE_URL"), os.Getenv("EVAL_LLM_API_KEY"), os.Getenv("EVAL_LLM_MODEL")
	s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = os.Getenv("VISION_LLM_BASE_URL"), os.Getenv("VISION_LLM_API_KEY"), os.Getenv("VISION_LLM_MODEL")
	if _, _, model := s.agentModel("courseware"); model != "glm-5-3-flash-260828" {
		t.Fatal("live test is not configured for GLM")
	}
	ctx, stop := context.WithTimeout(withCoursewareArtifacts(context.Background()), 6*time.Minute)
	defer stop()
	tx, err := s.tx(ctx, a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, "UPDATE tasks SET status='researching',mode='live' WHERE id=$1", id)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	started := time.Now()
	var recorded struct {
		Deck Deck `json:"deck"`
	}
	previous, err := os.ReadFile("../work/courseware-speed/live-result.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(previous, &recorded); err != nil || recorded.Deck.Plan == nil {
		t.Fatal("missing actual recorded teaching plan", err)
	}
	plan := recorded.Deck.Plan
	if err = validateTeachingPlan(*plan); err != nil {
		t.Fatal(err)
	}
	in.Pages = len(plan.Pages)
	d, err := s.stagedCoursewarePlan(ctx, a, id, in, nil, Deck{Plan: plan})
	for retry := 0; retry < 2 && err != nil && ctx.Err() == nil; retry++ {
		var unavailable pageReviewUnavailable
		if !errors.As(err, &unavailable) {
			break
		}
		t.Log("temporary review unavailable; automatically rechecking saved pages without restarting the deck")
		d, err = s.stagedCoursewarePlan(ctx, a, id, in, nil, d)
	}
	os.MkdirAll("../work/courseware-speed", 0700)
	os.WriteFile("../work/courseware-speed/live-result.json", asJSON(map[string]any{"model": s.cfg.CoursewareLLMModel, "duration_seconds": time.Since(started).Seconds(), "metrics": json.RawMessage(coursewareTimingReport(ctx)), "deck": d}), 0600)
	if err != nil {
		var unavailable pageReviewUnavailable
		if errors.As(err, &unavailable) {
			t.Logf("review unavailable cause: %v", unavailable.Cause)
		}
		t.Fatal(err)
	}
	if len(d.Slides) != 3 {
		t.Fatal("incomplete preview")
	}
	for _, sl := range d.Slides {
		if !pageCheckPassed(d.PageChecks[sl.ID], false) {
			t.Fatal("incomplete review")
		}
	}
	path, err := s.previewArtifact(ctx, a, id, &d)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || len(b) < 1000 || string(b[:2]) != "PK" {
		t.Fatal("actual PPTX missing", err)
	}
	os.WriteFile("../work/courseware-speed/glm-live-preview.pptx", b, 0600)
	if after := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String(); after != before {
		t.Fatal("live test changed business points")
	}
	t.Logf("GLM preview passed actual content/visual checks: %d planned pages, %.1fs through generation, metrics=%s", len(plan.Pages), time.Since(started).Seconds(), coursewareTimingReport(ctx))
}
