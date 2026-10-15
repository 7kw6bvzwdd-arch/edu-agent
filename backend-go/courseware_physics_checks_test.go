package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTeachingPhysicsKnownConditionsDoNotDependOnReviewer(t *testing.T) {
	wanted := map[string]bool{"brightness-cannot-prove-wiring": true, "voltmeter-range": true, "force-meter-range": true, "resistor-power-for-real-experiment": true}
	for _, tc := range teachingRegressionCases(t) {
		if !wanted[tc.ID] {
			continue
		}
		t.Run(tc.ID, func(t *testing.T) {
			s := &Server{}
			before := string(asJSON(tc.Bad))
			issues, status, err := s.reviewTeachingPage(context.Background(), Actor{}, "", tc.Input, *tc.Bad)
			if err != nil || status != "checked" || blockingPageIssues(issues) == "" {
				t.Fatal("explicit condition defect escaped deterministic review", issues, status, err)
			}
			if before != string(asJSON(tc.Bad)) {
				t.Fatal("checker rewrote the saved page")
			}
			if issues, status, err = s.reviewTeachingPage(context.Background(), Actor{}, "", tc.Input, tc.Good); err != nil || status != "unavailable" || len(issues) != 0 {
				t.Fatal("correct control rejected or content review bypassed", issues, status, err)
			}
		})
	}
}

func TestBuoyancyEquilibriumDoesNotProveImmersion(t *testing.T) {
	var tc teachingRegressionCase
	for _, candidate := range teachingRegressionCases(t) {
		if candidate.ID == "buoyancy-equilibrium-does-not-imply-immersion" {
			tc = candidate
		}
	}
	if tc.Bad == nil {
		t.Fatal("missing actual reviewed defect")
	}
	before := string(asJSON(tc.Bad))
	issues, status, err := (&Server{}).reviewTeachingPage(context.Background(), Actor{}, "", tc.Input, *tc.Bad)
	if err != nil || status != "checked" || blockingPageIssues(issues) == "" {
		t.Fatal("force balance wrongly certified immersion", issues, status, err)
	}
	if before != string(asJSON(tc.Bad)) {
		t.Fatal("checker mutated original")
	}
	for _, notes := range []string{
		"静止且只受重力和浮力，F浮=G，但不能由此推出漂浮、V排=V物。",
		"全浸没时V排=V物；悬浮时F浮=G。",
		"漂浮时F浮=G，且V排<V物。",
		"错误说法：可由F浮=G推出漂浮、V排=V物。纠正：平衡不能区分漂浮与悬浮。",
	} {
		if got := teachingPhysicsConditionIssues(Deck{Slides: []Slide{{Notes: notes}}}); len(got) > 0 {
			t.Error("valid or negated statement rejected", notes, got)
		}
	}
	if got := teachingPhysicsConditionIssues(tc.Good); len(got) > 0 {
		t.Fatal("corrected control rejected", got)
	}
}

func TestControlledNullResultIsValidHypothesisTest(t *testing.T) {
	d := Deck{Slides: []Slide{{Notes: "全浸没后下移，V排与液体密度不变，观察F示是否变化。", Table: &TeachingTable{Headers: []string{"猜想的变量", "怎样改变它", "保持不变", "观察什么"}, Rows: [][]string{{"浸入深度", "全浸没后下移", "V排、液体密度", "F示是否变化"}}}}}}
	issue := PageQualityIssue{Object: "全浸没后下移", Category: "knowledge", Certainty: "confirmed", Severity: "error", Evidence: "排液体积V排与液体密度均不变，浮力不变，弹簧测力计示数也不会变化，该行无法起到猜想验证作用。", Repair: "提前给出深度没有影响的结论。"}
	if _, err := reconcileTeachingContentIssues([]PageQualityIssue{issue}, d); err == nil {
		t.Fatal("unchanged outcome wrongly invalidated a controlled experiment")
	}
	issue.Evidence = "未控制液体密度，无法将结果归因于深度。"
	if _, err := reconcileTeachingContentIssues([]PageQualityIssue{issue}, d); err != nil {
		t.Fatal("genuine variable-control objection suppressed", err)
	}
}

func TestTeachingReviewWrongCertaintyIsARecheckableContract(t *testing.T) {
	raw := `{"issues":[{"object":"正确练习","category":"conditions","certainty":"suggestion","severity":"suggestion","evidence":"可选的教学表达建议","repair":"保留开放作图任务"}]}`
	_, err := parsePageReview(raw, "content")
	var contract pageReviewContractError
	if !errors.As(err, &contract) {
		t.Fatal("wrong certainty bypassed the bounded same-page contract recheck", err)
	}
}

