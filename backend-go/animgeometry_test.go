package main

import (
	"os"
	"strings"
	"testing"
)

func loadGeometryFixture(t *testing.T) AnimScript {
	t.Helper()
	b, err := os.ReadFile("../fixtures/animation-v2-geometry.json")
	if err != nil {
		t.Fatal(err)
	}
	sc, p := decodeAnimScript(b)
	if len(p) > 0 {
		t.Fatalf("geometry fixture: %v", p)
	}
	return sc
}
func TestAnimGeometryScript(t *testing.T) {
	sc := loadGeometryFixture(t)
	ms := map[string]int{}
	for _, s := range sc.Scenes {
		for _, n := range s.Narration {
			ms[n.ID] = 6000
		}
	}
	tl, p := buildAnimTimeline(sc, ms)
	if len(p) > 0 || len(tl.Scenes) != 4 || tl.TotalMS < 30000 || tl.TotalMS > 180000 {
		t.Fatal(tl, p)
	}
}
func TestAnimGeometryGates(t *testing.T) {
	tests := []struct {
		name, want string
		mutate     func(*AnimScript)
	}{
		{"zero size", "边长", func(s *AnimScript) { s.Scenes[0].Objects[1].Params["width"] = 0 }},
		{"invented pixel", "不接受参数x", func(s *AnimScript) { s.Scenes[0].Objects[1].Params["x"] = 50 }},
		{"missing shape", "引用", func(s *AnimScript) { s.Scenes[0].Objects[2].Shape = "missing" }},
		{"size tween", "可变参数", func(s *AnimScript) { s.Scenes[1].Actions[0].Param = "width" }},
		{"excessive target precision", "两位小数", func(s *AnimScript) { s.Scenes[1].Actions[0].To = f64(3.001) }},
		{"out of view", "超出坐标", func(s *AnimScript) { s.Scenes[1].Actions[0].To = f64(20) }},
		{"no real motion", "代表幕", func(s *AnimScript) { s.Scenes[1].Actions[0].To = f64(0) }},
		{"wrong area", "面积", func(s *AnimScript) { s.Scenes[0].Narration[1].Display = "面积是12平方单位。" }},
		{"wrong spoken area", "面积", func(s *AnimScript) { s.Scenes[0].Narration[1].Speech = "面积是十二平方单位。" }},
		{"wrong rotation", "旋转方向", func(s *AnimScript) { s.Scenes[2].Narration[1].Display = "顺时针旋转90度。" }},
		{"wrong displacement", "平移", func(s *AnimScript) { s.Scenes[1].Narration[0].Display = "向右平移5个单位。" }},
		{"function objects in geometry", "原型", func(s *AnimScript) { s.Scenes[0].Objects[1].Type = "line" }},
		{"clipped rotation middle", "超出坐标", func(s *AnimScript) {
			for i := range s.Scenes {
				s.Scenes[i].Objects[0].Params = map[string]float64{"xmin": -7, "xmax": 7, "ymin": -3.45, "ymax": 3.45}
			}
			s.Scenes[1].Actions[0].To = f64(1)
			s.Scenes[2].Objects[1].Type = "rectangle"
			s.Scenes[2].Actions[0].To = f64(180)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { s := loadGeometryFixture(t); tt.mutate(&s); expectProblem(t, s, tt.want) })
	}
}
func TestAnimGeometryTimelineRejectsConcurrentTransforms(t *testing.T) {
	s := loadGeometryFixture(t)
	s.Scenes[1].Actions = append(s.Scenes[1].Actions, AnimAction{ID: "rotation", Op: "tween", Object: "shape1", Param: "angle", To: f64(45), At: AnimTrigger{Sentence: "g3"}, DurationMS: 2500})
	ms := map[string]int{}
	for _, sc := range s.Scenes {
		for _, n := range sc.Narration {
			ms[n.ID] = 6000
		}
	}
	_, p := buildAnimTimeline(s, ms)
	if !strings.Contains(strings.Join(p, ";"), "时间重叠") {
		t.Fatal("concurrent geometry transforms accepted", p)
	}
}

func TestAnimGeometryRigidTransformAndArea(t *testing.T) {
	p := map[string]float64{"width": 3, "height": 2, "tx": 2, "ty": -1, "angle": 90, "cx": 1, "cy": 1}
	expected := [][2]float64{{4, -1}, {4, 2}, {2, 2}, {2, -1}}
	actual := animGeometryVertices("rectangle", p)
	for i, v := range actual {
		if !animNear(v[0], expected[i][0]) || !animNear(v[1], expected[i][1]) {
			t.Fatalf("vertex %d: %v want %v", i, v, expected[i])
		}
	}
	for _, kind := range []string{"rectangle", "right_triangle"} {
		for _, angle := range []float64{-270, -90, 0, 45, 90, 180, 360} {
			p["angle"] = angle
			vertices := animGeometryVertices(kind, p)
			sum := 0.0
			for i, a := range vertices {
				b := vertices[(i+1)%len(vertices)]
				sum += a[0]*b[1] - b[0]*a[1]
			}
			expectedArea := 6.0
			if kind == "right_triangle" {
				expectedArea = 3
			}
			if !animNear(sum/2, expectedArea) {
				t.Fatalf("%s at %g area %g", kind, angle, sum/2)
			}
		}
	}
}

func TestAnimGeometryCannotChangeTopicDuringEdit(t *testing.T) {
	if animRequestedArchetype(TaskInput{Topic: "矩形平移", Requirements: "对比一次函数中的图像平移"}) != "geo.transform@1" {
		t.Fatal("notes replaced explicit topic")
	}
	old := loadGeometryFixture(t)
	next := old
	next.Archetype = "func.params@1"
	if len(animRevisionProblems(old, next)) == 0 {
		t.Fatal("topic changed inside a local edit")
	}
}

func TestAnimScientificAndFunctionTopicRouting(t *testing.T) {
	for _, item := range []struct{ topic, notes, want string }{
		{"二次函数的开口变化", "", "func.params@1"},
		{"反比例函数的系数", "", "func.params@1"},
		{"通用动画分拣过程", "组合矩形、路径和标注", "lesson.explain@1"},
		{"细胞结构", "用矩形标注显示位置", "lesson.explain@1"},
		{"使用lesson.explain@1组合矩形图形与旋转动作", "", "lesson.explain@1"},
	} {
		if got := animRequestedArchetype(TaskInput{Topic: item.topic, Requirements: item.notes}); got != item.want {
			t.Errorf("%s routed to %s, want %s", item.topic, got, item.want)
		}
	}
}
