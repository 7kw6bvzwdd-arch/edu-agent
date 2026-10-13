package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// Regression from the actual generated homework page, not an invented defect.
func TestLiveExerciseVisibilityRegression(t *testing.T) {
	if os.Getenv("TEST_FULL_DECK_LIVE") != "1" {
		t.Skip("explicit paid reviewer opt-in required")
	}
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	id := worksFixture(t, s, token, "courseware", "succeeded", "实际作业页回归", TaskInput{})
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	s.cfg.Mode = "live"
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = os.Getenv("EVAL_LLM_BASE_URL"), os.Getenv("EVAL_LLM_API_KEY"), os.Getenv("EVAL_LLM_MODEL")
	b, err := os.ReadFile("../work/courseware-full-quality/linear/plan-fix/checkpoint.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Result Deck `json:"result"`
	}
	if err = json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	d := v.Result
	d.Slides = []Slide{d.Slides[len(d.Slides)-1]}
	results := []map[string]any{}
	defer func() { os.WriteFile("../work/courseware-full-quality/exercise-review.json", asJSON(results), 0600) }()
	for _, fixed := range []bool{false, true} {
		if fixed {
			d.Slides[0].Bullets = []string{"A必做：判断y=3x-1、y=5、y=-x+2是否为一次函数，写出k、b及y=-x+2的升降性。", "B必做：同一坐标比例画y=2x+1与y=-x+2，标出与两轴交点，估计两直线交点。", "C选做：求过(0,-1)和(1,1)的一次函数解析式。"}
			// The live auditor correctly found the original answer omitted b for
			// y=5. Complete that answer without adding unnecessary graph bounds.
			d.Slides[0].Answer = "A：y=3x-1中k=3、b=-1；y=5中k=0、b=5，因k=0不是一次函数；y=-x+2中k=-1、b=2，随x增大而减小。B：y=2x+1两轴交点(-1/2,0)、(0,1)；y=-x+2两轴交点(2,0)、(0,2)；两直线交点(1/3,5/3)，学生作图时估计，再用代数核验。C：代入(0,-1)得b=-1；代入(1,1)得1=k-1，k=2，故y=2x-1。"
		}
		ctx, stop := context.WithTimeout(context.Background(), time.Minute)
		issues, status, e := s.reviewTeachingPage(ctx, a, id, TaskInput{Topic: "一次函数", Subject: "数学", Grade: "八年级", LessonMinutes: 45}, d)
		stop()
		results = append(results, map[string]any{"fixed": fixed, "issues": issues, "status": status, "available": e == nil})
		if e != nil || status != "checked" {
			t.Fatal("review unavailable", e)
		}
		if (blockingPageIssues(issues) != "") == fixed {
			t.Fatal("hidden exercise question missed or complete question rejected", fixed, issues)
		}
	}
}

func TestLivePhysicsUnitConversionRegression(t *testing.T) {
	if os.Getenv("TEST_FULL_DECK_LIVE") != "1" {
		t.Skip("explicit paid reviewer opt-in required")
	}
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	id := worksFixture(t, s, token, "courseware", "succeeded", "实际体积单位回归", TaskInput{})
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	s.cfg.Mode = "live"
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = os.Getenv("EVAL_LLM_BASE_URL"), os.Getenv("EVAL_LLM_API_KEY"), os.Getenv("EVAL_LLM_MODEL")
	d := Deck{Title: "浮力", Theme: "ocean", Slides: []Slide{{Title: "浸没物体的浮力", Layout: "example", Bullets: []string{"体积1000 cm³的物体浸没在水中；ρ水=1000 kg/m³，g=10 N/kg，求浮力。"}, Formula: `F_{浮}=\rho_{水}gV_{排}=10\mathrm{N}`, Notes: "浸没时V排=V物，1000 cm³=0.001 m³。F浮=1000×10×0.001=10 N。题设计算，非实测。常见错误：V用cm³直接代入，结果小1000倍。"}}}
	results := []map[string]any{}
	defer func() {
		os.WriteFile("../work/courseware-full-quality/physics-unit-review.json", asJSON(results), 0600)
	}()
	for _, fixed := range []bool{false, true} {
		if fixed {
			d.Slides[0].Notes = "浸没时V排=V物。1 cm³=10⁻⁶ m³，故1000 cm³=0.001 m³。F浮=1000 kg/m³×10 N/kg×0.001 m³=10 N（按题设计算，非实测）。若把1000直接代入以m³为体积单位的公式，误算为10000000 N，是正确结果的1000000倍；因此必须先统一单位。"
		}
		ctx, stop := context.WithTimeout(context.Background(), time.Minute)
		issues, status, err := s.reviewTeachingPage(ctx, a, id, TaskInput{Topic: "浮力", Subject: "物理", Grade: "八年级", LessonMinutes: 45}, d)
		stop()
		results = append(results, map[string]any{"fixed": fixed, "issues": issues, "status": status, "available": err == nil})
		if err != nil || status != "checked" {
			t.Fatal("review unavailable", err)
		}
		if (blockingPageIssues(issues) != "") == fixed {
			t.Fatal("incorrect conversion missed or correct conversion rejected", fixed, issues)
		}
	}
}
