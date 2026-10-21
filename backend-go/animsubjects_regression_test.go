package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// Historical component validators remain covered for stored works and local revisions.
// New-model HTTP generation is exercised with authored HTML in direct_author_test.go.
func TestAnimationTextCueLegacyCompatibility(t *testing.T) {
	testLegacyAnimationPlan(t, "text-cues", TaskInput{Topic: "图形的相似点", Requirements: "借展示或高亮说明形状相似"}, "item_sentences")
}

func TestAnimationTextCompositionLegacyCompatibility(t *testing.T) {
	// Cued comparison/prompt components are valid alongside a separate graphic
	// representative. Do not turn composition advice into a panel-count veto.
	testLegacyAnimationPlan(t, "text-panels", TaskInput{Topic: "图形的相似点", Requirements: "借展示或高亮说明形状相似"}, "")
}

func TestAnimationSubjectMotionLegacyCompatibility(t *testing.T) {
	testLegacyAnimationPlan(t, "geography", TaskInput{Topic: "水循环：水从哪里来又到哪里去", Requirements: "至少一幕有非文字对象运动，代表幕观察水的路径"}, "当前代表幕")
}

func TestAnimationSubjectHighlightLegacyCompatibility(t *testing.T) {
	testLegacyAnimationPlan(t, "biology", TaskInput{Topic: "植物细胞结构与功能", Requirements: "按旁白依次高亮细胞壁、细胞膜、细胞核和叶绿体"}, "progress")
}

func TestAnimationPlanBudgetLegacyCompatibility(t *testing.T) {
	testLegacyAnimationPlan(t, "geography", TaskInput{Topic: "水循环", AnimationMinSeconds: 60, AnimationMaxSeconds: 90}, "预估")
}

func TestAnimationSubjectPartsLegacyCompatibility(t *testing.T) {
	testLegacyAnimationPlan(t, "science", TaskInput{Topic: "植物细胞结构", Requirements: "依次高亮细胞壁、细胞膜、细胞核和叶绿体"}, "cell_membrane")
}

func TestAnimationDrawingFeedbackLegacyCompatibility(t *testing.T) {
	testLegacyAnimationPlan(t, "invalid-scene", TaskInput{Topic: "观察物体位置"}, "font_size")
}

func TestAnimationTextFeedbackLegacyCompatibility(t *testing.T) {
	testLegacyAnimationPlan(t, "long-item", TaskInput{Topic: "英语位置表达"}, "第1条实际29字")
}

func TestAnimationBidirectionalLegacyCompatibility(t *testing.T) {
	testLegacyAnimationPlan(t, "history", TaskInput{Topic: "丝绸之路交流", Requirements: "代表幕展示货物双向移动"}, "双向")
}

func TestAnimationScientificEnvelopeLegacyCompatibility(t *testing.T) {
	testLegacyAnimationPlan(t, "invalid-envelope", TaskInput{Topic: "观察科学图示"}, "删除外层text")
}

func TestAnimationOptionalMotionLegacyCompatibility(t *testing.T) {
	testLegacyAnimationPlan(t, "optional-motion", TaskInput{Topic: "图形的相似点", Requirements: "借展示、移动或高亮说明形状相似"}, "")
}

func TestAnimationLensClaimLegacyCompatibility(t *testing.T) {
	testLegacyAnimationPlan(t, "lens-claim", TaskInput{Topic: "凸透镜成像"}, "放大")
}

func TestAnimationLensNoopLegacyCompatibility(t *testing.T) {
	testLegacyAnimationPlan(t, "lens-noop", TaskInput{Topic: "凸透镜成像"}, "初值")
}

