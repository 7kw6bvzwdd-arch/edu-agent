package main

import (
	"fmt"
	"math"
	"strings"
)

// All authoring/build entrypoints use the original request, so renaming a plan
// cannot bypass its visual requirement. Existing delivered artifacts are untouched.
func animVisualProblems(sc AnimScript, in TaskInput) []string {
	if sc.Archetype != "lesson.explain@1" {
		return nil
	}
	text := strings.ToLower(in.Topic + " " + in.Requirements + " " + in.Objectives)
	requireMotion := animHasAny(text, "运动", "移动", "沿路径", "沿路线", "位置变化", "motion")
	if animHasAny(text, "移动或高亮", "移动或展示", "展示或移动") && !animHasAny(text, "必须移动", "实际移动", "必须运动", "对象运动", "位置变化") {
		requireMotion = false
	}
	carbon := animHasAny(text, "碳单质", "碳的同素异形", "金刚石", "石墨", "富勒烯", "c60", "c₆₀")
	wantKinds := map[string]bool{}
	for _, rule := range []struct {
		kind  string
		terms []string
	}{
		{"thin_lens", []string{"凸透镜", "薄透镜"}},
		{"molecule", []string{"分子结构", "官能团", "羟基"}},
		{"cell", []string{"细胞结构", "植物细胞", "动物细胞"}},
	} {
		if animHasAny(text, rule.terms...) {
			wantKinds[rule.kind] = true
		}
	}
	if carbon {
		wantKinds["carbon"] = true
	}
	required := []string{}
	if carbon {
		if animHasAny(text, "碳单质", "碳的同素异形") {
			required = append(required, "diamond", "graphite")
		}
		for _, pair := range [][2]string{{"金刚石", "diamond"}, {"石墨", "graphite"}, {"富勒烯", "fullerene"}, {"c60", "fullerene"}, {"c₆₀", "fullerene"}} {
			if strings.Contains(text, pair[0]) {
				required = append(required, pair[1])
			}
		}
	}
	found := map[string]bool{}
	built, slid := false, false
	highlighted := map[string]bool{}
	directions := [][2]float64{}
	var problems []string
	var lensMagnifications []float64
	for _, scene := range sc.Scenes {
		for _, o := range scene.Objects {
			if o.Type == "scientific" && o.Scientific.Kind == "thin_lens" {
				d := o.Scientific
				us := []float64{d.ObjectDistanceCM}
				if d.Animation != nil {
					us = append(us, d.Animation.To)
				}
				for _, u := range us {
					if u != d.FocalLengthCM {
						lensMagnifications = append(lensMagnifications, math.Abs(d.FocalLengthCM/(u-d.FocalLengthCM)))
					}
				}
			}
		}
	}
	representative := false
	var candidates []string
	for _, scene := range sc.Scenes {
		for _, o := range scene.Objects {
			localCarbon := false
			if o.Type == "scientific" && o.Scientific.Kind == "thin_lens" {
				d := o.Scientific
				if d.Animation != nil && d.Animation.To == d.ObjectDistanceCM {
					problems = append(problems, "幕"+scene.ID+"透镜物距动画目标与初值相同，画面不会变化；静态说明请省略animation并用show")
				}
				for _, n := range scene.Narration {
					text := n.Display + " " + n.Speech
					if animHasAny(text, "两次", "两处", "两种") && animHasAny(text, "都是", "均为", "均是") && animHasAny(text, "放大", "缩小") {
						for _, m := range lensMagnifications {
							if strings.Contains(text, "放大") && m <= 1 || strings.Contains(text, "缩小") && m >= 1 {
								problems = append(problems, fmt.Sprintf("幕%s句%s所称两次均放大/缩小与本片透镜状态不符，存在绝对放大率%.2f；须区分倒立缩小实像、等大实像和倒立放大实像", scene.ID, n.ID, m))
								break
							}
						}
					}
				}
			}
			if o.Type == "scientific" && o.Scientific.Kind == "scene" {
				for _, a := range o.Scientific.Scene.Actions {
					for _, shape := range o.Scientific.Scene.Objects {
						if shape.ID == a.Object && shape.Type == "carbon" && a.To != nil {
							if a.Op == "reveal" && *a.To > 0 && shape.Allotrope == "diamond" {
								built = true
							}
							if a.Op == "slide" && *a.To > 0 && shape.Allotrope == "graphite" {
								slid = true
							}
						}
					}
				}
				for _, a := range o.Scientific.Scene.Actions {
					for _, shape := range o.Scientific.Scene.Objects {
						if shape.ID != a.Object || shape.Type == "text" {
							continue
						}
						if a.Op == "follow_path" {
							for _, path := range o.Scientific.Scene.Objects {
								if path.ID == a.Path && len(path.Points) > 1 {
									p, q := path.Points[0], path.Points[len(path.Points)-1]
									directions = append(directions, [2]float64{q[0] - p[0], q[1] - p[1]})
								}
							}
						}
						if a.Op == "move" && a.X != nil && a.Y != nil {
							directions = append(directions, [2]float64{*a.X - shape.X, *a.Y - shape.Y})
						}
					}
				}
			}
			if o.Type == "scientific" && scienceAnimated(o.Scientific) {
				driven := false
				for _, a := range scene.Actions {
					if a.Object == o.ID && a.Op == "tween" && a.Param == "progress" && a.At.Sentence != "" && a.To != nil && *a.To != o.Params["progress"] {
						driven = true
					}
				}
				if !driven {
					problems = append(problems, "幕"+scene.ID+"对象"+o.ID+"的内部动画未由句子锚点tween progress驱动；show/emphasize不能触发内部高亮或运动")
				}
				if driven && o.Scientific.Kind == "cell" {
					if animHasAny(in.Requirements, "依次高亮", "逐个高亮") && len(o.Scientific.HighlightParts) > 1 {
						problems = append(problems, "幕"+scene.ID+"同时高亮多个部位不等于依次高亮；请在现有各幕分别选择一个highlight_parts并锚定该结构的旁白")
					} else {
						for _, part := range o.Scientific.HighlightParts {
							highlighted[part] = true
						}
					}
				}
			}
			moving := false
			for _, a := range scene.Actions {
				if a.Object != o.ID || a.At.Sentence == "" {
					continue
				}
				if o.Type == "scientific" && scienceAnimated(o.Scientific) && a.Op == "tween" && a.Param == "progress" && a.To != nil && *a.To != o.Params["progress"] {
					if o.Scientific.Kind != "scene" {
						moving = true
					} else {
						for _, shape := range o.Scientific.Scene.Objects {
							if shape.Type == "text" {
								continue
							}
							for _, action := range o.Scientific.Scene.Actions {
								if action.Object == shape.ID {
									if requireMotion && !animContains([]string{"move", "follow_path", "rotate", "scale", "slide", "reveal"}, action.Op) {
										continue
									}
									if action.Op == "move" && action.X != nil && action.Y != nil && *action.X == shape.X && *action.Y == shape.Y {
										continue
									}
									if action.Op == "rotate" && action.To != nil && *action.To == 0 {
										continue
									}
									if action.Op == "scale" && action.To != nil && *action.To == 1 {
										continue
									}
									moving = true
									if shape.Type == "carbon" {
										found[shape.Allotrope] = true
										localCarbon = true
									}
								}
							}
						}
					}
				}
				explicitDiagram := o.Type == "timeline" && strings.Contains(in.Requirements, "时间线") || o.Type == "relationship" && strings.Contains(in.Requirements, "关系链")
				if !requireMotion && len(wantKinds) == 0 && (o.Type == "illustration" || explicitDiagram) && (a.Op == "show" || a.Op == "emphasize") {
					moving = true
				}
			}
			if moving && o.Type == "scientific" && o.Scientific.Kind == "carbon" {
				found[o.Scientific.Allotrope] = true
			}
			composedCarbon := carbon && localCarbon
			if moving && (len(wantKinds) == 0 || o.Type == "scientific" && wantKinds[o.Scientific.Kind] || composedCarbon) {
				candidates = append(candidates, scene.ID)
				if scene.ID == sc.Representative {
					representative = true
				}
			}
		}
	}
	if !representative {
		problems = append(problems, fmt.Sprintf("当前代表幕representative=%s未满足教学图形动作；请将representative改为核心变化幕（通过本项动作检查的幕：%s），或在当前代表幕补上实际变化；不要只重复原稿", sc.Representative, strings.Join(candidates, "、")))
		problems = append(problems, "代表幕必须呈现与课题对应的实际教学图形及句子锚点动作，不能只展示/强调文字卡；明确要求运动时必须有非文字对象实际运动，不能用静态插图淡入替代；可使用scientific专用模型或带非文字对象动作的scene")
	}
	seen := map[string]bool{}
	if strings.Contains(text, "双向") {
		opposed := false
		for _, a := range directions {
			for _, b := range directions {
				if a[0]*b[0]+a[1]*b[1] < -0.0001 {
					opposed = true
				}
			}
		}
		if !opposed {
			problems = append(problems, "要求双向交流但没有方向相反的非文字对象运动；请用两条相反方向路径或move目标，不能共用同向follow_path")
		}
	}
	if wantKinds["cell"] && strings.Contains(in.Requirements, "高亮") {
		for _, part := range [][2]string{{"细胞壁", "cell_wall"}, {"细胞膜", "cell_membrane"}, {"细胞核", "nucleus"}, {"叶绿体", "chloroplast"}, {"液泡", "vacuole"}, {"细胞质", "cytoplasm"}, {"线粒体", "mitochondrion"}} {
			if strings.Contains(in.Requirements, part[0]) && !highlighted[part[1]] {
				problems = append(problems, "缺少按旁白动态高亮的细胞结构"+part[0]+" highlight_parts="+part[1]+"；请安排到现有分镜并添加句子锚点progress动作")
			}
		}
	}
	for _, kind := range required {
		if !found[kind] && !seen[kind] {
			problems = append(problems, "碳结构课题缺少动态scientific kind=carbon allotrope="+kind+"，须用progress插值展示结构，不能用文字或二维分子图替代")
			seen[kind] = true
		}
	}
	if carbon && animHasAny(text, "逐步构建", "逐步搭建", "逐层构建", "逐步展开") && !built {
		problems = append(problems, "需求中的金刚石逐步构建须使用scene内部carbon diamond的reveal动作，完整模型旋转不等于构建")
	}
	if carbon && animHasAny(text, "层间滑动", "层间相对滑动") && !slid {
		legacy := false
		for _, s := range sc.Scenes {
			for _, o := range s.Objects {
				if o.Type == "scientific" && o.Scientific.Kind == "carbon" && o.Scientific.Allotrope == "graphite" && scienceAnimated(o.Scientific) {
					legacy = true
				}
			}
		}
		if !legacy {
			problems = append(problems, "层间滑动须有graphite的slide动作，不能用文字或整体旋转替代")
		}
	}
	return problems
}
