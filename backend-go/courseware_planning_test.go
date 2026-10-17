package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestQualityDoesNotRequireDecorativeMedia(t *testing.T) {
	d := sampleDeck(TaskInput{Topic: "数学", Pages: 8, Theme: "ocean"})
	for i := range d.Slides {
		d.Slides[i].Table = nil
		d.Slides[i].Figure = nil
		d.Slides[i].Chart = nil
		d.Slides[i].Diagram = ""
		d.Slides[i].Graph = nil
	}
	for _, issue := range coursewareQualityIssues(d, 8) {
		if strings.Contains(issue, "至少2页") {
			t.Fatal("visual quota encourages filler", issue)
		}
	}
}

func TestSinglePageReviewIncludesOnlyItsPlanAndConventions(t *testing.T) {
	d := sampleDeck(TaskInput{Topic: "浮力", Pages: 8, Theme: "ocean"})
	d.Plan = &TeachingPlan{Summary: "全课路线", Conventions: "G=2.7 N", Pages: []TeachingPage{{Position: 1, Goal: "未生成的其他页"}, {Position: 7, Goal: "本页控制变量实验"}}}
	d.Slides = d.Slides[:1]
	d.Slides[0].Order = 7
	page := pageForReview(d)
	if len(page.Plan.Pages) != 1 || page.Plan.Pages[0].Position != 7 || page.Plan.Conventions != d.Plan.Conventions {
		t.Fatal("review receives unrelated pages or loses global data")
	}
	if len(d.Plan.Pages) != 2 {
		t.Fatal("review changed original plan")
	}
}

func TestSavedPageContentReviewDoesNotLeakOtherLessonNotes(t *testing.T) {
	d := Deck{Title: "一次函数", Lesson: "第10页其他练习的条件和答案，不得当作本页缺陷。", Evaluation: "旧整套评审：第10页其他练习的条件和答案。", PageChecks: map[string]PageQualityCheck{"other": {Issues: []PageQualityIssue{{Evidence: "第10页其他练习的条件和答案。"}}}}, Slides: []Slide{{Order: 14, Title: "本页作业", Notes: "本页教师讲解与答案。"}}, Plan: &TeachingPlan{Conventions: "本课约定", Pages: []TeachingPage{{Position: 14, Goal: "本页任务"}}}}
	page := pageForReview(d)
	if strings.Contains(string(asJSON(page)), "第10页其他练习") {
		t.Fatal("saved-page audit receives unrelated lesson notes")
	}
	if page.Slides[0].Notes != d.Slides[0].Notes || page.Plan.Conventions != d.Plan.Conventions {
		t.Fatal("current page conditions or conventions lost")
	}
	if d.Lesson == "" {
		t.Fatal("original lesson modified")
	}
}

func TestPlanningTrustAndRepairPosition(t *testing.T) {
	if _, _, err := usageTimes("2026-10-10T00:00:00Z", "2026-10-10T00:00:00Z"); err == nil {
		t.Fatal("equal range accepted")
	}
	d := sampleDeck(TaskInput{Topic: "数学", Pages: 8, Theme: "ocean"})
	d.Plan = &TeachingPlan{Summary: "课堂规划"}
	for i := range d.Slides {
		d.Slides[i].Order = i + 1
		d.Plan.Pages = append(d.Plan.Pages, TeachingPage{Position: i + 1, Goal: d.Slides[i].Title, Task: d.Slides[i].Layout, Minutes: 4, Preview: i == 1 || i == 3 || i == 4})
	}
	d.Slides = d.Slides[:3]
	if err := validateDeck(d); err != nil {
		t.Fatal("short planned lesson rejected", err)
	}
	if row, err := teachingPageFor(d, 2); err != nil || row.Position != 2 {
		t.Fatal("repair must use existing teaching position", row, err)
	}
	restoreTeachingOrder(&d)
	check := PageQualityCheck{Content: "checked", Visual: "unavailable"}
	if pageCheckPassed(check, false) {
		t.Fatal("unavailable visual review accepted")
	}
}

