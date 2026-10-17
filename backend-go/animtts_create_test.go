package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestSeedAudioCreate(t *testing.T) {
	for _, scenario := range []string{"ok", "speaker", "classroom", "business-error", "http-error", "no-audio", "invalid-base64", "invalid-audio"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			audio := testMP3(50)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "POST" || r.Header.Get("X-Api-Key") != "private-test-key" || r.Header.Get("X-Api-Resource-Id") != "" || r.Header.Get("X-Api-Request-Id") == "" {
					t.Error("wrong create headers")
				}
				var body map[string]any
				if json.NewDecoder(r.Body).Decode(&body) != nil {
					t.Fatal("invalid payload")
				}
				prompt, _ := body["text_prompt"].(string)
				ac, _ := body["audio_config"].(map[string]any)
				if body["model"] != "seed-audio-1.0" || !strings.Contains(prompt, "k 等于 3") || !strings.Contains(prompt, "普通话") || body["req_params"] != nil || ac["sample_rate"] != float64(48000) || ac["format"] != "mp3" || ac["enable_subtitle"] != true {
					t.Error("wrong create body")
				}
				if scenario == "classroom" {
					for _, phrase := range []string{"亲切", "好奇", "提问", "不夸张", "逐字"} {
						if !strings.Contains(prompt, phrase) {
							t.Errorf("classroom delivery missing: %s", phrase)
						}
					}
				}
				refs, _ := body["references"].([]any)
				if scenario == "speaker" {
					if len(refs) != 1 || refs[0].(map[string]any)["speaker"] != "female-test" {
						t.Error("speaker reference missing")
					}
				} else if len(refs) != 0 {
					t.Error("empty speaker must not create a reference")
				}
				reply := map[string]any{"audio": base64.StdEncoding.EncodeToString(audio), "duration": 999, "original_duration": 1.4, "url": "http://127.0.0.1/private", "subtitle": map[string]any{"text": "k 等于 3", "sentences": []any{map[string]any{"text": "k 等于 3", "start_time": 100, "end_time": 1100, "words": []any{map[string]any{"text": "k", "start_time": 100, "end_time": 300}}}}}}
				switch scenario {
				case "business-error":
					reply = map[string]any{"code": 45000000, "message": "private-test-key"}
				case "http-error":
					w.WriteHeader(401)
					reply = map[string]any{"code": 1, "message": "private-test-key"}
				case "no-audio":
					delete(reply, "audio")
				case "invalid-base64":
					reply["audio"] = "not base64"
				case "invalid-audio":
					reply["audio"] = base64.StdEncoding.EncodeToString([]byte("private-test-key"))
				}
				w.Header().Set("X-Tt-Logid", "test-log")
				_ = json.NewEncoder(w).Encode(reply)
			}))
			defer srv.Close()
			c := ttsConfig{Provider: "volcengine", Protocol: "create", URL: srv.URL, Key: "private-test-key", Model: "seed-audio-1.0", Format: "mp3", Unit: "sentence", SampleRate: 48000}
			if scenario == "classroom" {
				c.Delivery = "classroom-v1"
			}
			if scenario == "speaker" {
				c.Speaker = "female-test"
			}
			if !c.configured() {
				t.Fatal("create needs model/key, not resource or speaker")
			}
			r, err := synthesizeSpeech(context.Background(), srv.Client(), c, "k 等于 3", true)
			if scenario == "ok" || scenario == "speaker" || scenario == "classroom" {
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(r.Audio, audio) || r.DurationMS != 1200 || r.Chunks != 1 || len(r.Words) != 1 || r.Words[0].StartTime != 0.1 || r.Words[0].EndTime != 0.3 || r.OriginalDurationMS != 1400 || len(r.Sentences) != 1 || r.Sentences[0].StartTime != 100 {
					t.Error("audio, billing duration, or subtitle units wrong")
				}
			} else if err == nil || strings.Contains(err.Error(), "private-test-key") {
				t.Error("must fail safely without echoing provider message")
			}
			if calls != 1 {
				t.Error("unexpected additional request")
			}
		})
	}
}

func TestAnimationVoiceDeliveryKeepsExistingAudio(t *testing.T) {
	c := ttsConfig{Provider: "volcengine", Protocol: "create", Model: "seed-audio-1.0", Format: "mp3", Unit: "sentence", Key: "private-test-key"}
	old := (&Server{cfg: Config{TTS: c}}).animVoiceFingerprint()
	c.Delivery = "classroom-v1"
	s := &Server{cfg: Config{TTS: c}}
	if s.animVoiceFingerprint() == old {
		t.Fatal("delivery style omitted from cache fingerprint")
	}
	voice, ok := s.animVoiceFor(old)
	if !ok || voice.Delivery != "" {
		t.Fatal("existing delivery cannot retain its original synthesis profile")
	}
	voice, ok = s.animVoiceFor(s.animVoiceFingerprint())
	if !ok || voice.Delivery != "classroom-v1" {
		t.Fatal("new tasks do not use classroom delivery")
	}
	c.Model = "other"
	if _, ok := (&Server{cfg: Config{TTS: c}}).animVoiceFor(old); ok {
		t.Fatal("unrelated configuration change accepted as legacy delivery")
	}
}

