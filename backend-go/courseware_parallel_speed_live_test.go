package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Same actual PPTX and review inputs, paired cold Office runs. This measures
// page review scheduling, not complete-deck generation or classroom quality.
func TestIndependentReviewSpeedLive(t *testing.T) {
	if os.Getenv("TEST_INDEPENDENT_REVIEW_SPEED_LIVE") != "1" {
		t.Skip("explicit isolated live opt-in required")
	}
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	id := worksFixture(t, s, token, "courseware", "succeeded", "双审核真实测速", TaskInput{})
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	s.cfg.Mode = "live"
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = os.Getenv("EVAL_LLM_BASE_URL"), os.Getenv("EVAL_LLM_API_KEY"), os.Getenv("EVAL_LLM_MODEL")
	s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = os.Getenv("VISION_LLM_BASE_URL"), os.Getenv("VISION_LLM_API_KEY"), os.Getenv("VISION_LLM_MODEL")
	if !s.coursewareQualityConfigured() {
		t.Fatal("live reviewers unavailable")
	}
	d := Deck{Title: "浮力方向", Theme: "ocean", Slides: []Slide{{ID: "speed-force", Title: "液体对物体的浮力", Layout: "concept", Bullets: []string{"静止均匀液体对浸入物体的压力合成浮力。", "浮力方向竖直向上，与重力方向相反。"}, Notes: "本页只讨论浮力方向；下一页学习用测力计测量浮力。"}}}
	if fixture := os.Getenv("TEST_SPEED_DECK"); fixture != "" {
		raw, e := os.ReadFile(fixture)
		if e != nil {
			t.Fatal(e)
		}
		if e = json.Unmarshal(raw, &d); e != nil {
			t.Fatal(e)
		}
		d.Slides = d.Slides[12:13]
	}
	pptx, err := s.renderPPTX(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	type result struct {
		Mode    string             `json:"mode"`
		Seconds float64            `json:"seconds"`
		Content string             `json:"content"`
		Visual  string             `json:"visual"`
		Issues  []PageQualityIssue `json:"issues"`
	}
	var results []result
	for _, mode := range []string{"sequential", "parallel", "parallel", "sequential", "sequential", "parallel"} {
		ctx, stop := context.WithTimeout(withCoursewareArtifacts(context.Background()), 2*time.Minute)
		start := time.Now()
		check := PageQualityCheck{}
		if mode == "parallel" {
			check, err = s.inspectGeneratedPage(ctx, a, id, TaskInput{Subject: "物理", Grade: "八年级"}, d, pptx)
		} else {
			// Previous scheduler prepared Office screenshot concurrently with content,
			// then waited for content before starting the actual visual model.
			type snapshot struct {
				png []byte
				err error
			}
			images := make(chan snapshot, 1)
			go func() { png, e := s.coursewarePagePNG(ctx, a, id, pptx); images <- snapshot{png, e} }()
			check.Issues, check.Content, err = s.reviewTeachingPage(ctx, a, id, TaskInput{Subject: "物理", Grade: "八年级"}, d)
			image := <-images
			if err == nil {
				err = image.err
			}
			if err == nil {
				var issues []PageQualityIssue
				issues, check.Visual, err = s.reviewPPTPagePNG(ctx, a, id, d, image.png)
				check.Issues = append(check.Issues, issues...)
			}
		}
		seconds := time.Since(start).Seconds()
		stop()
		results = append(results, result{mode, seconds, check.Content, check.Visual, check.Issues})
		dir := os.Getenv("TEST_SPEED_OUTPUT")
		if dir == "" {
			dir = ".."
		}
		filename := os.Getenv("TEST_SPEED_FILENAME")
		if filename == "" {
			filename = "live-speed.json"
		}
		if e := os.WriteFile(filepath.Join(dir, filename), asJSON(map[string]any{"results": results, "review_model": s.cfg.ReviewModel, "vision_model": s.cfg.VisionModel, "scope": "same-page review scheduling; cold Office; not end-to-end"}), 0600); e != nil {
			t.Fatal(e)
		}
		if err != nil || !pageCheckPassed(check, false) {
			t.Fatalf("live %s incomplete (details in isolated log): %v, %+v", mode, err, check)
		}
		t.Logf("%s both actual reviewers checked: %.3fs", mode, seconds)
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before {
		t.Fatal("business accounting changed")
	}
}
