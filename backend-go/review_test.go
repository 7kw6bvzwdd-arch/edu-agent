package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestPlannedCoursewareReportUsesCurrentPageChecks(t *testing.T) {
	s := &Server{}
	s.cfg.Mode = "live"
	d := Deck{Title: "一次函数", Plan: &TeachingPlan{}, Slides: []Slide{{ID: "page-one", Title: "斜率", Order: 1}}, PageChecks: map[string]PageQualityCheck{}}
	report := s.assessTeaching(context.Background(), Actor{}, "", "courseware", d, nil)
	if report["status"] != "unavailable" {
		t.Fatal("unchecked page reported as reviewed", report)
	}
	d.PageChecks["page-one"] = PageQualityCheck{Hash: slideContentHash(d, d.Slides[0]), Content: "checked", Visual: "checked"}
	report = s.assessTeaching(context.Background(), Actor{}, "", "courseware", d, nil)
	if report["status"] != "completed" || report["rubric_version"] != "page-review-v1" {
		t.Fatal("existing checks were not summarized", report)
	}
	d.Slides[0].Title = "修改后的斜率"
	if s.assessTeaching(context.Background(), Actor{}, "", "courseware", d, nil)["status"] != "unavailable" {
		t.Fatal("stale checks reported as reviewed")
	}
}

func TestIndependentReviewRoutingAndValidation(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	calls := 0
	bad := false
	picture := teachingTestPicture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer reviewer-only" {
			t.Error("review credentials routed incorrectly")
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if strings.Contains(string(asJSON(body)), picture.DataURL) {
			t.Error("text reviewer received uploaded image bytes")
		}
		format, _ := body["response_format"].(map[string]any)
		if body["model"] != "deepseek-flash" || format["type"] != "json_object" {
			t.Error("review JSON contract missing")
		}
		thinking, _ := body["thinking"].(map[string]any)
		if thinking["type"] != "disabled" {
			respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "length", "message": map[string]string{"content": ""}}}})
			return
		}
		response := map[string]any{"summary": "检查来源及适用条件", "scores": map[string]int{"knowledge": 4, "load": 4, "consistency": 3, "timing": 4, "formula": 3}, "issues": []map[string]any{{"slide": 1, "issue": "条件需核对", "suggestion": "保留液体密度近似不变", "source_ids": []string{"S1"}}}}
		if bad {
			response["scores"] = map[string]int{"knowledge": 100}
		}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(asJSON(response))}}}})
	}))
	defer server.Close()
	s.cfg.Mode = "live"
	s.cfg.LLMURL = "http://127.0.0.1:1"
	s.cfg.LLMKey = "generation-only"
	s.cfg.LLMModel = "generation"
	s.cfg.ReviewURL = server.URL
	s.cfg.ReviewKey = "reviewer-only"
	s.cfg.ReviewModel = "deepseek-flash"
	d := sampleDeck(TaskInput{Topic: "浮力", Pages: 8, Theme: "forest"})
	d.Slides[0].Picture = picture
	sources := []Source{{ID: "S1", Content: "液体密度近似不变时，浮力等于排开液体的重力。"}}
	report := s.assessTeaching(context.Background(), a, "review-check", "animation", d, sources)
	if report["status"] != "completed" || calls != 1 {
		t.Fatal("review unavailable", report, calls)
	}
	bad = true
	report = s.assessTeaching(context.Background(), a, "review-check", "animation", d, sources)
	if report["status"] != "unavailable" {
		t.Fatal("malformed review accepted", report)
	}
	s.cfg.ReviewKey = ""
	report = s.assessTeaching(context.Background(), a, "review-check", "animation", d, sources)
	if calls != 2 || report["status"] != "unavailable" {
		t.Fatal("partial config leaked generation key")
	}
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = "", "", ""
	s.cfg.EvalModel = "legacy-reviewer"
	url, key, model := s.reviewModel("animation")
	if url != "" || key != "" || model != "" {
		t.Fatal("missing review config borrowed generation credentials")
	}
	ledger := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	if strings.Contains(ledger, "review-check") {
		t.Fatal("review charged business credits")
	}
}

// Explicit opt-in: one small paid text review of synthetic material, no video or business charge.
func TestDeepSeekLiveReview(t *testing.T) {
	if os.Getenv("TEST_DEEPSEEK_LIVE") != "1" {
		t.Skip("set TEST_DEEPSEEK_LIVE=1 and independent EVAL_LLM_* credentials")
	}
	s := worksServer(t)
	token := worksLogin(t, s, "admin")
	var a Actor
	if err := json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	s.cfg.Mode = "live"
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = os.Getenv("EVAL_LLM_BASE_URL"), os.Getenv("EVAL_LLM_API_KEY"), os.Getenv("EVAL_LLM_MODEL")
	d := sampleDeck(TaskInput{Topic: "浮力", Pages: 8, Theme: "forest"})
	d.Slides = d.Slides[:1]
	d.Slides[0].Bullets = []string{"错误示例：物体完全浸没后继续下沉，浮力随深度线性增大。"}
	sources := []Source{{ID: "S1", Content: "同一液体中物体完全浸没后，密度和物体体积不变时，浮力不随深度增加。F浮=ρ液gV排。"}}
	id := worksFixture(t, s, token, "courseware", "succeeded", "DeepSeek评审联调", TaskInput{})
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), "UPDATE tasks SET result=$1,sources=$2 WHERE id=$3", asJSON(d), asJSON(sources), id)
	if err != nil {
		tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	path := "/api/tasks/" + id
	worksCall(t, s, "POST", path+"/review", token, nil, 200)
	result := worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
	report := result["result"].(map[string]any)["evaluation"].(map[string]any)
	if report["status"] != "completed" || report["reviewed_revision"] != result["revision"] {
		t.Fatal("official review was not saved against the current revision", report)
	}
	assessment := report["assessment"].(map[string]any)
	if len(assessment["issues"].([]any)) == 0 {
		t.Fatal("review missed intentionally incorrect material")
	}
	if after := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String(); after != before {
		t.Fatal("review changed business credits")
	}
	t.Log(string(asJSON(report)))
}
