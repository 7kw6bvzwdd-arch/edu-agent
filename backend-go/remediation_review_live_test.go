package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Explicit paid opt-in. Same known history pages, actual PPT screenshots, warm
// screenshot cache for all policies. This is not generated-deck or classroom acceptance.
func TestRemediationReviewPoliciesLive(t *testing.T) {
	if os.Getenv("TEST_REMEDIATION_REVIEW_LIVE") != "1" {
		t.Skip("explicit isolated paid review opt-in required")
	}
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	id := worksFixture(t, s, token, "courseware", "succeeded", "同页分级审核测速", TaskInput{})
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	s.cfg.Mode = "live"
	s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = os.Getenv("COURSEWARE_LLM_BASE_URL"), os.Getenv("COURSEWARE_LLM_API_KEY"), os.Getenv("COURSEWARE_LLM_MODEL")
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = os.Getenv("EVAL_LLM_BASE_URL"), os.Getenv("EVAL_LLM_API_KEY"), os.Getenv("EVAL_LLM_MODEL")
	s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = os.Getenv("VISION_LLM_BASE_URL"), os.Getenv("VISION_LLM_API_KEY"), os.Getenv("VISION_LLM_MODEL")
	s.cfg.CoursewareContentReviewDisabled = false
	if !s.coursewareQualityConfigured() {
		t.Fatal("independent reviewer and vision configuration required")
	}
	before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	in := TaskInput{Topic: "辛亥革命", Subject: "历史", Grade: "八年级"}
	pages := []Slide{
		{ID: "intro", Title: "从问题出发", Layout: "concept", Bullets: []string{"观察材料中的制度变化", "用自己的话描述观察所得"}},
		{ID: "fact", Title: "史实与制度变化", Layout: "concept", Bullets: []string{"1911年10月10日，武昌起义爆发", "1912年1月1日，中华民国临时政府在南京成立"}, Notes: "区分起义与政府成立的时间，不把二者混作同一事件。"},
		{ID: "task", Title: "比较任务", Layout: "exercise", Bullets: []string{"比较君主专制与民主共和的国家元首产生方式"}, Answer: "君主专制下通常依世袭继承；民主共和强调经法定选举或代表程序产生。"},
	}
	ctx := withCoursewareArtifacts(context.Background())
	decks := []Deck{}
	pptxs := [][]byte{}
	for _, sl := range pages {
		d := Deck{Title: in.Topic, Theme: "ocean", Slides: []Slide{sl}}
		p, err := s.renderCoursewarePage(ctx, &d, 1)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.coursewarePagePNG(ctx, a, id, p); err != nil {
			t.Fatal(err)
		}
		decks = append(decks, d)
		pptxs = append(pptxs, p)
	}
	rows := []map[string]any{}
	for _, policy := range []string{"none", "key", "all"} {
		s.cfg.CoursewareReviewPolicy = policy
		start := time.Now()
		checked, unchecked := 0, 0
		for i, d := range decks {
			c, err := s.inspectGeneratedPage(ctx, a, id, in, d, pptxs[i])
			if err != nil {
				t.Fatalf("%s page %d: %v", policy, i+1, err)
			}
			if c.Content == "checked" {
				checked++
			} else {
				unchecked++
			}
			if !s.pageCheckPassed(c, false, d.Slides[0]) {
				t.Fatalf("%s page %d rejected: %+v", policy, i+1, c)
			}
		}
		rows = append(rows, map[string]any{"policy": policy, "seconds": time.Since(start).Seconds(), "content_checked": checked, "content_unchecked": unchecked, "visual_checked": len(decks)})
	}
	s.cfg.CoursewareReviewPolicy = "key"
	for _, tc := range []struct{ subject, text string }{
		{"历史", "史实结论：辛亥革命发生于1840年，建立了中华人民共和国。"},
		{"语文", "原文引句：《静夜思》是杜甫的作品，首句是白日依山尽。"},
		{"化学", "实验安全结论：稀释浓硫酸时应把水快速倒入浓硫酸中，并用手握住试管口。"},
	} {
		d := Deck{Title: tc.subject, Slides: []Slide{{Title: tc.subject + "核验", Layout: "concept", Bullets: []string{tc.text}}}}
		issues, status, err := s.reviewTeachingPage(ctx, a, id, TaskInput{Subject: tc.subject, Topic: tc.subject}, d)
		if err != nil || status != "checked" || blockingPageIssues(issues) == "" {
			t.Fatalf("%s known defect missed: %s %v %+v", tc.subject, status, err, issues)
		}
	}
	after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	if after["balance"] != before["balance"] || after["reserved"] != before["reserved"] {
		t.Fatal("review benchmark charged business points")
	}
	out := filepath.Join("..", "work", "release-verification", "review-policies-live.json")
	if err := os.MkdirAll(filepath.Dir(out), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, asJSON(map[string]any{"scope": "same known history pages; warm screenshot cache; paid reviewers; no generation or classroom claim", "rows": rows, "known_defects_blocked": []string{"历史", "语文", "化学"}}), 0600); err != nil {
		t.Fatal(err)
	}
}