func TestWebSummaryCannotBecomeTeachingEvidence(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"results": []map[string]any{{"title": "浮力资料", "url": "http://127.0.0.1:9/unavailable", "content": "浮力完全浸没后随深度增大。"}}})
	}))
	defer provider.Close()
	s.cfg.Mode = "live"
	s.cfg.SearchURL = provider.URL
	s.cfg.SearchKey = "fixture"
	sources, err := s.retrieve(context.Background(), a, "八年级 物理 浮力", false, true, "浮力")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 0 {
		t.Fatal("search summary leaked as evidence", sources)
	}
}

func TestRepresentativePreviewAndTeachingOrderHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	in := TaskInput{Type: "courseware", Topic: "浮力", Subject: "物理", Detail: "concise", LessonMinutes: 45, ReferenceMode: "content", Theme: "ocean", UseKB: true, PreviewFirst: true, IdempotencyKey: newID()}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	route := "/api/tasks/" + id
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	v := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if v["status"] != "awaiting_preview" {
		t.Fatal("preview failed", v["error"])
	}
	var d Deck
	json.Unmarshal(asJSON(v["result"]), &d)
	if len(d.Slides) != 3 || d.Plan == nil || d.Slides[0].Order != 2 || d.Slides[1].Order != 4 || d.Slides[2].Order != 5 {
		t.Fatal("representative preview not selected", d)
	}
	t.Cleanup(func() { worksCall(t, s, "POST", route+"/cancel", token, nil, 200) })
	saved := map[string]string{}
	for _, sl := range d.Slides {
		saved[sl.ID] = slideContentHash(d, sl)
		worksCall(t, s, "POST", route+"/slides/"+sl.ID+"/review", token, map[string]any{"revision": v["revision"], "hash": saved[sl.ID], "approved": true}, 200)
	}
	worksCall(t, s, "POST", route+"/expand", token, map[string]any{"revision": v["revision"]}, 202)
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	v = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if v["status"] != "awaiting_confirmation" {
		t.Fatal("expansion failed", v["error"])
	}
	d = Deck{}
	json.Unmarshal(asJSON(v["result"]), &d)
	for i, sl := range d.Slides {
		if sl.Order != i+1 {
			t.Fatal("teaching order not restored", i, sl.Order)
		}
		if hash, ok := saved[sl.ID]; ok && hash != slideContentHash(d, sl) {
			t.Fatal("approved preview overwritten", sl.Order, sl, hash, slideContentHash(d, sl), d.Title, d.Theme)
		}
	}
	worksCall(t, s, "POST", route+"/cancel", token, nil, 200)
}

func TestWebEvidenceQuality(t *testing.T) {
	for _, body := range []string{"登录后下载课件，立即购买会员。", "浮力资料", strings.Repeat("点击下载课件 登录后可见\n", 30)} {
		if usableTeachingEvidence(body) {
			t.Fatal("unusable web body accepted", body)
		}
	}
	if !usableTeachingEvidence("阿基米德原理指出，浸在液体中的物体受到竖直向上的浮力，其大小等于它排开的液体所受的重力。计算时应明确液体密度、重力加速度和实际排开液体的体积；全部浸没后体积不再随深度改变。") {
		t.Fatal("valid teaching passage rejected")
	}
}

