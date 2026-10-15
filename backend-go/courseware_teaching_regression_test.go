package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type teachingRegressionCase struct {
	ID     string    `json:"id"`
	Input  TaskInput `json:"input"`
	Origin struct {
		Path, SHA256, Variant string
		Page                  int
	} `json:"origin"`
	Categories    []string `json:"categories"`
	EvidenceTerms []string `json:"evidence_terms"`
	Bad           *Deck    `json:"bad"`
	Good          Deck     `json:"good"`
}

func teachingRegressionCases(t *testing.T) []teachingRegressionCase {
	t.Helper()
	var cases []teachingRegressionCase
	for _, path := range []string{"testdata/teaching-review-regressions.json", "testdata/teaching-review-holdouts.json", "testdata/teaching-review-live-20261015.json"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var batch []teachingRegressionCase
		if err = json.Unmarshal(raw, &batch); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, batch...)
	}
	return cases
}

func TestTeachingRegressionCorpus(t *testing.T) {
	ids := map[string]bool{}
	for _, tc := range teachingRegressionCases(t) {
		if tc.ID == "" || ids[tc.ID] || tc.Origin.Path == "" || len(tc.Origin.SHA256) != 64 || tc.Origin.Page < 1 || tc.Origin.Variant == "" {
			t.Fatalf("missing identity/provenance: %+v", tc.Origin)
		}
		ids[tc.ID] = true
		decks := []Deck{tc.Good}
		if tc.Bad != nil {
			decks = append(decks, *tc.Bad)
			if len(tc.Categories) == 0 || len(tc.EvidenceTerms) == 0 || string(asJSON(tc.Bad)) == string(asJSON(tc.Good)) {
				t.Fatal("invalid positive/negative pair", tc.ID)
			}
		}
		for _, d := range decks {
			if len(d.Slides) != 1 || strings.Contains(string(asJSON(pageForReview(d))), "page_checks") {
				t.Fatal("fixture leaks old checks", tc.ID)
			}
		}
	}
	if len(ids) < 10 {
		t.Fatal("observed regression classes missing")
	}
}

func TestReviewDoesNotTreatVisualProposalAsRenderedFacts(t *testing.T) {
	d := Deck{Slides: []Slide{{Order: 3, Notes: "G=5 N，F示=3 N，F浮=2 N"}}, Plan: &TeachingPlan{Conventions: "同一例题G=5 N，F示=3 N，F浮=2 N", Pages: []TeachingPage{{Position: 3, Goal: "解释称重法", Task: "区分三力", Visual: "拟议示例F示=3.5 N，F浮=1.5 N"}}}}
	raw := string(asJSON(pageForReview(d)))
	if strings.Contains(raw, "3.5 N") || !strings.Contains(raw, "F示=3 N") || d.Plan.Pages[0].Visual == "" {
		t.Fatal("unused visual proposal became evidence, or original plan was mutated", raw)
	}
}

func TestContentReviewRejectsContradictoryError(t *testing.T) {
	raw := `{"issues":[{"object":"体积换算","category":"answer","certainty":"confirmed","severity":"error","evidence":"换算正确，故该句无误","repair":"无需修改；返回空issues。"}]}`
	if _, err := parsePageReview(raw, "content"); err == nil {
		t.Fatal("self-contradictory error would regenerate a correct page")
	}
}

func TestContentReviewCannotInventVisibleAnswerGraph(t *testing.T) {
	d := Deck{Slides: []Slide{{Notes: "参考图：y=2x+1（教师核对用）；本页不显示答案图。"}}}
	issue := PageQualityIssue{Object: "notes中参考图", Severity: "error", Evidence: "完整曲线构成答案泄露"}
	if _, err := reconcileTeachingContentIssues([]PageQualityIssue{issue}, d); err == nil {
		t.Fatal("hidden notes invented a visible graph")
	}
	d.Slides[0].Graph = &FunctionGraph{}
	if issues, err := reconcileTeachingContentIssues([]PageQualityIssue{issue}, d); err != nil || blockingPageIssues(issues) == "" {
		t.Fatal("actual visible graph leak suppressed", err)
	}
	d.Slides[0].Graph = nil
	issue.Evidence = "缺少学习函数性质所需的参考图，建议补充。"
	if _, err := reconcileTeachingContentIssues([]PageQualityIssue{issue}, d); err != nil {
		t.Fatal("missing graph is a different semantic issue", err)
	}
}