func TestTTSProtocolConfiguration(t *testing.T) {
	t.Setenv("ANIMATION_TTS_PROTOCOL", "create")
	t.Setenv("ANIMATION_TTS_PROVIDER", "volcengine")
	t.Setenv("ANIMATION_TTS_URL", "https://openspeech.bytedance.com/api/v3/tts/create")
	t.Setenv("ANIMATION_TTS_API_KEY", "private-test-key")
	t.Setenv("ANIMATION_TTS_MODEL", "seed-audio-1.0")
	t.Setenv("ANIMATION_TTS_RESOURCE_ID", "")
	t.Setenv("ANIMATION_TTS_SPEAKER", "")
	t.Setenv("ANIMATION_TTS_FORMAT", "mp3")
	t.Setenv("ANIMATION_TTS_UNIT", "sentence")
	c := ttsConfigFromEnv()
	if !c.configured() || c.Protocol != "create" || c.Model != "seed-audio-1.0" || c.Delivery != "classroom-v1" {
		t.Fatal("create env not wired")
	}
	c.Model = ""
	if c.configured() || !strings.Contains(strings.Join(c.missing(), ","), "ANIMATION_TTS_MODEL") {
		t.Error("create must require model")
	}
	c.Protocol = "unsupported"
	if c.configured() {
		t.Error("unknown protocol must fail closed")
	}
	c.Protocol = "unidirectional"
	if c.configured() {
		t.Error("streaming must still require speaker and resource")
	}
}

func TestTTSRedirectAndProviderMessagesDoNotLeakKey(t *testing.T) {
	leaked := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked++; w.WriteHeader(200) }))
	defer target.Close()
	for _, protocol := range []string{"unidirectional", "create"} {
		for _, redirect := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s-%v", protocol, redirect), func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if redirect {
						http.Redirect(w, r, target.URL, 307)
						return
					}
					fmt.Fprint(w, `{"code":45000000,"message":"private-test-key"}`)
				}))
				defer srv.Close()
				c := ttsConfig{Provider: "volcengine", Protocol: protocol, URL: srv.URL, Key: "private-test-key", Model: "seed-audio-1.0", Speaker: "test", ResourceID: "test", Unit: "sentence", Format: "mp3", SampleRate: 48000, Retries: 2}
				_, attempts, err := synthesizeWithRetry(context.Background(), srv.Client(), c, "你好")
				if err == nil || strings.Contains(err.Error(), c.Key) || attempts != 1 {
					t.Error("must refuse redirect/business error without retry or secret echo")
				}
			})
		}
	}
	if leaked != 0 {
		t.Error("credential-bearing request followed redirect")
	}
}

func TestSeedAudioCreateBuildsOfflineHTML(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "seed-audio-1.0" {
			t.Error("HTML did not use create adapter")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"audio": base64.StdEncoding.EncodeToString(testMP3(50))})
	}))
	defer srv.Close()
	t.Setenv("ANIMATION_TTS_PROVIDER", "volcengine")
	t.Setenv("ANIMATION_TTS_PROTOCOL", "create")
	t.Setenv("ANIMATION_TTS_MODEL", "seed-audio-1.0")
	t.Setenv("ANIMATION_TTS_URL", srv.URL)
	t.Setenv("ANIMATION_TTS_API_KEY", "private-test-key")
	t.Setenv("ANIMATION_TTS_SPEAKER", "")
	t.Setenv("ANIMATION_TTS_RESOURCE_ID", "")
	t.Setenv("ANIMATION_TTS_FORMAT", "mp3")
	t.Setenv("ANIMATION_TTS_UNIT", "sentence")
	t.Setenv("ANIMATION_TTS_RETRIES", "0")
	sc := loadLinearKFixture(t)
	out := filepath.Join(t.TempDir(), "create.html")
	if err := writeAnimPackage(context.Background(), sc, out, animVoiceTTS, true); err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 10 || !bytes.Contains(html, []byte("data:audio/mpeg;base64,")) || !bytes.Contains(html, []byte("AI合成语音")) || bytes.Contains(html, []byte("private-test-key")) {
		t.Error("create output did not reach a safe offline HTML package")
	}
	m := regexp.MustCompile(`<script type="application/json" id="anim-data">(.*?)</script>`).FindSubmatch(html)
	var data struct {
		Timeline AnimTimeline      `json:"timeline"`
		Audio    map[string]string `json:"audio"`
	}
	if len(m) != 2 || json.Unmarshal(m[1], &data) != nil || len(data.Audio) != calls {
		t.Fatal("embedded package data missing")
	}
	for _, scene := range data.Timeline.Scenes {
		for _, sentence := range scene.Sentences {
			if sentence.End-sentence.Start != 1200 {
				t.Fatal("timeline used estimated instead of decoded audio duration")
			}
		}
	}
}