func TestTeachingPhysicsControlsAndReviewedDecks(t *testing.T) {
	for _, tc := range teachingRegressionCases(t) {
		if issues := teachingPhysicsConditionIssues(tc.Good); len(issues) > 0 {
			t.Error("correct control rejected", tc.ID, issues)
		}
	}
	root := os.Getenv("TEST_TEACHING_ARITHMETIC_DECKS")
	if root == "" {
		return
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
			t.Fatal("missing manually reviewed deck", name, err)
		}
		for _, sl := range saved.Deck.Slides {
			if issues := teachingPhysicsConditionIssues(Deck{Plan: saved.Deck.Plan, Slides: []Slide{sl}}); len(issues) > 0 {
				t.Error("manually reviewed page rejected", name, sl.Order, issues)
			}
		}
		pages += len(saved.Deck.Slides)
	}
	t.Logf("Physics guard replayed %d manually revised pages; not a model-success benchmark", pages)
}

func TestTeachingCircuitCalculationsFollowActualConnection(t *testing.T) {
	for _, tc := range []struct {
		connection       string
		closed           bool
		total            float64
		currents, powers []float64
	}{
		{"series", true, .2, []float64{.2, .2}, []float64{.4, .8}},
		{"parallel", true, .9, []float64{.6, .3}, []float64{3.6, 1.8}},
		{"parallel", false, 0, []float64{0, 0}, []float64{0, 0}},
	} {
		sl := Slide{Scientific: &ScientificDiagram{Kind: "circuit", Title: "按题设计算", Voltage: 6, Resistances: []float64{10, 20}, Connection: tc.connection, Closed: &tc.closed}}
		before := string(asJSON(sl))
		values := calculateTeachingCircuit(sl)
		if values == nil || math.Abs(values.Current-tc.total) > 1e-10 {
			t.Fatal("wrong total", values)
		}
		for i := range tc.currents {
			if math.Abs(values.Currents[i]-tc.currents[i]) > 1e-10 || math.Abs(values.Powers[i]-tc.powers[i]) > 1e-10 {
				t.Fatal("wrong branch result", values)
			}
		}
		if before != string(asJSON(sl)) || !strings.Contains(teachingRenderEvidence(Deck{Slides: []Slide{sl}}), "total_current_A") {
			t.Fatal("calculation mutated page or did not reach reviewer")
		}
	}
	if calculateTeachingCircuit(Slide{Scientific: &ScientificDiagram{Kind: "circuit", Title: "非法短路", Voltage: 6, Resistances: []float64{0}, Connection: "parallel"}}) != nil {
		t.Fatal("unsupported circuit certified")
	}
}

func TestTeachingPhysicsConditionBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		slide Slide
		bad   bool
	}{
		{"equal voltage range", Slide{Title: "并联电路", Notes: "演示要点：电源6 V，两灯并联。电压表量程选0—6 V。"}, false},
		{"too small voltage range", Slide{Title: "并联电路", Notes: "演示要点：电源6 V，两灯并联。电压表量程选0—3 V。"}, true},
		{"negative range instruction", Slide{Title: "并联电路", Notes: "演示要点：电源6 V，两灯并联。电压表不能使用0—3 V量程，先选0—15 V。"}, false},
		{"unknown measured fraction in series", Slide{Title: "串联电路", Notes: "演示要点：电源6 V。电压表测某段电压，量程选0—3 V。"}, false},
		{"ambiguous supply", Slide{Title: "并联电路", Notes: "演示要点：电源6 V，另一电源2 V。电压表量程选0—3 V。"}, false},
		{"range judgement exercise", Slide{Title: "并联电路", Layout: "exercise", Bullets: []string{"电源6 V，两灯并联，电压表量程选0—3 V，判断实验方案是否合理。"}, Answer: "不合理，量程不足。"}, false},
		{"force division is not range", Slide{Title: "实验方案", Notes: "重27 N，空气中称重。测力计分度值0.2 N，量程0—30 N。"}, false},
		{"only submerged load", Slide{Title: "实验方案", Notes: "重27 N，完全浸没时示数2 N。测力计量程0—5 N。"}, false},
		{"missing wiring", Slide{Layout: "exercise", Bullets: []string{"d.两盏灯同时点亮"}, Answer: "d并联"}, true},
		{"known wiring", Slide{Layout: "exercise", Bullets: []string{"d.两盏灯同时点亮，各灯两端分别接到电源两极"}, Answer: "d并联"}, false},
		{"information insufficient is valid", Slide{Layout: "exercise", Bullets: []string{"d.两盏灯同时点亮"}, Answer: "d信息不足，无法判断"}, false},
		{"focus supplies wiring", Slide{Layout: "exercise", Focus: "两灯连接在电源相同两个节点", Bullets: []string{"d.两盏灯同时点亮"}, Answer: "d并联"}, false},
		{"ambiguous repeated labels", Slide{Layout: "exercise", Bullets: []string{"d.两盏灯同时点亮", "d.两盏灯分别接到电源两极"}, Answer: "d并联"}, false},
		{"independent behavior provided", Slide{Layout: "exercise", Bullets: []string{"d.两盏灯同时点亮，一盏灯灭后另一盏仍亮"}, Answer: "d并联"}, false},
		{"reference diagram needs review", Slide{Layout: "exercise", Bullets: []string{"d.两盏灯同时点亮，电路图见前页"}, Answer: "d并联"}, false},
		{"measured current paths need review", Slide{Layout: "exercise", Bullets: []string{"d.两盏灯同时点亮，干路电流等于两灯电流之和"}, Answer: "d并联"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if issues := teachingPhysicsConditionIssues(Deck{Slides: []Slide{tc.slide}}); (len(issues) > 0) != tc.bad {
				t.Fatal("wrong boundary verdict", issues)
			}
		})
	}
	for _, tc := range []struct {
		name, notes string
		closed      bool
		bad         bool
	}{
		{"real overloaded experiment", "课堂真实实验：6 V电源，两个电阻额定功率均0.25 W。", true, true},
		{"adequate rating", "课堂真实实验：6 V电源，两个电阻额定功率均5 W。", true, false},
		{"hypothetical experiment", "理想计算，不要求实际接线。若用于真实实验，两个电阻额定功率均0.25 W是否足够？", true, false},
		{"limited source changes operating point", "课堂真实实验：电源限流至0.05 A，两个电阻额定功率均0.25 W。", true, false},
		{"individual rating unmapped", "课堂真实实验：额定功率0.25 W。", true, false},
		{"open circuit", "课堂真实实验：两个电阻额定功率均0.25 W。", false, false},
		{"different experimental source", "课堂真实实验：用1 V学生电源，两个电阻额定功率均0.25 W。", true, false},
		{"different experimental resistors", "课堂真实实验：6 V电源，1000 Ω与2000 Ω两个电阻额定功率均0.25 W。", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sl := Slide{Notes: tc.notes, Scientific: &ScientificDiagram{Kind: "circuit", Title: "计算示意", Voltage: 6, Resistances: []float64{10, 20}, Connection: "parallel", Closed: &tc.closed}}
			if issues := teachingPhysicsConditionIssues(Deck{Slides: []Slide{sl}}); (len(issues) > 0) != tc.bad {
				t.Fatal("wrong power verdict", issues)
			}
		})
	}
	for _, tc := range []struct {
		conventions string
		bad         bool
	}{
		{"用电流路径判断串联和并联，答案不能倒填题设。", true},
		{"所有电路均为并联，各灯两端连接在电源相同两个节点。", false},
	} {
		d := Deck{Plan: &TeachingPlan{Conventions: tc.conventions}, Slides: []Slide{{Layout: "exercise", Bullets: []string{"d.两盏灯同时点亮"}, Answer: "d并联"}}}
		if issues := teachingPhysicsConditionIssues(d); (len(issues) > 0) != tc.bad {
			t.Fatal("generic convention confused with wiring evidence", tc.conventions, issues)
		}
	}
}

func TestBuoyancySharesWeightNotRemainingTension(t *testing.T) {
	for _, tc := range []struct {
		notes string
		bad   bool
	}{
		{"静止三力平衡：G=27 N，F示=17 N，F浮=10 N。浮力分担了17 N中的10 N。", true},
		{"静止三力平衡：G=27 N，F示=17 N，F浮=10 N。浮力分担了27 N重力中的10 N。", false},
		{"静止三力平衡：G=27 N。错误说法：浮力分担了17 N中的10 N；应是分担重力。", false},
		{"G=27 N，另一物体G=17 N。浮力分担了17 N中的10 N。", false},
		{"没有重力值。浮力分担了17 N中的10 N。", false},
	} {
		if issues := teachingPhysicsConditionIssues(Deck{Slides: []Slide{{Notes: tc.notes}}}); (len(issues) > 0) != tc.bad {
			t.Fatal(tc, issues)
		}
	}
}
