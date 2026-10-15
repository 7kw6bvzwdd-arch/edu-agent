package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBlindExerciseTruncationRetriesOnlyVisibleProblemHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	id := worksFixture(t, s, token, "courseware", "succeeded", "截断盲解", TaskInput{})
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	calls, alwaysLength := 0, false
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var p struct {
			Messages  []struct{ Content string }
			Thinking  map[string]string
			MaxTokens int `json:"max_tokens"`
		}
		json.NewDecoder(r.Body).Decode(&p)
		prompt := p.Messages[1].Content
		if strings.Contains(prompt, "PRIVATE-ANSWER") || strings.Contains(prompt, "PRIVATE-NOTES") {
			t.Error("retry leaked teacher answer")
		}
		if calls%2 == 0 && (p.Thinking["type"] != "disabled" || p.MaxTokens != 3000) {
			t.Error("fallback still spends budget on hidden reasoning")
		}
		finish, content := "length", `{"solution":"truncated`
		if calls%2 == 0 && !alwaysLength {
			finish, content = "stop", `{"solution":"只凭测力计示数为零不能确定漂浮或悬浮；全浸没才有排液体积等于物体体积。"}`
		}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": finish, "message": map[string]string{"content": content}}}})
	}))
	defer provider.Close()
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = provider.URL, "test", "deepseek-flash"
	d := Deck{Slides: []Slide{{Layout: "exercise", Bullets: []string{"示数为零能断定漂浮吗？"}, Notes: "PRIVATE-NOTES", Answer: "PRIVATE-ANSWER"}}}
	if _, err := s.solveVisibleTeachingExercise(context.Background(), a, id, provider.URL, "test", "deepseek-flash", TaskInput{Subject: "物理"}, d); err != nil || calls != 2 {
		t.Fatal("truncated blind solve did not recover", err, calls)
	}
	alwaysLength = true
	if _, err := s.solveVisibleTeachingExercise(context.Background(), a, id, provider.URL, "test", "deepseek-flash", TaskInput{Subject: "物理"}, d); !errors.Is(err, errModelTruncated) || calls != 4 {
		t.Fatal("retry was unbounded or incomplete JSON accepted", err, calls)
	}
}

func TestUnitExplanationGetsFocusedReviewHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	id := worksFixture(t, s, token, "courseware", "succeeded", "单位说明", TaskInput{})
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	calls, unavailable := 0, false
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&payload)
		answer := `{"issues":[]}`
		if strings.Contains(payload.Messages[1].Content, "你是教学风险复核员") {
			answer = `{"checks":[{"index":1,"evidence":"cm³数值未换算直接当m³使用会放大1000000倍，而不是缩小1000倍。","issues":[{"object":"结果小1000倍","category":"answer","certainty":"confirmed","severity":"error","evidence":"cm³数值直接当m³会大1000000倍","repair":"修正倍率，保留10 N答案"}]}]}`
			if unavailable {
				answer = `{}`
			}
		}
		respond(w, 200, map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": answer}}}})
	}))
	defer provider.Close()
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = provider.URL, "test-key", "test-review"
	d := Deck{Slides: []Slide{{Notes: "V用cm³直接代入，结果小1000倍。"}}}
	issues, status, err := s.reviewTeachingPage(context.Background(), a, id, TaskInput{}, d)
	if err != nil || status != "checked" || blockingPageIssues(issues) == "" || calls != 2 {
		t.Fatal("general pass hid unit error", issues, status, err, calls)
	}
	unavailable = true
	if _, status, err = s.reviewTeachingPage(context.Background(), a, id, TaskInput{}, d); err == nil || status != "unavailable" {
		t.Fatal("failed numerical check accepted", status, err)
	}
	prior := calls
	d.Slides[0].Notes = "浸没后浮力方向竖直向上。"
	if _, _, err = s.reviewTeachingPage(context.Background(), a, id, TaskInput{}, d); err != nil || calls != prior+1 {
		t.Fatal("unrelated page received extra check", err, calls)
	}
}

