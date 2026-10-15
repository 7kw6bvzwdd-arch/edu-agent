package main

import (
	"os"
	"strings"
	"testing"
)

func loadLinearKFixture(t *testing.T) AnimScript {
	t.Helper()
	b, e := os.ReadFile("../fixtures/animation-v2-linear-k.json")
	if e != nil {
		t.Fatal(e)
	}
	sc, p := decodeAnimScript(b)
	if len(p) > 0 {
		t.Fatalf("fixture must validate: %v", p)
	}
	return sc
}

func f64(v float64) *float64 { return &v }

func expectProblem(t *testing.T, sc AnimScript, want string) {
	t.Helper()
	p := validateAnimScript(sc)
	if !strings.Contains(strings.Join(p, "\n"), want) {
		t.Fatalf("want problem containing %q, got %v", want, p)
	}
}

func TestAnimScriptRejectsBuildResultsAndCoordinates(t *testing.T) {
	for _, field := range []string{`"subtitle":"x"`, `"timeline":{}`, `"duration_ms":3000`, `"x":120`} {
		raw := `{"version":"anim-v2","title":"t","archetype":"func.params@1","representative":"s","scenes":[{"id":"s","intent":"i","title":"t",` + field + `,"objects":[],"narration":[],"actions":[],"holds":[]}]}`
		if _, p := decodeAnimScript([]byte(raw)); len(p) != 1 || !strings.Contains(p[0], "不允许的字段") {
			t.Fatalf("%s: got %v", field, p)
		}
	}
	sc := loadLinearKFixture(t)
	sc.Scenes[1].Objects[1].Params["x"] = 300
	expectProblem(t, sc, "不接受参数x")
}

// Steepness follows |k| regardless of sign; direction follows the sign.
func TestAnimSlopeNarrationUsesAbsoluteK(t *testing.T) {
	sc := loadLinearKFixture(t)
	neg := &sc.Scenes[2]
	// k from -3 to -1 increases k but flattens the line.
	neg.Objects[1].Params["k"] = -3
	neg.Actions[0].To = f64(-1)
	neg.Narration[0].Display = "令 k 从 -3 变到 -1。"
	expectProblem(t, sc, "变陡要求|k|增大")

	sc = loadLinearKFixture(t)
	sc.Scenes[3].Narration[1].Display = "所以 k = 1 的直线比 k = -3 的更陡。"
	expectProblem(t, sc, "陡缓比较只看|k|")

	sc = loadLinearKFixture(t)
	sc.Scenes[2].Narration[1].Display = "直线从左到右上升，|k| 变大，直线也变陡。"
	expectProblem(t, sc, "旁白称直线上升")
}

func TestAnimSlopeZeroCrossingMustBeExplained(t *testing.T) {
	sc := loadLinearKFixture(t)
	sc.Scenes[1].Actions[0].To = f64(-2)
	sc.Scenes[1].Narration[0].Display = "让 k 从 1 变到 -2。"
	sc.Scenes[1].Narration[1].Display = "直线绕 y 轴上的交点转动。"
	sc.Scenes[1].Narration[1].Speech = "直线绕着它与 y 轴的交点转动。"
	sc.Scenes[1].Narration[2].Display = "直线的方向发生了改变。"
	sc.Scenes[1].Narration[2].Speech = "直线的方向发生了改变。"
	sc.Scenes[1].Objects[2].Text = ""
	expectProblem(t, sc, "会经过k=0")
	sc.Scenes[1].Narration[1].Speech = "经过 k 等于 0 的瞬间，它不是一次函数。"
	for _, p := range validateAnimScript(sc) {
		if strings.Contains(p, "k=0") {
			t.Fatalf("explained crossing still rejected: %s", p)
		}
	}
	sc.Scenes[1].Objects[1].Params["k"] = 0
	expectProblem(t, sc, "k=0时不是一次函数")
}

