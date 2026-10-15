package main

import (
	"context"
	"testing"
)

// Pausing the model review must not silently drop the program checks the
// teacher-facing notice says are retained.
func TestPausedContentReviewKeepsDeterministicChecks(t *testing.T) {
	s := &Server{cfg: Config{CoursewareContentReviewDisabled: true}}
	wrong := Deck{Slides: []Slide{{Title: "浮力计算", Notes: "按题设计算：800×10×0.001=9 N。"}}}
	issues, status, err := s.reviewTeachingPage(context.Background(), Actor{}, "", TaskInput{}, wrong)
	if err != nil || status != "disabled" || blockingPageIssues(issues) == "" {
		t.Fatal("wrong arithmetic escaped while the model review was paused", issues, status, err)
	}
	right := Deck{Slides: []Slide{{Title: "浮力计算", Notes: "按题设计算：800×10×0.001=8 N。"}}}
	issues, status, err = s.reviewTeachingPage(context.Background(), Actor{}, "", TaskInput{}, right)
	if err != nil || status != "disabled" || blockingPageIssues(issues) != "" {
		t.Fatal("correct page was blocked or reported as model-reviewed", issues, status, err)
	}
}

// An unconfigured visual reviewer makes no provider call and therefore must
// not touch the persisted call budget (a nil pool would panic if it did).
func TestUnconfiguredVisualReviewSkipsBudget(t *testing.T) {
	s := &Server{cfg: Config{Mode: "live"}}
	issues, status, err := s.reviewPPTPagePNGOnce(context.Background(), Actor{TenantID: 1}, "task", Deck{}, nil, "")
	if err != nil || status != "unavailable" || len(issues) != 0 {
		t.Fatal("unconfigured visual review should be unavailable without a budget reservation", issues, status, err)
	}
}
