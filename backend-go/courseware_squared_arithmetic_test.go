package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Reproduced from the live quadratic deck, page 14, 2026-10-15.
// A stopped content model must not let an explicit false calculation be approved.
func TestPausedReviewBlocksSquaredArithmeticHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	in := TaskInput{Topic: "二次函数", PreviewFirst: true, Detail: "default", UseKB: true}
	id := worksFixture(t, s, token, "courseware", "awaiting_preview", in.Topic, in)
	route := "/api/tasks/" + id
	d := sampleDeck(TaskInput{Topic: "浮力", Pages: 8, Theme: "ocean"})
	d.Title = "二次函数"
	d.Slides = d.Slides[:3]
	assignSlideIDs(&d)
	d.Plan = &TeachingPlan{Summary: "核对判别式"}
	d.PageChecks = map[string]PageQualityCheck{}
	for i := range d.Slides {
		d.Slides[i].Order = i + 1
		d.Plan.Pages = append(d.Plan.Pages, TeachingPage{Position: i + 1, Goal: "核对计算", Task: "concept", Minutes: 3})
	}
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), "UPDATE tasks SET result=$1,mode='live' WHERE id=$2", asJSON(d), id)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	vision := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": `{"issues":[]}`}}}})
	}))
	defer vision.Close()
	s.cfg.Mode = "live"
	s.cfg.CoursewareContentReviewDisabled = true
	s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = vision.URL, "fixture", "vision"
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	var saved struct {
		Result   Deck
		Revision int
	}
	json.Unmarshal(worksCall(t, s, "GET", route, token, nil, 200).Body.Bytes(), &saved)
	for _, tc := range []struct {
		text string
		bad  bool
	}{
		{"主例：Δ=4²-4×1×(-3)=16，两个交点。", true},
		{"主例：Δ=(-2)²-4×1×(-3)=16，两个交点。", false},
	} {
		saved.Result.Slides[0].Bullets = []string{tc.text}
		saved.Result.Slides[0].Notes = "只核对题中完整的数值算式，修正此页不改其他页。"
		worksCall(t, s, "POST", route+"/draft", token, map[string]any{"result": saved.Result, "revision": saved.Revision}, 200)
		json.Unmarshal(worksCall(t, s, "GET", route, token, nil, 200).Body.Bytes(), &saved)
		sl := saved.Result.Slides[0]
		check := saved.Result.PageChecks[sl.ID]
		if (blockingPageIssues(check.Issues) != "") != tc.bad || check.Content != "disabled" {
			t.Fatal("squared calculation was not checked honestly", tc.text, check)
		}
		want := 200
		if tc.bad {
			want = 409
		}
		worksCall(t, s, "POST", route+"/slides/"+sl.ID+"/review", token, map[string]any{"revision": saved.Revision, "hash": slideContentHash(saved.Result, sl), "approved": true}, want)
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before {
		t.Fatal("deterministic verification changed quota")
	}
	if strings.Contains(worksCall(t, s, "GET", "/api/usage", token, nil, 200).Body.String(), "deepseek") {
		t.Fatal("disabled content model was invoked")
	}
}