func TestContentReviewChecksActualEmphasisAndApproximation(t *testing.T) {
	for _, tc := range teachingRegressionCases(t) {
		if tc.ID == "count-domain-segment" {
			issue := PageQualityIssue{Object: "notes与emphasis中的线段", Category: "consistency", Severity: "error", Evidence: "若强调对象为连续线段则与离散点矛盾，需核对实际强调所指。"}
			if _, err := reconcileTeachingContentIssues([]PageQualityIssue{issue}, tc.Good); err == nil {
				t.Fatal("unused emphasis token treated as displayed text")
			}
		}
		if tc.ID == "independent-graph-answer-leak" {
			issue := PageQualityIssue{Object: "answer中的交点估计", Category: "answer", Severity: "error", Evidence: "估计值不应写成精确分数坐标。"}
			if _, err := reconcileTeachingContentIssues([]PageQualityIssue{issue}, tc.Good); err == nil {
				t.Fatal("approximate reference to rational coordinate treated as numerical error")
			}
			issue.Evidence = "交点坐标代入函数后不相等，答案数值错误。"
			if _, err := reconcileTeachingContentIssues([]PageQualityIssue{issue}, tc.Good); err != nil {
				t.Fatal("actual numeric objection suppressed", err)
			}
		}
	}
}

func TestContentReviewCannotInventRealExperiment(t *testing.T) {
	d := Deck{Slides: []Slide{{Title: "理想电路计算", Notes: "本题为理想计算，不要求实际接线。若用于真实演示，须另行核对额定功率。"}}}
	issue := PageQualityIssue{Object: "若用于真实演示，须另行核对额定功率", Category: "conditions", Severity: "error", Evidence: "未提供额定参数，真实演示条件不足", Repair: "补充额定参数"}
	if _, err := reconcileTeachingContentIssues([]PageQualityIssue{issue}, d); err == nil {
		t.Fatal("hypothetical safety reminder became a real experiment")
	}
	d.Slides[0].Bullets = []string{"课堂真实实验：连接电源，观察发热。"}
	if _, err := reconcileTeachingContentIssues([]PageQualityIssue{issue}, d); err != nil {
		t.Fatal("real experimental condition issue suppressed", err)
	}
}

func TestTeachingVisibilityEvidenceKeepsNotesHidden(t *testing.T) {
	d := Deck{Title: "函数", Slides: []Slide{{Title: "独立作图", Bullets: []string{"画y=2x+1"}, Notes: "PRIVATE-TEACHER-ANSWER 参考图y=2x+1", Emphasis: []string{"PRIVATE-STYLE-HINT"}}}}
	before := string(asJSON(d))
	evidence := string(asJSON(teachingVisibilityEvidence(d)))
	if strings.Contains(evidence, "PRIVATE-TEACHER-ANSWER") || strings.Contains(evidence, "PRIVATE-STYLE-HINT") || !strings.Contains(evidence, `"graph_present":false`) || !strings.Contains(evidence, "画y=2x+1") || before != string(asJSON(d)) {
		t.Fatal("visibility inventory leaked hidden answers or changed page", evidence)
	}
	if strings.Contains(evidence, `"notes":`) || strings.Contains(evidence, `"answer":`) {
		t.Fatal("hidden teacher fields presented as empty visible objects", evidence)
	}
}

func TestContentReviewOmitsStyleMetadata(t *testing.T) {
	d := Deck{Slides: []Slide{{Title: "离散点", Bullets: []string{"图象由11个离散点组成"}, Emphasis: []string{"UNUSED-STYLE-TOKEN"}, Notes: "教师解释：不能连接成连续线段", Answer: "11个离散点"}}}
	before := string(asJSON(d))
	page := string(asJSON(pageForReview(d)))
	if strings.Contains(page, "UNUSED-STYLE-TOKEN") || !strings.Contains(page, "教师解释") || !strings.Contains(page, "11个离散点") || string(asJSON(d)) != before {
		t.Fatal("content reviewer received unused style tokens or lost teaching content", page)
	}
}

func TestTeachingRiskSelectionKeepsRelatedChecksTogether(t *testing.T) {
	for _, tc := range []struct{ name, subject, notes, layout, term string }{
		{"units", "物理", "1000 cm3直接代入，结果小1000倍", "example", "单位换算"},
		{"force", "物理", "重力27 N，测力计拉力17 N", "example", "受力与实验"},
		{"meter", "物理", "电压表量程0—3V，待测6V", "steps", "电路"},
		{"exercise", "数学", "答案中给出题目未给的条件", "exercise", "练习"},
		{"measured", "物理", "称示例数据为实测", "concept", "来源"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			check := teachingRiskChecklist(TaskInput{Subject: tc.subject}, Deck{Slides: []Slide{{Notes: tc.notes, Layout: tc.layout}}})
			if !strings.Contains(check, tc.term) {
				t.Fatal("risk not selected", check)
			}
		})
	}
	if check := teachingRiskChecklist(TaskInput{Subject: "数学"}, Deck{Slides: []Slide{{Title: "学习目标", Bullets: []string{"认识变量"}}}}); check != "" {
		t.Fatal("unrelated page should not need focused check", check)
	}
}

