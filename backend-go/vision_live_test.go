package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Explicit opt-in: four real screenshot reviews of synthetic PPT fixtures in
// an isolated database. Never creates real user works or settles business credits.
func TestLiveVisualReviewKnownDefects(t *testing.T) {
	if os.Getenv("TEST_LIVE_VISUAL_REVIEW") != "1" {
		t.Skip("explicit live visual reviewer opt-in required")
	}
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	id := worksFixture(t, s, token, "courseware", "succeeded", "截图审核接入测试", TaskInput{})
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	s.cfg.Mode = "live"
	s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = os.Getenv("VISION_LLM_BASE_URL"), os.Getenv("VISION_LLM_API_KEY"), os.Getenv("VISION_LLM_MODEL")
	dir := "../work/product-improvements/vision-fixtures"
	raw, err := os.ReadFile(filepath.Join(dir, "decks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var full Deck
	if err = json.Unmarshal(raw, &full); err != nil {
		t.Fatal(err)
	}
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	results := []map[string]any{}
	for index, tc := range []struct {
		name string
		bad  bool
	}{
		{"correct-math", false}, {"correct-physics", false}, {"empty-colored-panels", true}, {"meaningless-chart", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pptx, err := os.ReadFile(filepath.Join(dir, tc.name+".pptx"))
			if err != nil {
				t.Fatal(err)
			}
			d := full
			d.Slides = []Slide{full.Slides[index]}
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			issues, status, err := s.reviewPPTPage(ctx, a, id, d, pptx)
			blocked := blockingPageIssues(issues) != ""
			results = append(results, map[string]any{"case": tc.name, "status": status, "blocked": blocked, "expected_blocked": tc.bad, "issues": issues, "available": err == nil})
			if err != nil || status != "checked" {
				t.Fatal("actual screenshot review unavailable", err)
			}
			if blocked != tc.bad {
				t.Fatal("missed known visual defect or blocked correct fixture", issues)
			}
		})
	}
	after := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	if before != after {
		t.Fatal("visual review changed business accounting")
	}
	report := map[string]any{"model": s.cfg.VisionModel, "results": results, "business_accounting_unchanged": before == after, "scope": "four known PPT fixtures only; not whole-deck quality acceptance"}
	if err := os.WriteFile("../work/product-improvements/vision-live-review.json", asJSON(report), 0600); err != nil {
		t.Fatal(err)
	}
}
