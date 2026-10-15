package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTeachingArithmeticBlocksWrongAnswerWithoutProvider(t *testing.T) {
	s := &Server{}
	d := Deck{Slides: []Slide{{Title: "浮力计算", Notes: "按题设计算：800×10×0.001=9 N。"}}}
	before := string(asJSON(d))
	issues, status, err := s.reviewTeachingPage(context.Background(), Actor{}, "", TaskInput{}, d)
	if err != nil || status != "checked" || blockingPageIssues(issues) == "" || !strings.Contains(issues[0].Evidence, "8") {
		t.Fatal("explicit wrong arithmetic escaped the deterministic review", issues, status, err)
	}
	if string(asJSON(d)) != before {
		t.Fatal("verification rewrote the saved page")
	}
	// A numeric check is not a replacement for the rest of the content audit.
	d.Slides[0].Notes = "按题设计算：800×10×0.001=8 N。"
	if _, status, err := s.reviewTeachingPage(context.Background(), Actor{}, "", TaskInput{}, d); err != nil || status != "unavailable" {
		t.Fatal("correct arithmetic bypassed unavailable content reviewer", status, err)
	}
}

func TestTeachingReviewerCannotIntroduceWrongArithmetic(t *testing.T) {
	d := Deck{Slides: []Slide{{Notes: "1 cm³=10⁻⁶ m³。"}}}
	for _, severity := range []string{"error", "suggestion"} {
		issue := PageQualityIssue{Object: "单位说明", Category: "answer", Severity: severity, Evidence: "倍率计算：10000/10=1000000。", Repair: "补充说明。"}
		if _, err := reconcileTeachingContentIssues([]PageQualityIssue{issue}, d); err == nil {
			t.Fatal("reviewer arithmetic was trusted", severity)
		}
		issue.Evidence = "倍率计算：10000/10=1000。"
		issue.Repair = "将结果改为800×10×0.001=9 N。"
		if _, err := reconcileTeachingContentIssues([]PageQualityIssue{issue}, d); err == nil {
			t.Fatal("repair arithmetic was trusted", severity)
		}
		issue.Repair = "补充说明。"
		if _, err := reconcileTeachingContentIssues([]PageQualityIssue{issue}, d); err != nil {
			t.Fatal("correct review rejected", err)
		}
	}
}

func TestTeachingVolumeGuardNeedsExplicitNumeratorFormula(t *testing.T) {
	for _, text := range []string{
		"密度ρ=m/V，若未换算，将cm³数值直接代入SI公式，结果缩小1000000倍。",
		"若未换算，将cm³数值直接代入SI公式，结果缩小1000000倍。",
	} {
		issues, _ := teachingVolumeErrorFactors(Deck{Slides: []Slide{{Notes: text}}})
		if len(issues) != 0 {
			t.Fatal("a formula without volume in the numerator was assumed to scale up", issues)
		}
	}
	d := Deck{Slides: []Slide{{Formula: "F浮=ρ液gV排²", Notes: "若未换算，将cm³数值直接代入SI公式，结果放大1000000000000倍。"}}}
	if issues, _ := teachingVolumeErrorFactors(d); len(issues) != 0 {
		t.Fatal("a powered volume term was treated as linear", issues)
	}
}

func TestTeachingArithmeticBoundaries(t *testing.T) {
	for _, tc := range []struct {
		text string
		bad  bool
	}{
		{"F=800×10×0.001=9 N。", true},
		{"倍率计算：10000/10=1000000。", true},
		{"6²/10=3.6 W。", false}, // Powers are not partially parsed as 10=3.6.
		{"2+3*4=20", true},
		{"(2+3)*4=20", false},
		{"−2×（3−1）=−4", false},
		{"0.1+0.2=0.3", false},
		{"0.1+0.2=0.4", true},
		{"1/2=0.5=1.5/3", false},
		{"1/2=0.5=0.6", true},
		{"1/3=0.33", false}, // Classroom recurring-decimal rounding is left alone.
		{"0.33=1/3", false},
		{"2/3≈0.67", false},
		{"50/100=50%", false},
		{"10^3=1000", false},
		{"100×10⁻⁶=0.0001", false},
		{"y=2x+1=7", false},
		{"2=4-2x", false},
		{"sin(0)=0", false},
		{"\\frac{10000}{10}=1000", false},
		{"2+2≠5", false},
		{"2+2<=5", false},
		{"1,000+2=1,002", false},
		{"1:2=0.5", false},
		{"2·3=6", false},
		{"错误示例：800×10×0.001=9 N；正确计算：800×10×0.001=8 N。", false},
		{"错误示例：800×10×0.001=9 N；正确计算：800×10×0.001=7 N。", true},
		{"错误示例：800×10×0.001=9 N，正确计算：800×10×0.001=7 N。", true},
		{"F=800 kg/m³×10 N/kg×0.001 m³=8 N", false},
	} {
		if issues := teachingArithmeticIssues(tc.text); (len(issues) > 0) != tc.bad {
			t.Errorf("%s: bad=%v, issues=%v", tc.text, tc.bad, issues)
		}
	}
}

func TestTeachingArithmeticReviewedDecks(t *testing.T) {
	root := os.Getenv("TEST_TEACHING_ARITHMETIC_DECKS")
	if root == "" {
		t.Skip("requires saved manually reviewed decks; not an automatic-generation benchmark")
	}
	pages := 0
	for _, name := range []string{"linear", "buoyancy", "quadratic", "circuits"} {
		raw, err := os.ReadFile(filepath.Join(root, name, "review-copy.json"))
		if err != nil {
			t.Fatal(err)
		}
		var saved struct {
			Deck Deck `json:"deck"`
		}
		if err := json.Unmarshal(raw, &saved); err != nil || len(saved.Deck.Slides) == 0 {
			t.Fatal("missing reviewed deck", name, err)
		}
		d := saved.Deck
		for _, sl := range d.Slides {
			if issues := teachingDeckArithmeticIssues(Deck{Slides: []Slide{sl}}); len(issues) > 0 {
				t.Error("manually reviewed page rejected", name, sl.Order, issues)
			}
		}
		pages += len(d.Slides)
	}
	t.Logf("Replayed %d manually reviewed pages; does not measure teaching correctness or model success rate", pages)
}

func TestTeachingArithmeticPreservesCorrectRegressionPages(t *testing.T) {
	for _, tc := range teachingRegressionCases(t) {
		if issues := teachingDeckArithmeticIssues(tc.Good); len(issues) > 0 {
			t.Error("correct control page rejected", tc.ID, issues)
		}
		if issues, _ := teachingVolumeErrorFactors(tc.Good); len(issues) > 0 {
			t.Error("correct unit explanation rejected", tc.ID, issues)
		}
		if tc.ID == "volume-error-factor" {
			if issues, _ := teachingVolumeErrorFactors(*tc.Bad); len(issues) == 0 {
				t.Error("observed buoyancy multiplier defect escaped guard")
			}
		}
	}
}

func TestFocusedTeachingEvidenceArithmeticIsNotTrusted(t *testing.T) {
	raw := `{"checks":[{"index":1,"evidence":"独立核对倍率：10000/10=1000000，所以结果正确无需改动。","issues":[]}]}`
	if _, err := parseFocusedTeachingReview(raw, 1); err == nil {
		t.Fatal("focused evidence with wrong arithmetic was accepted")
	}
}