func TestVisibleExerciseClassificationDoesNotCountAsAQuestion(t *testing.T) {
	bad := Slide{Layout: "exercise", Title: "分层作业", Bullets: []string{"A、B组必做：判断读图与两点法作图", "C组选做：由两点求解析式"}, Notes: "判断y=3x-1等；求过(0,-1)、(1,1)的一次函数。"}
	if len(exerciseVisibilityIssues(Deck{Slides: []Slide{bad}})) != 1 {
		t.Fatal("classification headings passed as complete homework")
	}
	for _, bullets := range [][]string{{"判断y=3x-1是否为一次函数，写出k、b。"}, {"解释完全浸没后浮力为何不随深度增加。"}, {"为什么并联电路一个支路断开，另一支路仍有电流？"}} {
		good := bad
		good.Bullets = bullets
		if len(exerciseVisibilityIssues(Deck{Slides: []Slide{good}})) > 0 {
			t.Fatal("specific question rejected", bullets)
		}
	}
	bad.Graph = &FunctionGraph{}
	if len(exerciseVisibilityIssues(Deck{Slides: []Slide{bad}})) > 0 {
		t.Fatal("program must defer meaningful diagrams to auditor")
	}
}

func TestTeachingRiskReviewIsBoundedAndFailClosedHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	id := worksFixture(t, s, token, "courseware", "succeeded", "教学风险复核", TaskInput{})
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	calls, focused := 0, 0
	broken := false
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&payload)
		answer := `{"issues":[]}`
		if strings.Contains(payload.Messages[1].Content, "你是教学风险复核员") {
			focused++
			if !strings.Contains(payload.Messages[1].Content, "actual_visible_input") || !strings.Contains(payload.Messages[1].Content, "来源") {
				t.Error("focused review missing context")
			}
			answer = `{"checks":[{"index":1,"evidence":"该实验数据为自编示例，不应直接标为已经实测，须核对原始记录。","issues":[]},{"index":2,"evidence":"6V并联电路支路为6V，超过3V最大量程，必须先使用15V量程。","issues":[{"object":"6V用0—3V量程","category":"conditions","certainty":"confirmed","severity":"error","evidence":"6V超出量程","repair":"先选0—15V量程"}]}]}`
			if broken {
				answer = `{}`
			}
		}
		respond(w, 200, map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": answer}}}})
	}))
	defer provider.Close()
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = provider.URL, "test", "test"
	d := Deck{Slides: []Slide{{Layout: "steps", Bullets: []string{"两灯并联的6V电路，测量支路电压。"}, Notes: "电压表选0—3V试触，示例数据标实测。"}}}
	in := TaskInput{Subject: "物理"}
	issues, status, err := s.reviewTeachingPage(context.Background(), a, id, in, d)
	if err != nil || status != "checked" || blockingPageIssues(issues) == "" || calls != 2 || focused != 1 {
		t.Fatal("multiple risk types must share one extra review", issues, status, err, calls, focused)
	}
	broken = true
	if _, status, err = s.reviewTeachingPage(context.Background(), a, id, in, d); err == nil || status != "unavailable" {
		t.Fatal("unavailable risk check accepted", status, err)
	}
	prior := calls
	d = Deck{Slides: []Slide{{Title: "课题目标", Bullets: []string{"认识函数的自变量与因变量。"}}}}
	if _, status, err = s.reviewTeachingPage(context.Background(), a, id, TaskInput{Subject: "数学"}, d); err != nil || status != "checked" || calls != prior+1 {
		t.Fatal("unrelated page received extra review", status, err, calls)
	}
}

