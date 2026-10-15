package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Actual GLM generation and independent review, English primary excerpt only:
// this is not PEP original-text validation or teacher classroom acceptance.
func TestLiveTeachingEvidenceAssociation(t *testing.T) {
	if os.Getenv("TEST_TEXTBOOK_EVIDENCE_LIVE") != "1" {
		t.Skip("explicit paid source-association opt-in required")
	}
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	in := TaskInput{Type: "courseware", Topic: "一次函数的增减性", Subject: "数学", Grade: "八年级", Pages: 8, Theme: "ocean", UseKB: true, IdempotencyKey: newID()}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	defer worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
	var originals []Source
	raw, err := os.ReadFile("../work/courseware-full-quality/verified-sources.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &originals); err != nil || len(originals) == 0 {
		t.Fatal("verified primary excerpt unavailable")
	}
	src := originals[0]
	src.ID = "S1"
	src.Hash = fmt.Sprintf("%x", sha256.Sum256([]byte(src.Content)))
	src.EvidenceKind = "extracted_page"
	src.Textbook = nil
	sources := []Source{src}
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), "UPDATE tasks SET status='researching',sources=$2 WHERE id=$1", id, asJSON(sources))
	if err != nil {
		tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.cfg.Mode = "live"
	s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = os.Getenv("LLM_BASE_URL"), os.Getenv("LLM_API_KEY"), os.Getenv("LLM_MODEL")
	s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = os.Getenv("COURSEWARE_LLM_BASE_URL"), os.Getenv("COURSEWARE_LLM_API_KEY"), os.Getenv("COURSEWARE_LLM_MODEL")
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = os.Getenv("EVAL_LLM_BASE_URL"), os.Getenv("EVAL_LLM_API_KEY"), os.Getenv("EVAL_LLM_MODEL")
	if !s.agentConfigured("courseware") {
		t.Fatal("live generator not configured")
	}
	dir := os.Getenv("TEST_TEXTBOOK_EVIDENCE_OUTPUT")
	if dir == "" {
		dir = "../work/textbook-evidence/live"
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	transport := s.http.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	var replies atomic.Int32
	s.http.Transport = coursewareTransportFunc(func(r *http.Request) (*http.Response, error) {
		res, e := transport.RoundTrip(r)
		if e != nil || !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			return res, e
		}
		b, e := io.ReadAll(res.Body)
		res.Body.Close()
		res.Body = io.NopCloser(bytes.NewReader(b))
		if e == nil {
			os.WriteFile(filepath.Join(dir, fmt.Sprintf("response-%02d.json", replies.Add(1))), b, 0600)
		}
		return res, e
	})
	t.Cleanup(func() {
		tx, e := s.tx(context.Background(), a.TenantID)
		if e != nil {
			return
		}
		rows, e := queryMaps(context.Background(), tx, "SELECT operation,model,count(*) AS calls,sum(input_tokens) AS input_tokens,sum(output_tokens) AS output_tokens FROM usage_logs WHERE task_id=$1 GROUP BY operation,model", id)
		tx.Rollback(context.Background())
		if e == nil {
			os.WriteFile(filepath.Join(dir, "usage.json"), asJSON(map[string]any{"calls": rows}), 0600)
		}
	})
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Minute)
	defer stop()
	plan := &TeachingPlan{Summary: "一页来源关联验证，OpenStax作为事实参照，不是人教原文。", Pages: []TeachingPage{{Position: 1, Goal: "只讲正斜率时y随x增大而增大，引用真实英语原文并用evidence关联正文。", Task: "concept：两条短要点，不生成作业、练习或高中术语。", Visual: "无须图表；若使用图，仅简短图题。", Minutes: 5, Preview: true}}}
	base := Deck{Title: in.Topic, Theme: in.Theme, Plan: plan}
	sl, title, lesson, err := s.coursewareSinglePage(ctx, a, id, in, sources, base, 1, "保持本页为单个概念任务；引用来源S1的真实英语摘录，至少一项evidence指向本页短正文。不把该事实参照冒充人教教材原文。", "full")
	if err != nil {
		t.Fatal(err)
	}
	d := Deck{Title: title, Theme: in.Theme, Lesson: lesson, Slides: []Slide{sl}}
	if len(d.Slides) != 1 || len(d.Slides[0].Evidence) == 0 {
		t.Fatal("real model omitted claim evidence")
	}
	d.Theme = in.Theme
	results := []map[string]any{}
	os.WriteFile(filepath.Join(dir, "generated-page.json"), asJSON(d), 0600)
	for _, bad := range []bool{false, true} {
		page := d
		if bad {
			claim := "对一次函数，斜率为正时函数递减。"
			page = Deck{Title: "一次函数的增减性", Theme: in.Theme, Slides: []Slide{{Title: "单调性", Layout: "concept", Bullets: []string{claim, "观察随x增大时y的变化。"}, Notes: "教师讲解：正斜率时从左往右函数值变小。依据 [S1]", Evidence: []TeachingClaimEvidence{{Claim: claim, SourceID: "S1", SourceHash: src.Hash, Quote: src.Content}}}}}
		}
		if err = coursewareSourceError(page, sources); err != nil {
			t.Fatal("literal checks should leave semantic judgment to reviewer", err)
		}
		issues, status, e := s.reviewTeachingPage(ctx, a, id, in, page)
		results = append(results, map[string]any{"injected_wrong_claim": bad, "issues": issues, "status": status, "available": e == nil})
		os.WriteFile(filepath.Join(dir, "review-results.json"), asJSON(results), 0600)
		if e != nil || status != "checked" {
			t.Fatal("source semantic review unavailable", e)
		}
		if (blockingPageIssues(issues) != "") != bad {
			t.Fatal("source reviewer misclassified original/contradicting claim", issues)
		}
	}
	ppt, e := s.renderCourseware(ctx, &d)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, "source-associated-page.pptx"), ppt, 0600); e != nil {
		t.Fatal(e)
	}
	var usage map[string]any
	tx, err = s.tx(ctx, a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := queryMaps(ctx, tx, "SELECT operation,model,count(*) AS calls,sum(input_tokens) AS input_tokens,sum(output_tokens) AS output_tokens FROM usage_logs WHERE task_id=$1 GROUP BY operation,model", id)
	tx.Rollback(ctx)
	if err != nil {
		t.Fatal("isolated usage evidence unavailable")
	}
	usage = map[string]any{"calls": rows}
	os.WriteFile(filepath.Join(dir, "usage.json"), asJSON(usage), 0600)
}
