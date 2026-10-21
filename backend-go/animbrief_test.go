package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Exercise the teacher-facing boundary: a missing opening must trigger bounded
// repair before a plan is presented, while an explicit direct start is respected.
func TestAnimationOpeningContractHTTP(t *testing.T) {
	for _, opening := range []string{"natural", "direct"} {
		t.Run(opening, func(t *testing.T) {
			s := worksServer(t)
			s.cfg.Mode = "live"
			script := directModelFixture(t)
			calls := 0
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls > 1 {
					script.Scenes[0].Narration[0].Display = "同学们，今天我们一起观察一次函数的图像。"
					script.Scenes[0].Narration[0].Speech = script.Scenes[0].Narration[0].Display
				}
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(asJSON(script))}}}})
			}))
			defer provider.Close()
			s.cfg.AnimationLLMURL, s.cfg.AnimationLLMKey, s.cfg.AnimationLLMModel = provider.URL, "fixture", "fixture"
			s.cfg.TTS = ttsConfig{Provider: "volcengine", Protocol: "create", URL: provider.URL + "/tts", Key: "fixture", Model: "seed-audio-1.0", Unit: "sentence", Format: "mp3", SampleRate: 48000}
			token := worksLogin(t, s, "teacher")
			before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
			input := map[string]any{"type": "animation", "topic": script.Title, "animation_opening": opening, "idempotency_key": newID()}
			id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, input, 202))["id"].(string)
			if err := s.workOne(context.Background()); err != nil {
				t.Fatal(err)
			}
			path := "/api/tasks/" + id + "/animation"
			state := worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
			want := 2
			if opening == "direct" {
				want = 1
			}
			if state["status"] != "anim_plan" || calls != want {
				t.Fatalf("status=%v calls=%d want=%d", state["status"], calls, want)
			}
			stored := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))["input"].(map[string]any)
			if stored["animation_opening"] != opening {
				t.Fatal("opening preference was not persisted")
			}
			if opening == "natural" {
				// Manual JSON must not bypass the same authoring gate.
				script.Scenes[0].Narration[0].Display = "这是一次函数的图像。"
				script.Scenes[0].Narration[0].Speech = script.Scenes[0].Narration[0].Display
				worksCall(t, s, "POST", path+"/revise-plan", token, map[string]any{"revision": state["revision"], "request_key": newID(), "script": script}, 400)
			}
			after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
			if before["balance"] != after["balance"] || before["reserved"] != after["reserved"] {
				t.Fatal("opening repair changed quota")
			}
			worksCall(t, s, "POST", path+"/cancel", token, map[string]any{"revision": state["revision"]}, 200)
		})
	}
}

// Provider audio is deliberately shorter than the requested range. The teacher
// must be able to repair the plan without paying or losing completed audio.
func TestAnimationMeasuredDurationHTTP(t *testing.T) {
	s := worksServer(t)
	s.cfg.Mode = "live"
	script := directModelFixture(t)
	script.Scenes[0].Narration[0].Display = "同学们，今天我们一起观察一次函数的图像。"
	script.Scenes[0].Narration[0].Speech = script.Scenes[0].Narration[0].Display
	calls, ttsCalls := 0, 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tts" {
			ttsCalls++
			json.NewEncoder(w).Encode(map[string]any{"audio": base64.StdEncoding.EncodeToString(testMP3(250)), "subtitle": map[string]any{"sentences": []any{map[string]any{"text": "", "start_time": 240, "end_time": 5600}}}})
			return
		}
		calls++
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(asJSON(script))}}}})
	}))
	defer provider.Close()
	s.cfg.AnimationLLMURL, s.cfg.AnimationLLMKey, s.cfg.AnimationLLMModel = provider.URL, "fixture", "fixture"
	s.cfg.TTS = ttsConfig{Provider: "volcengine", Protocol: "create", URL: provider.URL + "/tts", Key: "fixture", Model: "seed-audio-1.0", Unit: "sentence", Format: "mp3", SampleRate: 48000}
	token := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "other")
	before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	input := map[string]any{"type": "animation", "topic": script.Title, "animation_min_seconds": 90, "animation_max_seconds": 120, "idempotency_key": newID()}
	invalid := map[string]any{"type": "animation", "topic": script.Title, "animation_min_seconds": 120, "animation_max_seconds": 90, "idempotency_key": newID()}
	worksCall(t, s, "POST", "/api/tasks", token, invalid, 400)
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, input, 202))["id"].(string)
	path := "/api/tasks/" + id + "/animation"
	state := func() map[string]any { return worksObject(t, worksCall(t, s, "GET", path, token, nil, 200)) }
	run := func() map[string]any {
		t.Helper()
		if err := s.workOne(context.Background()); err != nil {
			t.Fatal(err)
		}
		return state()
	}
	v := run()
	if v["status"] != "anim_plan" {
		t.Fatal(v)
	}
	worksCall(t, s, "POST", path+"/confirm-plan", token, map[string]any{"revision": v["revision"]}, 202)
	v = run()
	if v["status"] != "anim_representative" {
		t.Fatal("full-duration target applied to representative", v["status"])
	}
	worksCall(t, s, "POST", path+"/confirm-representative", token, map[string]any{"revision": v["revision"], "hash": v["representative_hash"]}, 202)
	v = run()
	if v["status"] != "failed" || v["charge_state"] != "released" || v["has_delivery"] != false {
		t.Fatal("out-of-range audio reached acceptance", v["status"], v["charge_state"])
	}
	if !strings.Contains(string(asJSON(v["problems"])), "90") || v["duration_ms"].(float64) < 30000 {
		t.Fatal("missing measured-duration feedback", v["problems"])
	}
	after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	if before["balance"] != after["balance"] || before["reserved"] != after["reserved"] {
		t.Fatal("duration failure changed quota")
	}
	worksCall(t, s, "POST", path+"/accept", token, map[string]any{"revision": v["revision"], "hash": v["build_hash"]}, 409)
	// Explicitly repair only the summary's teaching observation holds. No hidden
	// silence is inserted by the program and the approved representative is reused.
	n := script.Scenes[3].Narration[1].ID
	script.Scenes[3].Holds = []AnimHold{{ID: "observe-one", Type: "observe", After: n, MS: 10000}, {ID: "observe-two", Type: "ask", After: n, MS: 10000}, {ID: "observe-three", Type: "await", After: n, MS: 10000}}
	body := map[string]any{"revision": v["revision"], "request_key": newID(), "script": script}
	worksCall(t, s, "POST", path+"/revise-plan", other, body, 404)
	worksCall(t, s, "POST", path+"/revise-plan", token, body, 202)
	v = state()
	worksCall(t, s, "POST", path+"/confirm-plan", token, map[string]any{"revision": v["revision"]}, 202)
	v = run()
	if v["status"] != "anim_acceptance" || ttsCalls != 10 || calls != 1 {
		t.Fatal("repair did not preserve audio/representative", v["status"], ttsCalls, calls)
	}
	if d := v["duration_ms"].(float64); d < 90000 || d > 120000 {
		t.Fatal("repair duration", d)
	}
	worksCall(t, s, "POST", path+"/accept", token, map[string]any{"revision": v["revision"], "hash": v["build_hash"]}, 200)
	worksCall(t, s, "GET", "/api/tasks/"+id+"/download/html", token, nil, 200)
	after = worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	if after["balance"].(float64) != before["balance"].(float64)-40 || after["reserved"] != before["reserved"] {
		t.Fatal("repair settled more than once")
	}
}
