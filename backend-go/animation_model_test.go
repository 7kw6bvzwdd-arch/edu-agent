package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This provider fixture emits the same points.0 scalar type error seen in the UI.
func animationCoordinateFixture(in TaskInput, mode string) string {
	d := sampleDeck(in)
	var obj map[string]any
	json.Unmarshal(asJSON(d), &obj)
	sl := obj["slides"].([]any)[0].(map[string]any)
	scene := sl["scene"].(map[string]any)
	el := map[string]any{"id": "body", "kind": "rectangle", "label": "物体", "color": "blue", "points": []any{-1.0, -1.0}, "size": []any{1.0, 1.0}, "move": []any{0.0, 0.0}}
	scene["elements"] = append(scene["elements"].([]any), el)
	switch mode {
	case "named-point":
		el["points"] = []any{map[string]any{"x": "-1", "y": "-1"}}
		el["size"] = []any{"1", "1"}
		el["move"] = []any{"0", "0"}
		scene["duration_sec"] = "3"
	case "bad-point", "repairable":
		el["points"] = []any{[]any{1, 2, 3}}
	case "null-point":
		el["points"] = []any{[]any{nil, 0}}
	case "expression":
		el["points"] = []any{[]any{"PRIVATE-EXPRESSION", 0}}
	case "outside":
		el["move"] = []any{10, 0}
	case "bad-proof":
		scene["evidence"] = "proof"
		scene["checks"] = []any{}
	case "ambiguous-flat":
		el["kind"] = "line"
		el["points"] = []any{-1, -1, 1, 1}
	case "multiple-errors":
		el["points"] = []any{1, 2, 3}
		sl2 := obj["slides"].([]any)[3].(map[string]any)
		sl2["scene"].(map[string]any)["elements"].([]any)[0].(map[string]any)["move"] = []any{1, 2, 3}
	}
	return string(asJSON(obj))
}

func TestAnimationCoordinateRecoveryHTTP(t *testing.T) {
	if os.Getenv("TEST_VIDEO_SANDBOX") != "1" {
		t.Skip("requires local Docker video renderer")
	}
	s := worksServer(t)
	s.cfg.AnimationImage = "edu-manim:local"
	if root := os.Getenv("TEST_ANIMATION_COORDINATE_ARTIFACT_DIR"); root != "" {
		s.cfg.Storage = root
	}
	token := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "other")
	in := TaskInput{Type: "animation", AnimationEngine: "manim", Topic: "浮力", Pages: 8, Theme: "forest", UseKB: true, IdempotencyKey: newID()}
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": animationCoordinateFixture(in, "flat-anchor")}}}})
	}))
	defer provider.Close()
	s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "test", "test"
	before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))["balance"].(float64)
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	route := "/api/tasks/" + id
	t.Cleanup(func() { worksCall(t, s, "POST", route+"/cancel", token, nil, 200) })
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	if err := s.workOne(ctx); err != nil {
		t.Fatal(err)
	}
	task := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if task["status"] != "awaiting_confirmation" || calls != 1 {
		t.Fatal("coordinate repair blocked plan", task["error"], calls)
	}
	worksCall(t, s, "POST", route+"/confirm", token, map[string]any{"result": task["result"], "revision": task["revision"], "accept_review": true}, 202)
	if err := s.workOne(ctx); err != nil {
		t.Fatal(err)
	}
	task = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if task["status"] != "awaiting_acceptance" || task["charge_state"] != "reserved" {
		t.Fatal("video not available for acceptance", task["error"])
	}
	video := worksCall(t, s, "GET", route+"/download/mp4", token, nil, 200).Body.Bytes()
	if len(video) < 1000 || string(video[4:8]) != "ftyp" {
		t.Fatal("missing real MP4")
	}
	worksCall(t, s, "GET", route+"/download/mp4", other, nil, 404)
	worksCall(t, s, "GET", route+"/frames/0", token, nil, 200)
	if root := os.Getenv("TEST_ANIMATION_COORDINATE_ARTIFACT_DIR"); root != "" {
		if err := os.WriteFile(filepath.Join(root, "coordinate-recovery.mp4"), video, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))["balance"].(float64); got != before {
		t.Fatal("charged before video acceptance")
	}
	worksCall(t, s, "POST", route+"/accept-animation", token, map[string]any{"accept": true, "revision": task["revision"]}, 200)
	worksCall(t, s, "POST", route+"/accept-animation", token, map[string]any{"accept": true, "revision": task["revision"]}, 409)
	if got := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))["balance"].(float64); got != before-40 {
		t.Fatal("video settlement was not exactly 40 credits")
	}
}