func TestReviewUnavailableSavesPageAndResumesWithoutRegeneration(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	in := TaskInput{Type: "courseware", Topic: "浮力", Subject: "物理", Detail: "concise", LessonMinutes: 45, ReferenceMode: "content", Theme: "ocean", UseKB: true, PreviewFirst: true, IdempotencyKey: newID()}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), "UPDATE tasks SET status='researching',stage='researching' WHERE id=$1", id)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	plan, err := s.planCourseware(context.Background(), a, id, in, nil)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		d := sampleDeck(TaskInput{Pages: 10, Theme: "ocean", Topic: "浮力"})
		d.Slides = d.Slides[1:2]
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(asJSON(d))}}}})
	}))
	defer provider.Close()
	s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "fixture", "generation"
	d, err := s.stagedCoursewarePlan(context.Background(), a, id, in, nil, Deck{Plan: plan})
	var unavailable pageReviewUnavailable
	if !errors.As(err, &unavailable) || len(d.Slides) != 1 || calls != 1 {
		t.Fatal("unavailable review regenerated or discarded page", err, len(d.Slides), calls)
	}
	hash := slideContentHash(d, d.Slides[0])
	d, err = s.stagedCoursewarePlan(context.Background(), a, id, in, nil, d)
	if !errors.As(err, &unavailable) || calls != 1 || hash != slideContentHash(d, d.Slides[0]) {
		t.Fatal("resume must recheck saved page without regeneration", err, calls)
	}
	if pageCheckPassed(d.PageChecks[d.Slides[0].ID], false) {
		t.Fatal("unavailable marked passed")
	}
	worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
}

func TestStructuredReviewSeverity(t *testing.T) {
	issues, err := parsePageReview(`{"issues":[{"object":"比较面板","evidence":"标注可以更清晰","severity":"suggestion","repair":"增加单位标签"}]}`)
	if err != nil || blockingPageIssues(issues) != "" {
		t.Fatal("aesthetic suggestion blocks", err)
	}
	issues, err = parsePageReview(`{"issues":[{"object":"浮力图表","evidence":"没有给出数据或变化条件，无法支持结论","severity":"error","repair":"依据排液体积计算数据，并明确条件"}]}`)
	if err != nil || blockingPageIssues(issues) == "" {
		t.Fatal("meaningless chart not blocked", err)
	}
	for _, raw := range []string{`{}`, `{"issues":null}`, `{"issues":[{"severity":"error"}]}`} {
		if _, err := parsePageReview(raw); err == nil {
			t.Fatal("incomplete review accepted", raw)
		}
	}
}

func TestRenderedPageReviewUsesActualScreenshot(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	id := worksFixture(t, s, token, "courseware", "succeeded", "画面审核", TaskInput{})
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	issue := PageQualityIssue{Object: "图表", Category: "empty_panel", Certainty: "confirmed", Location: "右侧图表区域", Observed: "只有空白填色框", Expected: "显示页面输入的公式或数据", Evidence: "图表区域没有显示应有内容", Severity: "error", Repair: "恢复该区域应显示的公式或数据"}
	visualCalls := 0
	contractRecovery, recoveryCalls := false, 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Model    string
			Messages []struct{ Content any }
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		issues := []PageQualityIssue{}
		if input.Model == "vision" {
			visualCalls++
			content, ok := input.Messages[0].Content.([]any)
			if !ok || len(content) != 2 || !strings.HasPrefix(content[1].(map[string]any)["image_url"].(map[string]any)["url"].(string), "data:image/png;base64,") {
				t.Error("visual reviewer did not receive actual PPT screenshot")
			}
			issues = append(issues, issue)
			if contractRecovery {
				recoveryCalls++
				if recoveryCalls == 2 {
					if !strings.Contains(content[0].(map[string]any)["text"].(string), "审核结构重查") {
						t.Error("missing bounded recheck instruction")
					}
					issues = []PageQualityIssue{}
				}
			}
		}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(asJSON(map[string]any{"issues": issues}))}}}})
	}))
	defer provider.Close()
	s.cfg.Mode = "live"
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = provider.URL, "fixture", "review"
	s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = provider.URL, "fixture", "vision"
	d := sampleDeck(TaskInput{Pages: 8, Theme: "ocean", Topic: "浮力"})
	d.Slides = d.Slides[1:2]
	pptx, err := s.renderCoursewareAvailable(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	ctx := withCoursewareArtifacts(context.Background())
	check, err := s.inspectGeneratedPage(ctx, a, id, TaskInput{}, d, pptx)
	if err == nil || check.Visual != "checked" || pageCheckPassed(check, false) || visualCalls != 1 {
		t.Fatal("visual error not blocked", err, check, visualCalls)
	}
	issue.Severity = "suggestion"
	check, err = s.inspectGeneratedPage(ctx, a, id, TaskInput{}, d, pptx)
	if err != nil || !pageCheckPassed(check, false) || visualCalls != 2 {
		t.Fatal("aesthetic suggestion blocked", err, check, visualCalls)
	}
	issue.Severity, issue.Certainty = "error", "uncertain"
	check, err = s.inspectGeneratedPage(ctx, a, id, TaskInput{}, d, pptx)
	if err != nil || !pageCheckPassed(check, false) || check.Issues[0].Reviewer != "visual" || check.Issues[0].Severity != "suggestion" {
		t.Fatal("uncertain visual estimate triggered page regeneration", err, check)
	}
	issue.Category, issue.Certainty = "answer", "confirmed"
	beforeCalls := visualCalls
	check, err = s.inspectGeneratedPage(ctx, a, id, TaskInput{}, d, pptx)
	var unavailable pageReviewUnavailable
	if !errors.As(err, &unavailable) || check.Visual != "unavailable" || pageCheckPassed(check, false) || visualCalls != beforeCalls+2 {
		t.Fatal("out-of-scope visual response accepted as a completed review", err, check)
	}
	contractRecovery = true
	check, err = s.inspectGeneratedPage(ctx, a, id, TaskInput{}, d, pptx)
	if err != nil || !pageCheckPassed(check, false) || recoveryCalls != 2 {
		t.Fatal("contract recheck did not recover without regenerating page", err, check, recoveryCalls)
	}
	for _, observed := range []string{"标注完整可见，但底部可能被裁切", "根据刻度估读，点看起来约31而不是32"} {
		issue.Category, issue.Certainty, issue.Severity = "layout", "confirmed", "error"
		issue.Observed = observed
		recoveryCalls = 0
		check, err = s.inspectGeneratedPage(ctx, a, id, TaskInput{}, d, pptx)
		if err != nil || !pageCheckPassed(check, false) || recoveryCalls != 2 {
			t.Fatalf("estimate/risk regenerated correct page instead of rechecking same screenshot: %s calls=%d err=%v", observed, recoveryCalls, err)
		}
	}
}

