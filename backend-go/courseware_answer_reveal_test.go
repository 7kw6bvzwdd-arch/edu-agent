package main

import "testing"

func TestObservedExerciseDisclosureAtGenerationBoundary(t *testing.T) {
	for _, sl := range []Slide{
		{Layout: "exercise", Title: "读图", Bullets: []string{"两直线相交，读出交点坐标"}, Focus: "交点(1/3,5/3)处函数值相等吗？", Graph: &FunctionGraph{Series: []FunctionSeries{{K: 2, B: 1}, {K: -1, B: 2}}}},
		{Layout: "exercise", Title: "配方", Bullets: []string{"对y=x²-2x-3配方，求顶点"}, Formula: "y=x^2-2x-3=(x-1)^2-4"},
	} {
		sl.Notes = "教师讲解：保留已给条件，学生先独立作答，最后展示教师答案并核对。"
		if sl.Graph != nil {
			sl.Graph.Title = "给定两条直线"
			sl.Graph.XMin = -2
			sl.Graph.XMax = 3
			sl.Graph.YMin = -3
			sl.Graph.YMax = 7
		}
		d := Deck{Title: "课堂练习", Theme: "ocean", Lesson: "本页教学说明见教师备注。", Slides: []Slide{sl}, NeedsReview: true}
		if err := validateDeckMinimum(d, 1); err != nil {
			t.Fatal("invalid disclosure fixture", err)
		}
		if err := coursewarePageIssue(d, 1, nil); err == nil {
			t.Fatal("generation accepted visible solution", sl)
		}
		d.Slides[0].Layout = "example"
		if len(exerciseVisibilityIssues(d)) != 0 {
			t.Fatal("worked example was hidden")
		}
	}
}

func TestExerciseExplicitSolutionStaysTeacherOnly(t *testing.T) {
	for _, field := range []string{"title", "body", "focus", "table"} {
		sl := Slide{Title: "求浮力", Layout: "exercise", Bullets: []string{"物块空气中称重5 N，浸没后示数3 N，求浮力"}, Answer: "先列式\n答案：2 N"}
		switch field {
		case "title":
			sl.Title = "答案：2 N"
		case "body":
			sl.Bullets = append(sl.Bullets, "答案：2 N")
		case "focus":
			sl.Focus = "答案：2 N"
		case "table":
			sl.Table = &TeachingTable{Headers: []string{"量", "结果"}, Rows: [][]string{{"答案：2 N", "2"}}}
		}
		if len(exerciseVisibilityIssues(Deck{Slides: []Slide{sl}})) == 0 {
			t.Fatal("visible explicit answer passed", field)
		}
	}
	for _, sl := range []Slide{
		{Layout: "exercise", Title: "检验给定答案", Bullets: []string{"已知参考答案：2 N，判断是否正确并说明理由"}},
		{Layout: "exercise", Title: "求浮力", Bullets: []string{"空气中5 N，浸没后3 N，求浮力"}, Answer: "答案：2 N", Notes: "答案：2 N"},
		{Layout: "example", Title: "例题", Bullets: []string{"答案：2 N"}},
	} {
		if len(exerciseVisibilityIssues(Deck{Slides: []Slide{sl}})) != 0 {
			t.Fatal("valid teaching control blocked", sl)
		}
	}
}
