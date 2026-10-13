package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCoursewareReviewResponsibilities(t *testing.T) {
	for _, tc := range []struct {
		name, scope, fields string
		invalid, blocked    bool
	}{
		{"content-answer", "content", `"category":"answer","certainty":"confirmed"`, false, true},
		{"content-meaningless-chart", "content", `"category":"pedagogy","certainty":"confirmed"`, false, true},
		{"visual-cannot-recalculate", "visual", `"category":"answer","certainty":"confirmed"`, true, false},
		{"content-cannot-judge-pixels", "content", `"category":"layout","certainty":"confirmed"`, true, false},
		{"uncertain-coordinate", "visual", `"category":"render_mismatch","certainty":"uncertain","location":"右侧图红点","observed":"看起来约1.8","expected":"坐标(2,0)"`, false, false},
		{"actual-wrong-label", "visual", `"category":"render_mismatch","certainty":"confirmed","location":"右侧图红点旁文字","observed":"标签(3,0)","expected":"坐标(2,0)","input_path":"/slides/0/graph/series/1/mark_x/1","observed_value":3`, false, true},
		{"actual-clipping", "visual", `"category":"layout","certainty":"confirmed","location":"右侧图例","observed":"函数式尾部被裁切","expected":"完整显示y=2x+1"`, false, true},
		{"empty-colored-panel", "visual", `"category":"empty_panel","certainty":"confirmed","location":"下方蓝色框","observed":"没有文字、图形或数据","expected":"显示该区域计划的结论"`, false, true},
		{"missing-location", "visual", `"category":"layout","certainty":"confirmed","observed":"裁切","expected":"完整"`, true, false},
		{"no-expected-content", "visual", `"category":"render_mismatch","certainty":"confirmed","location":"图例","observed":"y=3x+1"`, true, false},
		{"no-category", "visual", `"certainty":"confirmed"`, true, false},
		{"unknown-certainty", "content", `"category":"knowledge","certainty":"likely"`, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := `{"issues":[{"object":"当前页对象","evidence":"具体问题依据","severity":"error","repair":"只修复该对象",` + tc.fields + `}]}`
			issues, err := parsePageReview(raw, tc.scope)
			if (err != nil) != tc.invalid {
				t.Fatalf("invalid=%v, err=%v", tc.invalid, err)
			}
			if err == nil && (blockingPageIssues(issues) != "") != tc.blocked {
				t.Fatalf("blocked=%v, issues=%+v", tc.blocked, issues)
			}
		})
	}
}

func TestVisualReviewCannotInventExpectedDisplay(t *testing.T) {
	d := Deck{Slides: []Slide{{Formula: "y=2x+1", Notes: "隐藏答案", Chart: map[string]any{"labels": []string{"A", "B", "C"}, "values": []float64{3, 7, 4}}, Graph: &FunctionGraph{Series: []FunctionSeries{{K: 2, B: 1, MarkX: []float64{0, 1}}}}}}}
	for _, tc := range []struct {
		name, path, observed string
		invalid, blocked     bool
	}{
		{"correct-values-semantic-objection", "/slides/0/chart/values", "[3,7,4]", false, false},
		{"actual-chart-value-error", "/slides/0/chart/values", "[3,6,4]", false, true},
		{"correct-point-with-optional-label", "/slides/0/graph/series/0/mark_x/1", "1.0", false, false},
		{"actual-wrong-point", "/slides/0/graph/series/0/mark_x/1", "2", false, true},
		{"actual-wrong-point-y", "/graph_points/1/y", "2.6", false, true},
		{"correct-computed-point-y", "/graph_points/1/y", "3", false, false},
		{"actual-wrong-formula", "/slides/0/formula", `"y=3x+1"`, false, true},
		{"formula-spacing-is-not-difference", "/slides/0/formula", `"y = 2x + 1"`, false, false},
		{"invented-expected-point-label", "/slides/0/graph/point_labels/0", `""`, true, false},
		{"hidden-answer", "/slides/0/notes", `""`, true, false},
		{"other-page", "/slides/1/formula", `"y=3x+1"`, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issue := PageQualityIssue{Reviewer: "visual", Category: "render_mismatch", Severity: "error", InputPath: tc.path, ObservedValue: json.RawMessage(tc.observed), Expected: "审核模型自行提出的新答案"}
			issues, err := reconcileVisualDisplayIssues([]PageQualityIssue{issue}, d)
			if (err != nil) != tc.invalid {
				t.Fatal("wrong validation", err)
			}
			if err == nil && ((blockingPageIssues(issues) != "") != tc.blocked || issues[0].Expected == issue.Expected) {
				t.Fatal("display review replaced authoritative input", issues)
			}
		})
	}
}

func TestVisualFormulaTypographyKeepsMathematicalContent(t *testing.T) {
	d := Deck{Slides: []Slide{{Formula: `F_{浮}=\\rho_{液}gV_{排}=20\\mathrm{N}`}}}
	for _, tc := range []struct {
		observed string
		blocked  bool
	}{
		{`F_{浮} = \rho_{液}gV_{排} = 20N`, false},
		{`F浮 = ρ液gV排 = 20N`, false},
		{`F_浮 = ρ_液gV_排 = 20N`, false},
		{`F浮 = ρ液gV排 = 30N`, true},
		{`F_{浮} = \rho_{液}gV_{排} = 30N`, true},
	} {
		issue := PageQualityIssue{Category: "render_mismatch", Severity: "error", InputPath: "/slides/0/formula", ObservedValue: asJSON(tc.observed)}
		issues, err := reconcileVisualDisplayIssues([]PageQualityIssue{issue}, d)
		if err != nil || (blockingPageIssues(issues) != "") != tc.blocked {
			t.Fatal("typography comparison changed content", err, issues)
		}
	}
}

func TestVisualReviewExcludesHiddenContentWithoutChangingDeck(t *testing.T) {
	d := Deck{Title: "函数", Lesson: "隐藏教案", Evaluation: "旧评审", Plan: &TeachingPlan{}, Slides: []Slide{{Title: "函数图", Notes: "隐藏讲解", Answer: "隐藏答案", Formula: "y=2x+1", Graph: &FunctionGraph{Series: []FunctionSeries{{K: 2, B: 1}}}}}}
	before := string(asJSON(d))
	visible := string(asJSON(pageForVisualReview(d)))
	if string(asJSON(d)) != before {
		t.Fatal("visual projection changed original lesson or answer")
	}
	for _, hidden := range []string{"隐藏教案", "隐藏讲解", "隐藏答案", "旧评审", `"plan"`} {
		if strings.Contains(visible, hidden) {
			t.Fatal("visual review received hidden content", hidden)
		}
	}
	if !strings.Contains(visible, "y=2x+1") || !strings.Contains(visible, `"graph"`) {
		t.Fatal("visual review lost displayed formula/graph")
	}
}