func TestAnimNarrationMustMatchParameters(t *testing.T) {
	sc := loadLinearKFixture(t)
	sc.Scenes[1].Narration[0].Display = "让 k 从 1 增大到 4。"
	expectProblem(t, sc, "没有对应的参数变化")
	sc = loadLinearKFixture(t)
	sc.Scenes[0].Narration[0].Display = "这是一次函数 y = 2x + 1 的图像。"
	expectProblem(t, sc, "与本幕直线参数不一致")
	sc = loadLinearKFixture(t)
	sc.Scenes[1].Objects[2].Text = "(0, 2)"
	expectProblem(t, sc, "与直线上的实际点")
	sc = loadLinearKFixture(t)
	sc.Scenes[0].Narration[0].Speech = "y = x + 1"
	expectProblem(t, sc, "须转换为明确读法")
}

func TestAnimStructureGates(t *testing.T) {
	sc := loadLinearKFixture(t)
	sc.Representative = "intro"
	expectProblem(t, sc, "代表幕须")
	sc = loadLinearKFixture(t)
	sc.Scenes[3].Objects[0].Params["xmax"] = 8
	expectProblem(t, sc, "各幕坐标范围须一致")
	sc = loadLinearKFixture(t)
	sc.Scenes[0].Objects[2].Slot = "main"
	expectProblem(t, sc, "只能放在side槽位")
	sc = loadLinearKFixture(t)
	sc.Scenes[1].Actions[1].At = AnimTrigger{After: "a6"}
	expectProblem(t, sc, "after须引用本幕更早的动作")
}

func TestAnimTimelineFollowsAudioAndHolds(t *testing.T) {
	sc := loadLinearKFixture(t)
	audio := map[string]int{}
	for _, s := range sc.Scenes {
		for _, n := range s.Narration {
			audio[n.ID] = 4000
		}
	}
	tl, p := buildAnimTimeline(sc, audio)
	if len(p) > 0 {
		t.Fatal(p)
	}
	intro := tl.Scenes[0]
	if intro.Sentences[0].Start != 0 || intro.Sentences[0].End != 4000 || intro.Sentences[1].Start != 4000+animSentenceGapMS {
		t.Fatalf("sentence spans %+v", intro.Sentences)
	}
	if h := intro.Holds[0]; h.Start != intro.Sentences[1].End+animSentenceGapMS || h.End-h.Start != 1200 {
		t.Fatalf("hold must follow its sentence: %+v", h)
	}
	if a3 := intro.Actions[2]; a3.Start != intro.Actions[1].End {
		t.Fatalf("after trigger: %+v", intro.Actions)
	}
	for i := 1; i < len(tl.Scenes); i++ {
		if tl.Scenes[i].Start != tl.Scenes[i-1].End {
			t.Fatal("scenes must be contiguous")
		}
	}
	if tl.TotalMS != tl.Scenes[len(tl.Scenes)-1].End || len(tl.Warnings) > 0 {
		t.Fatalf("total %d warnings %v", tl.TotalMS, tl.Warnings)
	}
	// Longer real audio moves later content; the timeline never uses estimates when audio exists.
	audio["n1"] = 9000
	tl2, _ := buildAnimTimeline(sc, audio)
	if tl2.TotalMS-tl.TotalMS != 5000 {
		t.Fatalf("audio delta not applied: %d", tl2.TotalMS-tl.TotalMS)
	}
	delete(audio, "n2")
	if _, p := buildAnimTimeline(sc, audio); len(p) != 1 || !strings.Contains(p[0], "缺少实际音频时长") {
		t.Fatalf("missing audio: %v", p)
	}
	sc.Scenes[1].Actions = append(sc.Scenes[1].Actions, AnimAction{ID: "dup", Op: "tween", Object: "l1", Param: "k", To: f64(2), At: AnimTrigger{Sentence: "n3"}})
	audio["n2"] = 4000
	if _, p := buildAnimTimeline(sc, audio); len(p) != 1 || !strings.Contains(p[0], "时间重叠") {
		t.Fatalf("overlap: %v", p)
	}
}
