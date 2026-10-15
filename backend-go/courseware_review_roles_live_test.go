package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Known teaching fixtures, actual Office screenshots and independent paid reviewers.
// This tests the review boundary; it is not a GLM generated full-deck acceptance.
func TestCoursewareReviewRolesLive(t *testing.T) {
	if os.Getenv("TEST_LIVE_REVIEW_ROLES") != "1" {
		t.Skip("explicit isolated live review opt-in required")
	}
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	id := worksFixture(t, s, token, "courseware", "succeeded", "双审核职责抽测", TaskInput{})
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	s.cfg.Mode = "live"
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = os.Getenv("EVAL_LLM_BASE_URL"), os.Getenv("EVAL_LLM_API_KEY"), os.Getenv("EVAL_LLM_MODEL")
	s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = os.Getenv("VISION_LLM_BASE_URL"), os.Getenv("VISION_LLM_API_KEY"), os.Getenv("VISION_LLM_MODEL")
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	results := []map[string]any{}
	defer func() { os.WriteFile("../work/courseware-review-roles/live-review.json", asJSON(results), 0600) }()
	var fixtures Deck
	raw, err := os.ReadFile("../work/product-improvements/vision-fixtures/decks.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                  string
		index                 int
		contentBad, visualBad bool
	}{
		{"correct-math", 0, false, false}, {"correct-physics", 1, false, false},
		{"empty-colored-panels", 2, false, true}, {"meaningless-chart", 3, true, false},
		{"correct-function-coordinates", -1, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, stop := context.WithTimeout(withCoursewareArtifacts(context.Background()), 2*time.Minute)
			defer stop()
			d := fixtures
			var pptx []byte
			if tc.index >= 0 {
				d.Slides = []Slide{fixtures.Slides[tc.index]}
				pptx, err = os.ReadFile(filepath.Join("../work/product-improvements/vision-fixtures", tc.name+".pptx"))
			} else {
				d = Deck{Title: "一次函数", Theme: "ocean", EditableFormulas: true, Slides: []Slide{{Title: "斜率与截距", Layout: "example", Bullets: []string{"y=2x+1：k>0，b=1", "y=-x+2：k<0，b=2"}, Notes: "y=2x+1经过一、二、三象限，y=-x+2经过一、二、四象限。标记点由函数式计算：(0,1)、(1,3)、(0,2)、(2,0)。", Graph: &FunctionGraph{Title: "函数对比", XMin: -4, XMax: 4, YMin: -3, YMax: 5, Series: []FunctionSeries{{K: 2, B: 1, MarkX: []float64{0, 1}}, {K: -1, B: 2, MarkX: []float64{0, 2}}}}}}}
				pptx, err = s.renderCoursewarePage(ctx, &d, 1)
				if err == nil {
					os.WriteFile("../work/courseware-review-roles/coordinate-check.pptx", pptx, 0600)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			in := TaskInput{Subject: "数学", Grade: "八年级", Topic: d.Title}
			if tc.name == "correct-physics" {
				in.Subject = "物理"
			}
			content, cs, ce := s.reviewTeachingPage(ctx, a, id, in, d)
			visual, vs, ve := s.reviewPPTPage(ctx, a, id, d, pptx)
			results = append(results, map[string]any{"case": tc.name, "content_status": cs, "content_issues": content, "content_available": ce == nil, "visual_status": vs, "visual_issues": visual, "visual_available": ve == nil})
			if ce != nil || ve != nil || cs != "checked" || vs != "checked" {
				t.Fatalf("review unavailable: content=%v visual=%v", ce, ve)
			}
			if (blockingPageIssues(content) != "") != tc.contentBad || (blockingPageIssues(visual) != "") != tc.visualBad {
				t.Fatalf("wrong responsibility/verdict: content=%+v visual=%+v", content, visual)
			}
		})
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before {
		t.Fatal("reviews changed business points")
	}
}
