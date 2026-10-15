package main

import (
	"context"
	"encoding/json"
	"testing"
)

func intersectionDisclosureCase(t *testing.T) teachingRegressionCase {
	t.Helper()
	for _, tc := range teachingRegressionCases(t) {
		if tc.ID == "intersection-coordinate-focus-disclosure" {
			return tc
		}
	}
	t.Fatal("missing original disclosure regression")
	return teachingRegressionCase{}
}

func TestCompletingSquareExerciseCannotShowDerivedFormula(t *testing.T) {
	var tc teachingRegressionCase
	for _, candidate := range teachingRegressionCases(t) {
		if candidate.ID == "completing-square-formula-disclosure" {
			tc = candidate
		}
	}
	if tc.Bad == nil {
		t.Fatal("missing actual quadratic exercise")
	}
	before := string(asJSON(tc.Bad))
	issues, status, err := (&Server{}).reviewTeachingPage(context.Background(), Actor{}, "", tc.Input, *tc.Bad)
	if err != nil || status != "checked" || blockingPageIssues(issues) == "" {
		t.Fatal("visible formula reveals the completing-square result", issues, status, err)
	}
	if before != string(asJSON(tc.Bad)) {
		t.Fatal("checking changed original exercise")
	}
	if err := coursewarePageIssue(*tc.Bad, 1, nil); err == nil {
		t.Fatal("generation preflight accepted the disclosed formula")
	}
	for _, test := range []struct {
		name   string
		change func(*Slide)
	}{
		{"only original formula", func(s *Slide) { s.Formula = "y=x^2-2x-3" }},
		{"worked example", func(s *Slide) { s.Layout = "example" }},
		{"given vertex form", func(s *Slide) { s.Bullets = []string{"已知y=x²-2x-3=(x-1)²-4，说明顶点式的意义"} }},
		{"asked to verify given identity", func(s *Slide) { s.Bullets = []string{"验证y=x²-2x-3=(x-1)²-4中的恒等关系"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			var d Deck
			json.Unmarshal(asJSON(tc.Bad), &d)
			test.change(&d.Slides[0])
			if got := exerciseVisibilityIssues(d); len(got) > 0 {
				t.Fatal("valid control rejected", got)
			}
		})
	}
	for _, control := range teachingRegressionCases(t) {
		if got := exerciseVisibilityIssues(control.Good); len(got) > 0 {
			t.Error("correct control rejected", control.ID, got)
		}
	}
}

func TestReadGraphFocusCannotRevealRequestedCoordinate(t *testing.T) {
	tc := intersectionDisclosureCase(t)
	before := string(asJSON(tc.Bad))
	// The actual review entry must catch this even without a configured model.
	issues, status, err := (&Server{}).reviewTeachingPage(context.Background(), Actor{}, "", tc.Input, *tc.Bad)
	if err != nil || status != "checked" || blockingPageIssues(issues) == "" {
		t.Fatal("requested intersection coordinate disclosed before answer reveal", issues, status, err)
	}
	if before != string(asJSON(tc.Bad)) {
		t.Fatal("checker changed original page")
	}
	if err := coursewarePageIssue(*tc.Bad, 1, nil); err == nil {
		t.Fatal("generation preflight accepted disclosed answer")
	}
	for _, control := range teachingRegressionCases(t) {
		if got := exerciseVisibilityIssues(control.Good); len(got) != 0 {
			t.Error("correct control rejected", control.ID, got)
		}
	}
	for _, test := range []struct {
		name   string
		change func(*Slide)
	}{
		{"example explanation", func(s *Slide) { s.Layout = "example" }},
		{"given intersection, ask meaning", func(s *Slide) { s.Bullets = []string{"已知交点为(1/3,5/3)，解释两个函数值的关系"} }},
		{"coordinate in original premise", func(s *Slide) { s.Bullets = append(s.Bullets, "已知点P(1/3,5/3)，读出它的坐标") }},
		{"teacher-only coordinate", func(s *Slide) { s.Focus = "交点处两个函数值有什么关系？" }},
		{"unrelated point", func(s *Slide) { s.Focus = "交点(0,1)处两个函数值有什么关系？" }},
		{"nonlinear graph", func(s *Slide) { s.Graph.Series[0].Kind = "quadratic"; s.Graph.Series[0].A = 1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			var d Deck
			json.Unmarshal(asJSON(tc.Bad), &d)
			test.change(&d.Slides[0])
			if got := exerciseVisibilityIssues(d); len(got) > 0 {
				t.Fatal("ambiguous/valid task must stay model-reviewed", got)
			}
		})
	}
	if _, status, err := (&Server{}).reviewTeachingPage(context.Background(), Actor{}, "", tc.Input, tc.Good); err != nil || status != "unavailable" {
		t.Fatal("correct control bypassed independent review", status, err)
	}
}
