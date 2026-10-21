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
)

func TestAnimGenerateCLI(t *testing.T) {
	fixture := directModelFixture(t)
	render := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"version": "authored-animation-v1"})
	}))
	defer render.Close()
	t.Setenv("RENDER_SERVICE_URL", render.URL)
	fixture.Scenes[0].Narration[0].Display = "同学们，今天我们一起观察一次函数的图像。"
	fixture.Scenes[0].Narration[0].Speech = fixture.Scenes[0].Narration[0].Display
	for _, scenario := range []string{"success", "repair", "invalid", "truncated", "provider-error", "missing-model", "missing-voice", "bad-voice", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body struct {
					Model    string                           `json:"model"`
					Messages []struct{ Role, Content string } `json:"messages"`
					Format   map[string]string                `json:"response_format"`
					Thinking map[string]string                `json:"thinking"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if r.Method != "POST" || r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer test-animation-secret" || body.Model != "deepseek-flash" || body.Format["type"] != "json_object" || body.Thinking["type"] != "disabled" {
					t.Error("generation provider contract mismatch")
				}
				if len(body.Messages) < 2 || !strings.Contains(body.Messages[1].Content, "b保持为2") {
					t.Error("user brief missing")
				}
				if len(body.Messages) == 0 || !strings.Contains(body.Messages[0].Content, animSkillVersion) || !strings.Contains(body.Messages[0].Content, "manim-video") || !strings.Contains(body.Messages[0].Content, "frontend-slides") || !strings.Contains(body.Messages[0].Content, "技术契约") {
					t.Error("actual animation request must include the adapted creative skill and separate technical contract")
				}
				if len(body.Messages) > 0 && (strings.Contains(body.Messages[0].Content, "月亮polygon.points=") || strings.Contains(body.Messages[0].Content, "英语位置课的on")) {
					t.Error("unrelated topic examples must not bias a function lesson")
				}
				if scenario == "provider-error" {
					w.WriteHeader(401)
					_, _ = w.Write([]byte("test-animation-secret"))
					return
				}
				content, finish := string(asJSON(fixture)), "stop"
				if scenario == "invalid" || scenario == "repair" && calls == 1 {
					content = `{"version":"anim-v2","html":"<script>bad()</script>"}`
				}
				if scenario == "truncated" {
					finish = "length"
				}
				if scenario == "repair" && calls == 2 && (len(body.Messages) < 4 || !strings.Contains(body.Messages[len(body.Messages)-1].Content, "不允许")) {
					t.Error("repair lacks validation feedback")
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": finish, "message": map[string]string{"content": content}}}, "usage": map[string]int{"prompt_tokens": 20, "completion_tokens": 30}})
			}))
			defer provider.Close()
			t.Setenv("ANIMATION_LLM_BASE_URL", provider.URL)
			t.Setenv("ANIMATION_LLM_API_KEY", "test-animation-secret")
			t.Setenv("ANIMATION_LLM_MODEL", "deepseek-flash")
			t.Setenv("ANIMATION_TTS_API_KEY", "")
			t.Setenv("ANIMATION_TTS_SPEAKER", "")
			if scenario == "missing-model" {
				t.Setenv("ANIMATION_LLM_API_KEY", "")
			}
			dir := t.TempDir()
			brief, out := filepath.Join(dir, "brief.txt"), filepath.Join(dir, "out.html")
			if err := os.WriteFile(brief, []byte("一次函数改变k，b保持为2"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(out, []byte("old-output"), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"--anim-generate", brief, out, "estimated"}
			if scenario == "missing-voice" {
				args = args[:3]
			}
			if scenario == "bad-voice" {
				args[3] = "invalid"
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "cancelled" {
				cancel()
			}
			handled, err := runAnimCLI(ctx, args)
			if !handled {
				t.Fatal("--anim-generate must be handled")
			}
			got, readErr := os.ReadFile(out)
			if readErr != nil {
				t.Fatal(readErr)
			}
			wantCalls := 1
			switch scenario {
			case "repair", "invalid", "truncated":
				wantCalls = 2
			case "missing-model", "missing-voice", "bad-voice", "cancelled":
				wantCalls = 0
			}
			if calls != wantCalls {
				t.Errorf("calls=%d want=%d", calls, wantCalls)
			}
			if scenario == "success" || scenario == "repair" {
				if err != nil {
					t.Fatal(err)
				}
				for _, want := range []string{"<!doctype html>", "AI生成内容", "无真实语音", "id=\"anim-data\""} {
					if !strings.Contains(string(got), want) {
						t.Errorf("missing %s", want)
					}
				}
				if strings.Contains(string(got), "固定样例脚本") || strings.Contains(string(got), "test-animation-secret") {
					t.Error("wrong provenance or leaked credential")
				}
				raw, e := os.ReadFile(out + ".script.json")
				if e != nil {
					t.Fatal(e)
				}
				if _, problems := decodeAnimScript(raw); len(problems) > 0 {
					t.Fatal(problems)
				}
				raw, e = os.ReadFile(out + ".generation.json")
				if e != nil {
					t.Fatal(e)
				}
				var record struct {
					Model        string `json:"model"`
					SkillVersion string `json:"skill_version"`
					Calls        int    `json:"calls"`
					InputTokens  int    `json:"input_tokens"`
					OutputTokens int    `json:"output_tokens"`
				}
				if json.Unmarshal(raw, &record) != nil || record.Model != "deepseek-flash" || record.SkillVersion != animSkillVersion || record.Calls != calls || record.InputTokens != 20*calls || record.OutputTokens != 30*calls || strings.Contains(string(raw), "test-animation-secret") {
					t.Error("generation evidence missing or unsafe")
				}
			} else {
				if err == nil {
					t.Fatal("expected explicit failure")
				}
				if string(got) != "old-output" {
					t.Error("failed generation overwrote prior output")
				}
				if strings.Contains(err.Error(), "test-animation-secret") {
					t.Error("provider secret reached error")
				}
			}
		})
	}
}

func TestAnimAuthorOpeningAndRevisionContract(t *testing.T) {
	for _, phrase := range []string{"开场介绍", "同学们，今天我们一起看看", "第一幕", "不额外增加一幕", "不要每幕重复问好", "教师明确要求"} {
		if !strings.Contains(animAuthorPrompt, phrase) {
			t.Errorf("opening instruction missing: %s", phrase)
		}
	}
}

func TestAnimScriptRejectsTrailingData(t *testing.T) {
	raw := string(asJSON(loadLinearKFixture(t)))
	for _, suffix := range []string{` {"html":"bad"}`, "garbage"} {
		if _, problems := decodeAnimScript([]byte(raw + suffix)); len(problems) == 0 {
			t.Error("trailing model data was accepted")
		}
	}
}
func TestAnimModelHTTP1Compatibility(t *testing.T) {
	fixture := directModelFixture(t)
	render := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"version": "authored-animation-v1"})
	}))
	defer render.Close()
	t.Setenv("RENDER_SERVICE_URL", render.URL)
	fixture.Scenes[0].Narration[0].Display = "同学们，今天我们一起观察一次函数的图像。"
	fixture.Scenes[0].Narration[0].Speech = fixture.Scenes[0].Narration[0].Display
	calls, protocol := 0, 0
	provider := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		protocol = r.ProtoMajor
		if protocol != 1 {
			w.WriteHeader(502)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(asJSON(fixture))}}}})
	}))
	provider.EnableHTTP2 = true
	provider.StartTLS()
	defer provider.Close()
	original := http.DefaultTransport
	transport := original.(*http.Transport).Clone()
	transport.TLSClientConfig = provider.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	transport.ForceAttemptHTTP2 = true
	http.DefaultTransport = transport
	defer func() { http.DefaultTransport = original; transport.CloseIdleConnections() }()
	t.Setenv("ANIMATION_LLM_BASE_URL", provider.URL)
	t.Setenv("ANIMATION_LLM_API_KEY", "test")
	t.Setenv("ANIMATION_LLM_MODEL", "deepseek-flash")
	dir := t.TempDir()
	brief := filepath.Join(dir, "brief.txt")
	if err := os.WriteFile(brief, []byte("一次函数改变k，b保持为2"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := animGenerateCommand(context.Background(), brief, filepath.Join(dir, "animation.html"), animVoiceEstimated); err != nil {
		t.Fatalf("animation provider protocol=%d: %v", protocol, err)
	}
	if calls != 1 || protocol != 1 {
		t.Fatalf("unexpected request replay or protocol: calls=%d protocol=%d", calls, protocol)
	}
	if http.DefaultTransport != transport || !transport.ForceAttemptHTTP2 || transport.Protocols != nil {
		t.Fatal("animation changed the shared transport")
	}
}