func TestLiveContentReviewKnownDefects(t *testing.T) {
	if os.Getenv("TEST_LIVE_CONTENT_REVIEW") != "1" {
		t.Skip("explicit live reviewer opt-in required")
	}
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	id := worksFixture(t, s, token, "courseware", "succeeded", "实际内容审核抽测", TaskInput{})
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	s.cfg.Mode = "live"
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = os.Getenv("EVAL_LLM_BASE_URL"), os.Getenv("EVAL_LLM_API_KEY"), os.Getenv("EVAL_LLM_MODEL")
	for _, tc := range []struct {
		name, topic, subject, text string
		bad                        bool
	}{
		{"buoyancy-depth", "浮力", "物理", "物体已完全浸没在密度不变的水中，继续下沉，浮力随深度增大；深度加倍，浮力加倍。", true},
		{"slope-sign", "一次函数", "数学", "函数y=2x+1中，k大于0，因此y随x增大而减小。", true},
		{"correct-computation", "浮力", "物理", "水的密度为1000 kg/m³，g取10 N/kg，完全浸没物体排水体积0.002 m³，浮力为20 N。", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := Deck{Title: tc.topic, Theme: "ocean", Lesson: "依据题设条件讲授，核对公式与单位。", Slides: []Slide{{Title: tc.topic + "的性质", Layout: "concept", Bullets: []string{tc.text}, Notes: tc.text}}}
			issues, status, err := s.reviewTeachingPage(context.Background(), a, id, TaskInput{Topic: tc.topic, Subject: tc.subject, Grade: "八年级", LessonMinutes: 45}, d)
			if err != nil || status != "checked" {
				t.Fatal("actual reviewer unavailable", err)
			}
			blocked := blockingPageIssues(issues) != ""
			if blocked != tc.bad {
				t.Fatal("reviewer missed defect or blocked correct page", tc.name, issues)
			}
			t.Log("actual reviewer classified known fixture correctly; this is a narrow content check, not deck quality acceptance")
		})
	}
}
