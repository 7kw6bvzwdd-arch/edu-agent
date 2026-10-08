package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVisualReviewIsolationAndBlocking(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	id := reservedVideoFixture(t, s, token, a, TaskInput{Type: "animation", Topic: "光的反射", AnimationEngine: "manim", Pages: 8, Theme: "forest", UseKB: true, IdempotencyKey: newID()})
	root := filepath.Join(s.cfg.Storage, "animation-quality", id)
	os.MkdirAll(root, 0700)
	os.WriteFile(filepath.Join(root, "frame-0.jpg"), []byte("image-fixture"), 0600)
	calls := 0
	reply := `{"issues":["箭头穿过文字"]}`
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer vision-only" {
			t.Error("vision credential isolation failed")
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "image-model" || !strings.Contains(string(asJSON(body)), "data:image/jpeg;base64,") {
			t.Error("missing image input")
		}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": reply}}}})
	}))
	defer provider.Close()
	s.cfg.Mode = "live"
	s.cfg.LLMKey = "generation-only"
	s.cfg.ReviewKey = "deepseek-only"
	s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = provider.URL, "vision-only", "image-model"
	d := sampleDeck(TaskInput{Type: "animation", Topic: "光的反射", Pages: 8, Theme: "forest"})
	v := s.reviewAnimationFrames(context.Background(), a, id, d, []QualityFrame{{Index: 0, Second: 1}})
	if v.Status != "issues" || calls != 1 {
		t.Fatal("visible failure lost", v)
	}
	reply = `{"issues":null}`
	if v = s.reviewAnimationFrames(context.Background(), a, id, d, []QualityFrame{{Index: 0}}); v.Status != "unavailable" {
		t.Fatal("malformed review accepted")
	}
	s.cfg.VisionKey = ""
	if v = s.reviewAnimationFrames(context.Background(), a, id, d, []QualityFrame{{Index: 0}}); v.Status != "unavailable" || calls != 2 {
		t.Fatal("fallback key leaked")
	}
	d.AnimationQuality = &AnimationQuality{Version: "scene-v1", Status: "blocked", Visual: v}
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), "UPDATE tasks SET status='awaiting_acceptance',result=$1 WHERE id=$2", asJSON(d), id)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))["balance"]
	worksCall(t, s, "POST", "/api/tasks/"+id+"/accept-animation", token, map[string]any{"accept": true, "revision": 0}, 409)
	worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
	d.AnimationQuality.Status = "accepted"
	d.AnimationQuality.AcceptedBy = a.MemberID
	if err = s.finish(context.Background(), a.TenantID, id, "succeeded", "late acceptance", "", d); err == nil {
		t.Fatal("cancelled task reported successful acceptance")
	}
	if after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))["balance"]; after != before {
		t.Fatal("rejected video charged")
	}
}

func TestAnimationGoldenRender(t *testing.T) {
	if os.Getenv("TEST_ANIMATION_GOLDEN") != "1" {
		t.Skip("opt-in local render with retained review artifacts")
	}
	b, err := os.ReadFile("../fixtures/animation-scenes-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var d Deck
	if err = json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	if err = validateAnimationPlan(d, nil, 0, false); err != nil {
		t.Fatal(err)
	}
	s := &Server{cfg: Config{Storage: "../work/animation-quality-golden", AnimationImage: "edu-manim:local"}}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	path, err := s.runAnimation(ctx, 1, "golden-v1", controlledAnimation(d), 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.prepareAnimationFrames(ctx, "golden-v1", path, d, false, 32)
	if err != nil {
		t.Fatal(err)
	}
	for scene := range d.Slides {
		root := filepath.Join(s.cfg.Storage, "animation-quality", "golden-v1")
		early, err := os.ReadFile(filepath.Join(root, fmt.Sprintf("frame-%d.jpg", scene*3)))
		if err != nil {
			t.Fatal(err)
		}
		middle, err := os.ReadFile(filepath.Join(root, fmt.Sprintf("frame-%d.jpg", scene*3+1)))
		if err != nil || bytes.Equal(early, middle) {
			t.Fatalf("scene %d did not build between keyframes: %v", scene+1, err)
		}
		late, err := os.ReadFile(filepath.Join(root, fmt.Sprintf("frame-%d.jpg", scene*3+2)))
		if err != nil || bytes.Equal(early, late) {
			t.Fatalf("scene %d did not progress between keyframes: %v", scene+1, err)
		}
		if scene == 5 && bytes.Equal(middle, late) {
			t.Fatal("moving scene did not advance after its middle keyframe")
		}
	}
	t.Log("retained:", path)
}

func TestAnimationPlanRepairIsBounded(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	in := TaskInput{Type: "animation", Topic: "一次函数", Pages: 8, Theme: "forest"}
	calls := 0
	validRepair := true
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		format, _ := body["response_format"].(map[string]any)
		if format["type"] != "json_object" {
			t.Error("missing native JSON constraint")
		}
		raw := `{"slides":[{"scene":{"elements":[{"points":[[1,2,3]]}]}}]}`
		if calls%2 == 0 && validRepair {
			raw = string(asJSON(sampleDeck(in)))
		}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": raw}}}})
	}))
	defer provider.Close()
	s.cfg.Mode = "live"
	s.cfg.LLMURL = provider.URL
	s.cfg.LLMKey = "test"
	s.cfg.LLMModel = "test"
	if _, err := s.generate(context.Background(), a, "plan-repair-test", in, nil); err != nil || calls != 2 {
		t.Fatal("repair did not recover invalid shape JSON", err, calls)
	}
	validRepair = false
	calls = 0
	if _, err := s.generate(context.Background(), a, "plan-repair-test", in, nil); err == nil || calls != 2 {
		t.Fatal("repair was not bounded", err, calls)
	}
}