func testLegacyAnimationPlan(t *testing.T, fixture string, in TaskInput, problem string) {
	t.Helper()
	path := "../fixtures/animation-regression-" + fixture + ".json"
	if fixture == "science" || fixture == "invalid-scene" || fixture == "invalid-envelope" || fixture == "optional-motion" || fixture == "long-item" || fixture == "text-cues" || fixture == "text-panels" || strings.HasPrefix(fixture, "lens-") {
		path = "../fixtures/animation-v2-science.json"
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if fixture == "long-item" {
		var sc AnimScript
		if err = json.Unmarshal(raw, &sc); err != nil {
			t.Fatal(err)
		}
		sc.Scenes[0].Objects = append(sc.Scenes[0].Objects, AnimObject{ID: "example", Type: "card", Slot: "main", Text: "英语例句", Items: []string{strings.Repeat("a", 29)}})
		raw = asJSON(sc)
	}
	if fixture == "invalid-scene" {
		var sc AnimScript
		if err = json.Unmarshal(raw, &sc); err != nil {
			t.Fatal(err)
		}
		sc.Scenes[3].Objects[0].Scientific.Scene.Objects[2].FontSize = 16
		raw = asJSON(sc)
	}
	if fixture == "invalid-envelope" {
		var sc AnimScript
		if err = json.Unmarshal(raw, &sc); err != nil {
			t.Fatal(err)
		}
		sc.Scenes[0].Objects[0].Text = "重复标题"
		raw = asJSON(sc)
	}
	if strings.HasPrefix(fixture, "lens-") {
		var sc AnimScript
		if err = json.Unmarshal(raw, &sc); err != nil {
			t.Fatal(err)
		}
		sc.Scenes[0].Objects[0].Scientific.Animation.To = 15
		if fixture == "lens-noop" {
			sc.Scenes[0].Objects[0].Scientific.Animation.To = 30
		} else {
			sc.Scenes[0].Narration[0].Display = "两次都是倒立、放大的实像。"
			sc.Scenes[0].Narration[0].Speech = sc.Scenes[0].Narration[0].Display
		}
		raw = asJSON(sc)
	}
	if fixture == "optional-motion" {
		var sc AnimScript
		if err = json.Unmarshal(raw, &sc); err != nil {
			t.Fatal(err)
		}
		sc.Representative = "general"
		sc.Scenes[3].Objects[0].Scientific.Scene.Actions = []DrawingAction{{Object: "particle", Op: "emphasize", DurationMS: 2500}}
		raw = asJSON(sc)
	}
	if fixture == "history" {
		var sc AnimScript
		if err = json.Unmarshal(raw, &sc); err != nil {
			t.Fatal(err)
		}
		// Remove the independently rejected jump so this exercises direction alone.
		for i := range sc.Scenes {
			for j := range sc.Scenes[i].Objects {
				d := sc.Scenes[i].Objects[j].Scientific
				if d == nil || d.Scene == nil {
					continue
				}
				for _, a := range d.Scene.Actions {
					if a.Op == "follow_path" {
						for _, path := range d.Scene.Objects {
							if path.ID == a.Path {
								for k := range d.Scene.Objects {
									if d.Scene.Objects[k].ID == a.Object {
										d.Scene.Objects[k].X, d.Scene.Objects[k].Y = path.Points[0][0], path.Points[0][1]
									}
								}
							}
						}
					}
				}
			}
		}
		raw = asJSON(sc)
	}
	if fixture == "text-cues" || fixture == "text-panels" {
		var prepared AnimScript
		if err := json.Unmarshal(raw, &prepared); err != nil {
			t.Fatal(err)
		}
		if fixture == "text-panels" {
			prepared.Representative = "molecule"
		}
		scene := &prepared.Scenes[0]
		scene.Objects = []AnimObject{{ID: "ideas", Type: "card", Slot: "main", Text: "观察图形", Items: []string{"先观察", "再比较"}}}
		scene.Actions = []AnimAction{{ID: "show", Op: "show", Object: "ideas", At: AnimTrigger{Sentence: scene.Narration[0].ID}}}
		if fixture == "text-panels" {
			scene.Objects = append(scene.Objects, AnimObject{ID: "tips", Type: "card", Slot: "main", Text: "思考方向", Items: []string{"先观察再比较"}})
		}
		raw = asJSON(prepared)
	}
	if fixture != "text-cues" {
		var prepared AnimScript
		if err := json.Unmarshal(raw, &prepared); err != nil {
			t.Fatal(err)
		}
		for i := range prepared.Scenes {
			scene := &prepared.Scenes[i]
			for j := range scene.Objects {
				o := &scene.Objects[j]
				if len(o.Items) > 0 {
					o.ItemSentences = nil
					for k := range o.Items {
						o.ItemSentences = append(o.ItemSentences, scene.Narration[k*len(scene.Narration)/len(o.Items)].ID)
					}
				}
			}
		}
		raw = asJSON(prepared)
	}
	prepared, problems := decodeAnimScript(raw)
	if len(problems) == 0 {
		problems = animTextCueProblems(prepared, AnimScript{})
	}
	if len(problems) == 0 {
		problems = animAuthorProblems(prepared, in)
	}
	if len(problems) == 0 {
		problems = animPlanBudgetProblems(prepared, in)
	}
	if problem == "" {
		if len(problems) != 0 {
			t.Fatalf("valid historical composition rejected: %v", problems)
		}
	} else if len(problems) == 0 || !strings.Contains(strings.Join(problems, "\n"), problem) {
		t.Fatalf("historical %s validation lost: %v", problem, problems)
	}
}
