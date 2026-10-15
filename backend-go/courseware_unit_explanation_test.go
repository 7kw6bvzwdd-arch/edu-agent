package main

import (
	"strings"
	"testing"
)

func TestTeachingVolumeErrorFactorIsCalculated(t *testing.T) {
	for _, tc := range []struct {
		text string
		bad  bool
	}{
		{"1 cm³=10⁻⁶ m³，若未换算而将cm³数值直接代入该SI公式，结果小1000倍。", true},
		{"1 cm³=10⁻⁶ m³，若未换算而将cm³数值直接代入该SI公式，结果大1000000倍。", false},
		{"若未换算，mm^3数值直接代入SI公式，结果会放大1000000倍。", true},
		{"若未换算，mm³数值直接代入SI公式，结果放大十亿倍。", false},
		{"若未换算，cm³数值直接代入SI公式，结果放大百万倍。", false},
		{"长度用cm未换算直接代入SI公式，结果放大100倍。", false},
		{"若未换算，cm²数值直接代入SI公式，结果放大10000倍。", false},
		{"cm³到m³换算体积数值缩小1000000倍。", false},
		{"错误说法：若未换算，将cm³数值直接代入SI公式，结果缩小1000倍；实际会放大1000000倍。", false},
		{"cm³的浮力示例：物体下移，体积小1000倍。", false},
	} {
		d := Deck{Slides: []Slide{{Formula: `F_浮=\rho_{液}gV_{排}`, Notes: tc.text}}}
		before := string(asJSON(d))
		issues, _ := teachingVolumeErrorFactors(d)
		if (blockingPageIssues(issues) != "") != tc.bad || before != string(asJSON(d)) {
			t.Fatal(tc.text, issues)
		}
		if tc.bad && !strings.Contains(issues[0].Evidence, "倍") {
			t.Fatal("missing arithmetic evidence")
		}
	}
}

func TestTeachingCorrectVolumeStatementCannotBeRewritten(t *testing.T) {
	sentence := "若未换算而将cm³数值直接代入该SI公式，结果大1000000倍"
	d := Deck{Slides: []Slide{{Formula: `F_浮=\rho_{液}gV_{排}`, Notes: sentence + "。"}}}
	issue := PageQualityIssue{Object: sentence, Category: "answer", Severity: "error", Evidence: "这种说法不清楚，应该修改倍率"}
	if _, err := reconcileTeachingContentIssues([]PageQualityIssue{issue}, d); err == nil {
		t.Fatal("computed correct multiplier accepted as error")
	}
	issue.Object = "另一个数值计算 800×10×0.001=9 N"
	if _, err := reconcileTeachingContentIssues([]PageQualityIssue{issue}, d); err != nil {
		t.Fatal("unrelated error suppressed")
	}
	d.Slides[0].Notes = strings.ReplaceAll(d.Slides[0].Notes, "大1000000倍", "小1000倍")
	issue.Object = d.Slides[0].Notes
	if _, err := reconcileTeachingContentIssues([]PageQualityIssue{issue}, d); err != nil {
		t.Fatal("incorrect statement suppressed")
	}
}