func TestFocusedTeachingReviewRequiresEveryCheckAndEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, reply string
		required    int
		valid       bool
	}{
		{"generic-empty", `{"issues":[]}`, 1, false},
		{"missing-check", `{"checks":[]}`, 1, false},
		{"no-reason", `{"checks":[{"index":1,"evidence":"正确","issues":[]}]}`, 1, false},
		{"duplicate", `{"checks":[{"index":1,"evidence":"已独立核对本页可见题设和教师答案，具体数量关系成立。","issues":[]},{"index":1,"evidence":"已独立核对本页可见题设和教师答案，具体数量关系成立。","issues":[]}]}`, 2, false},
		{"complete", `{"checks":[{"index":1,"evidence":"6V测量使用15V量程，数值在量程内且保留换接前断电说明。","issues":[]}]}`, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseFocusedTeachingReview(tc.reply, tc.required)
			if (err == nil) != tc.valid {
				t.Fatal("invalid review completeness decision", err)
			}
		})
	}
}

type teachingReviewTrace struct {
	base        http.RoundTripper
	dir, prefix string
	calls       int
}

func (trace *teachingReviewTrace) RoundTrip(r *http.Request) (*http.Response, error) {
	res, err := trace.base.RoundTrip(r)
	if err != nil {
		return res, err
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 25<<20))
	res.Body.Close()
	if err != nil {
		return nil, err
	}
	res.Body = io.NopCloser(bytes.NewReader(body))
	trace.calls++
	// Response envelopes only; never record requests, credentials or headers.
	name := filepath.Join(trace.dir, trace.prefix+"-"+time.Now().Format("150405.000000")+".json")
	if err = os.WriteFile(name, body, 0600); err != nil {
		return nil, err
	}
	return res, nil
}

// This is a paid reviewer boundary test in an isolated database, not a new
// automatically generated deck or an estimate of classroom content accuracy.
func TestLiveTeachingRegressionCorpus(t *testing.T) {
	if os.Getenv("TEST_TEACHING_REVIEW_LIVE") != "1" {
		t.Skip("explicit live review test opt-in required")
	}
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	id := worksFixture(t, s, token, "courseware", "succeeded", "教学审核回归", TaskInput{})
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	s.cfg.Mode = "live"
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = os.Getenv("EVAL_LLM_BASE_URL"), os.Getenv("EVAL_LLM_API_KEY"), os.Getenv("EVAL_LLM_MODEL")
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	results := []map[string]any{}
	dir := os.Getenv("TEST_TEACHING_REVIEW_OUTPUT")
	if dir == "" {
		dir = "../work/teaching-review-regressions"
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	base := s.reviewHTTP.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	trace := &teachingReviewTrace{base: base, dir: dir}
	s.reviewHTTP.Transport = trace
	defer func() {
		if err := os.WriteFile(filepath.Join(dir, "results.json"), asJSON(results), 0600); err != nil {
			t.Error(err)
		}
	}()
	for _, tc := range teachingRegressionCases(t) {
		if filter := os.Getenv("TEST_TEACHING_REVIEW_CASES"); filter != "" && !strings.Contains(","+filter+",", ","+tc.ID+",") {
			continue
		}
		for _, variant := range []string{"bad", "good"} {
			if variant == "bad" && tc.Bad == nil {
				continue
			}
			t.Run(tc.ID+"/"+variant, func(t *testing.T) {
				trace.prefix = tc.ID + "-" + variant
				d := tc.Good
				if variant == "bad" {
					d = *tc.Bad
				}
				started := time.Now()
				ctx, stop := context.WithTimeout(context.Background(), 2*time.Minute)
				issues, status, err := s.reviewTeachingPage(ctx, a, id, tc.Input, d)
				stop()
				targetDetected := false
				for _, issue := range issues {
					if issue.Severity != "error" || !strings.Contains("|"+strings.Join(tc.Categories, "|")+"|", "|"+issue.Category+"|") {
						continue
					}
					for _, term := range tc.EvidenceTerms {
						if strings.Contains(issue.Object+issue.Evidence+issue.Repair, term) {
							targetDetected = true
						}
					}
				}
				passed := status == "checked" && err == nil && ((variant == "bad" && targetDetected) || (variant == "good" && blockingPageIssues(issues) == ""))
				results = append(results, map[string]any{"case": tc.ID, "variant": variant, "status": status, "available": err == nil, "target_detected": targetDetected, "passed": passed, "issues": issues, "elapsed_ms": time.Since(started).Milliseconds()})
				if !passed {
					t.Errorf("incorrect review: status=%s available=%t target=%t err=%v issues=%+v", status, err == nil, targetDetected, err, issues)
				}
			})
		}
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before {
		t.Fatal("review test changed business points")
	}
	if len(results) == 0 {
		t.Fatal("no regression cases selected; this run did not validate any page")
	}
}