func TestAnimationCoordinateRecovery(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	in := TaskInput{Type: "animation", AnimationEngine: "manim", Topic: "浮力", Pages: 8, Theme: "forest"}
	for _, mode := range []string{"flat-anchor", "named-point", "repairable", "bad-point", "null-point", "expression", "outside", "bad-proof", "ambiguous-flat", "multiple-errors"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if mode == "multiple-errors" && calls == 2 {
					var body struct{ Messages []struct{ Content string } }
					json.NewDecoder(r.Body).Decode(&body)
					prompt := body.Messages[len(body.Messages)-1].Content
					if !strings.Contains(prompt, "第1幕") || !strings.Contains(prompt, "第4幕") {
						t.Error("repair missed structural errors in another scene")
					}
				}
				fixtureMode := mode
				if mode == "repairable" && calls == 2 {
					fixtureMode = "flat-anchor"
				}
				respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": animationCoordinateFixture(in, fixtureMode)}}}})
			}))
			defer provider.Close()
			s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "test", "test"
			d, err := s.generate(context.Background(), a, "animation-coordinate-test", in, nil)
			success := mode == "flat-anchor" || mode == "named-point" || mode == "repairable"
			want := 2
			if mode == "flat-anchor" || mode == "named-point" {
				want = 1
			}
			if calls != want {
				t.Fatalf("model calls=%d want=%d", calls, want)
			}
			if success {
				if err != nil {
					t.Fatal("recoverable coordinates blocked animation", err)
				}
				if err := validateAnimationPlan(d, nil, 0, false); err != nil {
					t.Fatal(err)
				}
				el := d.Slides[0].Scene.Elements[1]
				if len(el.Points) != 1 || el.Points[0] != [2]float64{-1, -1} || el.Size != [2]float64{1, 1} {
					t.Fatal("geometry changed")
				}
			} else {
				if err == nil {
					t.Fatal("ambiguous or invalid geometry accepted")
				}
				for _, leak := range []string{"cannot unmarshal", "[]float64", "PRIVATE-EXPRESSION"} {
					if strings.Contains(err.Error(), leak) {
						t.Fatal("raw error leaked", err)
					}
				}
				if !strings.Contains(err.Error(), "第1幕") {
					t.Fatal("scene diagnostic missing", err)
				}
			}
		})
	}
}

func TestAnimationInvalidCoordinatesReleaseHTTP(t *testing.T) {
	if os.Getenv("TEST_VIDEO_SANDBOX") != "1" {
		t.Skip("animation HTTP creation checks the local image")
	}
	s := worksServer(t)
	s.cfg.AnimationImage = "edu-manim:local"
	token := worksLogin(t, s, "teacher")
	in := TaskInput{Type: "animation", AnimationEngine: "manim", Topic: "浮力", Pages: 8, Theme: "forest", UseKB: true, IdempotencyKey: newID()}
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		mode := "bad-point"
		if calls == 2 {
			mode = "outside"
		}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": animationCoordinateFixture(in, mode)}}}})
	}))
	defer provider.Close()
	s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "test", "test"
	before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	task := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
	if task["status"] != "failed" || task["charge_state"] != "released" || calls != 2 {
		t.Fatal("semantic failure after format repair must release, without third model call", task["error"], calls)
	}
	if !strings.Contains(task["error"].(string), "第1幕") {
		t.Fatal("scene-specific reason missing")
	}
	found := false
	for _, entry := range task["events"].([]any) {
		ev := entry.(map[string]any)
		if ev["stage"] == "repairing" && strings.Contains(ev["message"].(string), "points") {
			found = true
		}
	}
	if !found {
		t.Fatal("repair diagnostic wasn't persisted")
	}
	after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	if after["balance"] != before["balance"] || after["available"] != before["available"] {
		t.Fatal("failed animation charged or retained reservation")
	}
}