func TestAnimationPlanRepairReceivesAllSceneFailures(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	in := TaskInput{Type: "animation", Topic: "勾股定理", Pages: 8, Theme: "forest"}
	calls := 0
	stillInvalid := false
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&request)
		d := sampleDeck(in)
		if calls%2 == 1 {
			d.Slides[0].Scene.Elements[0].Move = [2]float64{10, 0}
			d.Slides[3].Scene.Evidence = "proof"
		} else {
			prompt := request.Messages[len(request.Messages)-1].Content
			if !strings.Contains(prompt, "第1幕：") || !strings.Contains(prompt, "第4幕：") {
				t.Error("repair must receive both the bounds failure and the unsupported proof")
			}
			if stillInvalid {
				d.Slides[3].Scene.Evidence = "proof"
			}
		}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(asJSON(d))}}}})
	}))
	defer provider.Close()
	s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "test", "test"
	d, err := s.generate(context.Background(), a, "scene-repair-test", in, nil)
	if err != nil || calls != 2 || !d.NeedsReview || d.Slides[3].Scene.Evidence != "illustration" {
		t.Fatal("complete repair did not return a pending-review illustration", err, calls)
	}
	calls, stillInvalid = 0, true
	if _, err = s.generate(context.Background(), a, "scene-repair-test", in, nil); err == nil || calls != 2 {
		t.Fatal("unsupported proof must remain blocked after the single repair", err, calls)
	}
}

func TestAnimationLiveStructuredPlans(t *testing.T) {
	if os.Getenv("TEST_ANIMATION_LIVE") != "1" {
		t.Skip("explicit opt-in for paid text planning; no cloud video")
	}
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	s.cfg.Mode = "live"
	s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = os.Getenv("LLM_BASE_URL"), os.Getenv("LLM_API_KEY"), os.Getenv("LLM_MODEL")
	s.cfg.AnimationLLMURL, s.cfg.AnimationLLMKey, s.cfg.AnimationLLMModel = os.Getenv("ANIMATION_LLM_BASE_URL"), os.Getenv("ANIMATION_LLM_API_KEY"), os.Getenv("ANIMATION_LLM_MODEL")
	for _, topic := range []string{"勾股定理", "浮力"} {
		t.Run(topic, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			in := TaskInput{Type: "animation", AnimationEngine: "manim", Topic: topic, Grade: "八年级", Subject: "数学与物理", Pages: 8, Theme: "midnight"}
			in.Objectives = "围绕一个概念设计可视化；所有图形遵循受控组件契约，不以示意替代证明。"
			d, err := s.generate(ctx, a, "animation-plan-live-"+newID(), in, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err = validateAnimationPlan(d, nil, 0, false); err != nil {
				t.Fatal(err)
			}
			root := "../work/animation-quality-live"
			if err = os.MkdirAll(root, 0700); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(root, topic+".json"), asJSON(d), 0600); err != nil {
				t.Fatal(err)
			}
			local := &Server{cfg: Config{Storage: root, AnimationImage: "edu-manim:local"}}
			id := newID()
			path, err := local.runAnimation(ctx, 1, id, controlledAnimation(d), 0)
			if err != nil {
				t.Fatal(err)
			}
			seconds := 0.0
			for _, sl := range d.Slides {
				seconds += sl.Scene.Duration
			}
			if _, err = local.prepareAnimationFrames(ctx, id, path, d, false, seconds); err != nil {
				t.Fatal(err)
			}
			t.Logf("topic=%s scenes=%d duration=%.1f retained=%s", topic, len(d.Slides), seconds, path)
		})
	}
}