func TestTeachingReviewContractRecheckNeverRegeneratesPageHTTP(t *testing.T) {
	for _, tc := range []struct{ name, first string }{
		{"contradictory error", `{"issues":[{"object":"正确对象","category":"answer","certainty":"confirmed","severity":"error","evidence":"实际正确","repair":"无需修改"}]}`},
		{"wrong certainty", `{"issues":[{"object":"正确对象","category":"conditions","certainty":"suggestion","severity":"suggestion","evidence":"可选表达建议","repair":"保留开放作图任务"}]}`},
		{"wrong category", `{"issues":[{"object":"正确对象","category":"style","certainty":"uncertain","severity":"suggestion","evidence":"可选表达建议","repair":"保持正确内容"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) { testTeachingReviewContractRecheckHTTP(t, tc.first) })
	}
}

func testTeachingReviewContractRecheckHTTP(t *testing.T, first string) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	id := worksFixture(t, s, token, "courseware", "succeeded", "审核契约重查", TaskInput{})
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&payload)
		answer := first
		if calls == 2 {
			if !strings.Contains(payload.Messages[1].Content, "上次审核契约冲突") {
				t.Error("contract recheck missing correction")
			}
			answer = `{"issues":[]}`
		}
		respond(w, 200, map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": answer}}}})
	}))
	defer provider.Close()
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = provider.URL, "test", "test"
	d := Deck{Slides: []Slide{{Title: "知识目标", Bullets: []string{"理解变量关系"}}}}
	before := string(asJSON(d))
	issues, status, err := s.reviewTeachingPage(context.Background(), a, id, TaskInput{}, d)
	if err != nil || status != "checked" || blockingPageIssues(issues) != "" || calls != 2 || string(asJSON(d)) != before {
		t.Fatal("wrong contract recovery", issues, status, err, calls)
	}
}

func TestTeachingArithmeticReviewRecheckIsBoundedHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	id := worksFixture(t, s, token, "courseware", "succeeded", "算术审核重查", TaskInput{})
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	before := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
	billing := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	calls, persistent := 0, false
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload struct {
			Messages []struct{ Content string }
		}
		json.NewDecoder(r.Body).Decode(&payload)
		if calls%2 == 0 && !strings.Contains(payload.Messages[1].Content, "审核依据或修复建议含错误四则运算") {
			t.Error("second request did not identify the actual arithmetic error")
		}
		answer := `{"issues":[{"object":"单位解释","category":"answer","certainty":"confirmed","severity":"suggestion","evidence":"倍率计算：10000/10=1000000。","repair":"补充说明"}]}`
		if calls%2 == 0 && !persistent {
			answer = `{"issues":[]}`
		}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}}}})
	}))
	defer provider.Close()
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = provider.URL, "test", "test"
	d := Deck{Slides: []Slide{{Title: "知识目标", Bullets: []string{"理解变量关系"}}}}
	issues, status, err := s.reviewTeachingPage(context.Background(), a, id, TaskInput{}, d)
	if err != nil || status != "checked" || len(issues) != 0 || calls != 2 {
		t.Fatal("a wrong suggestion was not corrected automatically", issues, status, err, calls)
	}
	persistent = true
	if _, status, err = s.reviewTeachingPage(context.Background(), a, id, TaskInput{}, d); err == nil || status != "unavailable" || calls != 4 {
		t.Fatal("persistent wrong review was accepted or retried without a bound", status, err, calls)
	}
	after := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
	for _, field := range []string{"result", "revision", "status", "charge_state"} {
		if string(asJSON(before[field])) != string(asJSON(after[field])) {
			t.Error("review changed saved work", field)
		}
	}
	if got := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String(); got != billing {
		t.Fatal("review recheck changed business credits")
	}
	worksCall(t, s, "GET", "/api/tasks/"+id, worksLogin(t, s, "other"), nil, 404)
}

func TestCoursewareArithmeticErrorRepairsOnlyCurrentPageHTTP(t *testing.T) {
	testCoursewareCalculatedPageRepairHTTP(t, "arithmetic")
}

func TestCoursewarePhysicsConditionRepairsOnlyCurrentPageHTTP(t *testing.T) {
	testCoursewareCalculatedPageRepairHTTP(t, "physics")
}

func TestCoursewareDisclosureRepairsOnlyCurrentPageHTTP(t *testing.T) {
	testCoursewareCalculatedPageRepairHTTP(t, "disclosure")
}

func TestCoursewareVertexDisclosureRepairsOnlyCurrentPageHTTP(t *testing.T) {
	testCoursewareCalculatedPageRepairHTTP(t, "vertex-disclosure")
}

func TestCoursewareBuoyancyEquilibriumRepairsOnlyCurrentPageHTTP(t *testing.T) {
	testCoursewareCalculatedPageRepairHTTP(t, "equilibrium")
}

func testCoursewareCalculatedPageRepairHTTP(t *testing.T, scenario string) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	in := TaskInput{Type: "courseware", Topic: "浮力", Subject: "物理", Pages: 8, Theme: "ocean", Detail: "concise", LessonMinutes: 45, UseKB: true, PreviewFirst: true, IdempotencyKey: newID()}
	physics, readGraph, equilibrium := scenario == "physics", scenario == "disclosure", scenario == "equilibrium"
	vertex := scenario == "vertex-disclosure"
	if readGraph {
		in.Topic, in.Subject = "一次函数", "数学"
	}
	if vertex {
		in.Topic, in.Subject = "二次函数", "数学"
	}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	defer worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(context.Background(), "UPDATE tasks SET status='researching',mode='live' WHERE id=$1", id); err != nil {
		tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	d := sampleDeck(in)
	before := string(asJSON(d))
	generated, textReviewed := 0, 0
	repairEvidence := "程序按四则运算核对"
	if physics {
		repairEvidence = "本页要求在空气中称重"
	}
	var disclosureCase teachingRegressionCase
	if readGraph {
		disclosureCase = intersectionDisclosureCase(t)
		repairEvidence = "上屏提示已直接给出"
	}
	if vertex {
		for _, tc := range teachingRegressionCases(t) {
			if tc.ID == "completing-square-formula-disclosure" {
				disclosureCase = tc
			}
		}
		if disclosureCase.Bad == nil {
			t.Fatal("missing actual completing-square exercise")
		}
		repairEvidence = "可见公式已在题设多项式后给出"
	}
	if equilibrium {
		for _, tc := range teachingRegressionCases(t) {
			if tc.ID == "buoyancy-equilibrium-does-not-imply-immersion" {
				disclosureCase = tc
			}
		}
		if disclosureCase.Bad == nil {
			t.Fatal("missing actual equilibrium case")
		}
		repairEvidence = "浮力等于重力只给出力的平衡"
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model    string
			Messages []struct{ Content any }
		}
		json.NewDecoder(r.Body).Decode(&req)
		answer := `{"issues":[]}`
		if req.Model == "generation" {
			generated++
			prompt := req.Messages[len(req.Messages)-1].Content.(string)
			if !strings.Contains(prompt, "本批只返回第2至第2页") {
				t.Error("repair requested another page")
			}
			if generated == 2 && !strings.Contains(prompt, repairEvidence) {
				t.Error("repair did not receive calculated error evidence")
			}
			next := sampleDeck(in)
			next.Slides = next.Slides[1:2]
			next.Slides[0].Notes += " 按题设计算：800×10×0.001=8 N。"
			if generated == 1 {
				next.Slides[0].Notes = strings.ReplaceAll(next.Slides[0].Notes, "800×10×0.001=8 N", "800×10×0.001=9 N")
			}
			if physics {
				next.Slides[0].Title = "测力计实验操作"
				next.Slides[0].Notes = "真实实验：使用测力计在空气中称重，重27 N的金属块；牢固固定，竖直调零，缓慢悬挂。数值为示意，课堂另行记录真实读数。"
				next.Slides[0].Bullets = []string{"器材：量程0—30 N、分度值0.2 N的测力计，重27 N的金属块与牢固细线"}
				if generated == 1 {
					next.Slides[0].Bullets[0] = strings.ReplaceAll(next.Slides[0].Bullets[0], "0—30 N", "0—5 N")
				}
			}
			if readGraph || equilibrium || vertex {
				next = disclosureCase.Good
				if generated == 1 {
					next = *disclosureCase.Bad
				}
			}
			answer = string(asJSON(next))
		} else if req.Model == "review" {
			textReviewed++
			prompt := req.Messages[len(req.Messages)-1].Content.(string)
			if (readGraph || equilibrium || vertex) && strings.Contains(prompt, "你是独立解题员") {
				answer = `{"solution":"模拟独立求解，仅验证本地流程。实际题设不足时不可断定浸入状态；两直线交点按联立方程求得。"}`
			}
			if strings.Contains(prompt, "你是教学风险复核员") {
				checks := []map[string]any{}
				for i := 0; i < strings.Count(prompt, "\n复核项"); i++ {
					checks = append(checks, map[string]any{"index": i + 1, "evidence": "模拟审核：已经核对当前页的题设数值和实验条件，仅证明流程契约。", "issues": []PageQualityIssue{}})
				}
				answer = string(asJSON(map[string]any{"checks": checks}))
			}
		}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": answer}}}})
	}))
	defer provider.Close()
	s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "test", "generation"
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = provider.URL, "test", "review"
	s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = provider.URL, "test", "vision"
	sl, _, _, err := s.coursewareSinglePage(context.Background(), a, id, in, nil, d, 2, "", "complete")
	correct := strings.Contains(sl.Notes, "800×10×0.001=8 N")
	if physics {
		correct = len(sl.Bullets) == 1 && strings.Contains(sl.Bullets[0], "0—30 N")
	}
	if readGraph {
		want := disclosureCase.Good.Slides[0]
		correct = sl.Focus == want.Focus && sl.Answer == want.Answer && sl.Notes == want.Notes && string(asJSON(sl.Graph)) == string(asJSON(want.Graph))
	}
	if equilibrium {
		want := disclosureCase.Good.Slides[0]
		correct = sl.Answer == want.Answer && sl.Notes == want.Notes && sl.Focus == want.Focus && string(asJSON(sl.Bullets)) == string(asJSON(want.Bullets))
	}
	if vertex {
		want := disclosureCase.Good.Slides[0]
		correct = sl.Formula == want.Formula && sl.Answer == want.Answer && sl.Notes == want.Notes && sl.Focus == want.Focus && string(asJSON(sl.Bullets)) == string(asJSON(want.Bullets))
	}
	if err != nil || generated != 2 || textReviewed == 0 || !correct {
		t.Fatal("arithmetic error required manual continuation", err, generated, textReviewed)
	}
	if string(asJSON(d)) != before {
		t.Fatal("local repair altered saved neighboring pages")
	}
}

func TestTeachingExerciseBlindSolveHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	id := worksFixture(t, s, token, "courseware", "succeeded", "盲解核对", TaskInput{})
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	calls, broken := 0, false
	firstPrompt := ""
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
			Thinking  map[string]string `json:"thinking"`
			MaxTokens int               `json:"max_tokens"`
		}
		json.NewDecoder(r.Body).Decode(&payload)
		prompt := payload.Messages[1].Content
		answer := `{"issues":[]}`
		if calls%2 == 1 {
			if payload.Thinking["type"] != "enabled" || payload.MaxTokens != 8000 {
				t.Error("independent reasoning mode/budget missing")
			}
			if strings.Contains(prompt, "PRIVATE-ANSWER") || strings.Contains(prompt, "PRIVATE-NOTES") || strings.Contains(prompt, "PRIVATE-LESSON") || strings.Contains(prompt, "PRIVATE-PLAN-ANSWER") {
				t.Error("independent solver received hidden teaching answers")
			}
			if firstPrompt != "" && firstPrompt != prompt {
				t.Error("changing answer changed blind problem")
			}
			firstPrompt = prompt
			answer = `{"solution":"只给一节电池同时点亮两灯，未提供连线，串联和并联均可能，无法确定连接方式。"}`
			if broken {
				answer = `{}`
			}
		} else {
			if payload.Thinking["type"] != "disabled" || payload.MaxTokens != 3000 {
				t.Error("ordinary review mode changed")
			}
			if !strings.Contains(prompt, "无法确定连接方式") || !strings.Contains(prompt, "PRIVATE-ANSWER") {
				t.Error("review missing independent solution or teaching answer")
			}
		}
		respond(w, 200, map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": answer}}}})
	}))
	defer provider.Close()
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = provider.URL, "test", "deepseek-flash"
	d := Deck{Lesson: "PRIVATE-LESSON", Plan: &TeachingPlan{Conventions: "PRIVATE-PLAN-ANSWER"}, Slides: []Slide{{Layout: "exercise", Bullets: []string{"一节电池同时点亮两灯，判断连接方式。"}, Notes: "PRIVATE-NOTES", Answer: "PRIVATE-ANSWER 并联"}}}
	before := string(asJSON(d))
	for i := 0; i < 2; i++ {
		if _, status, err := s.reviewTeachingPage(context.Background(), a, id, TaskInput{Subject: "物理"}, d); err != nil || status != "checked" {
			t.Fatal(status, err)
		}
		d.Slides[0].Answer = "PRIVATE-ANSWER 无法判断"
	}
	if calls != 4 {
		t.Fatal("exercise needs one blind solution and one comparison, no third audit", calls)
	}
	d.Slides[0].Answer = "PRIVATE-ANSWER 并联"
	if string(asJSON(d)) != before {
		t.Fatal("review mutated page")
	}
	broken = true
	if _, status, err := s.reviewTeachingPage(context.Background(), a, id, TaskInput{Subject: "物理"}, d); err == nil || status != "unavailable" || calls != 5 {
		t.Fatal("missing blind solution accepted or retried without bound", status, err, calls)
	}
}
