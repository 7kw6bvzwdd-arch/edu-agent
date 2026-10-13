package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"testing"
	"time"
)

// Explicit live replay in an isolated test database. Production task/credits
// are never updated; captured renderer inputs contain no provider credentials.
func TestCoursewareRecordedPreviewLive(t *testing.T) {
	replay := os.Getenv("TEST_RECORDED_COURSEWARE")
	if replay != "1" && replay != "candidate" {
		t.Skip("explicit recorded page replay opt-in required")
	}
	s := worksServer(t)
	raw, err := os.ReadFile("../work/product-improvements/latest-courseware-diagnostic.json")
	if err != nil {
		t.Fatal(err)
	}
	var recorded struct {
		Input   TaskInput `json:"input"`
		Result  Deck      `json:"result"`
		Sources []Source  `json:"sources"`
	}
	if err = json.Unmarshal(raw, &recorded); err != nil {
		t.Fatal(err)
	}
	token := worksLogin(t, s, "teacher")
	in := recorded.Input
	in.IdempotencyKey = newID()
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	defer worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	tx, err := s.tx(ctx, a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, "UPDATE tasks SET status='researching',stage='researching',sources=$2 WHERE id=$1", id, asJSON(recorded.Sources))
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	target, _ := url.Parse(s.cfg.RenderURL)
	proxy := httputil.NewSingleHostReverseProxy(target)
	calls := 0
	renderer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/render/pptx" {
			b, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(b))
			calls++
			os.WriteFile(fmt.Sprintf("../work/product-improvements/recorded-page-render-%d.json", calls), b, 0600)
		}
		proxy.ServeHTTP(w, r)
	}))
	defer renderer.Close()
	s.cfg.Mode, s.cfg.RenderURL = "live", renderer.URL
	s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = os.Getenv("LLM_BASE_URL"), os.Getenv("LLM_API_KEY"), os.Getenv("LLM_MODEL")
	s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = os.Getenv("COURSEWARE_LLM_BASE_URL"), os.Getenv("COURSEWARE_LLM_API_KEY"), os.Getenv("COURSEWARE_LLM_MODEL")
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = os.Getenv("EVAL_LLM_BASE_URL"), os.Getenv("EVAL_LLM_API_KEY"), os.Getenv("EVAL_LLM_MODEL")
	s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = os.Getenv("VISION_LLM_BASE_URL"), os.Getenv("VISION_LLM_API_KEY"), os.Getenv("VISION_LLM_MODEL")
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	in.Pages = len(recorded.Result.Plan.Pages)
	recorded.Result.PageChecks = map[string]PageQualityCheck{}
	d := recorded.Result
	if replay == "candidate" {
		candidateRaw, readErr := os.ReadFile("../work/product-improvements/recorded-page-render-1.json")
		if readErr != nil {
			t.Fatal(readErr)
		}
		var candidate Deck
		if err = json.Unmarshal(candidateRaw, &candidate); err != nil {
			t.Fatal(err)
		}
		var pptx []byte
		pptx, err = s.renderCoursewarePage(ctx, &candidate, 1)
		if err == nil {
			candidate.Plan = recorded.Result.Plan
			_, err = s.inspectGeneratedPage(ctx, a, id, in, candidate, pptx)
		}
		d = candidate
	} else {
		d, err = s.stagedCoursewarePlan(ctx, a, id, in, recorded.Sources, recorded.Result)
	}
	if err != nil {
		for _, sl := range d.Slides {
			check := d.PageChecks[sl.ID]
			t.Logf("saved position %d: content=%s visual=%s", sl.Order, check.Content, check.Visual)
		}
		os.WriteFile("../work/product-improvements/recorded-preview-partial.json", asJSON(d), 0600)
		var unavailable pageReviewUnavailable
		if errors.As(err, &unavailable) {
			t.Logf("review unavailable cause: %v", unavailable.Cause)
		}
		t.Fatal(err)
	}
	if replay == "1" {
		if len(d.Slides) != 3 {
			t.Fatal("representative preview incomplete")
		}
		for _, sl := range d.Slides {
			if !pageCheckPassed(d.PageChecks[sl.ID], false) {
				t.Fatal("preview did not pass independent checks")
			}
		}
	}
	os.WriteFile("../work/product-improvements/recorded-page-fixed.json", asJSON(d), 0600)
	pptx, err := s.renderPPTX(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile("../work/product-improvements/recorded-page-fixed.pptx", pptx, 0600)
	if before != worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() {
		t.Fatal("page replay changed accounting")
	}
	t.Logf("%d real planned preview pages rendered and independently reviewed; %d render attempts", len(d.Slides), calls)
}
