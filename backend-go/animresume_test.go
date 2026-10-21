package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Public local build boundary: a speech failure must not force another model call.
func TestAnimationResumeSavedGeneration(t *testing.T) {
	calls := 0
	broken := true
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if broken && calls == 2 {
			w.WriteHeader(503)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"audio": base64.StdEncoding.EncodeToString(testMP3(150))})
	}))
	defer provider.Close()
	t.Setenv("ANIMATION_TTS_PROVIDER", "volcengine")
	t.Setenv("ANIMATION_TTS_PROTOCOL", "create")
	t.Setenv("ANIMATION_TTS_MODEL", "seed-audio-1.0")
	t.Setenv("ANIMATION_TTS_URL", provider.URL)
	t.Setenv("ANIMATION_TTS_API_KEY", "resume-test-secret")
	t.Setenv("ANIMATION_TTS_FORMAT", "mp3")
	t.Setenv("ANIMATION_TTS_UNIT", "sentence")
	t.Setenv("ANIMATION_TTS_RETRIES", "0")
	sc := loadLinearKFixture(t)
	sc.Scenes[0].Narration[0].Speech = "同学们，今天一起观察函数。"
	sc.Scenes[0].Narration[0].Display = sc.Scenes[0].Narration[0].Speech
	out := filepath.Join(t.TempDir(), "animation.html")
	if err := os.WriteFile(out+".script.json", asJSON(sc), 0600); err != nil {
		t.Fatal(err)
	}
	record := map[string]any{"model": "saved-model", "skill_version": animSkillVersion, "calls": 1, "voice": "tts", "script_hash": animDigest(sc), "input": TaskInput{Topic: sc.Title, AnimationMinSeconds: 30, AnimationMaxSeconds: 180}}
	if err := os.WriteFile(out+".generation.json", asJSON(record), 0600); err != nil {
		t.Fatal(err)
	}
	handled, err := runAnimCLI(context.Background(), []string{"--anim-resume", out})
	if !handled || err == nil || calls != 2 {
		t.Fatalf("initial failure: handled=%v err=%v calls=%d", handled, err, calls)
	}
	broken = false
	handled, err = runAnimCLI(context.Background(), []string{"--anim-resume", out})
	if !handled || err != nil {
		t.Fatalf("resume: %v %v", handled, err)
	}
	if calls != 11 {
		t.Fatalf("successful sentence re-synthesized: calls=%d want 11", calls)
	}
	raw, err := os.ReadFile(out)
	if err != nil || strings.Contains(string(raw), "固定样例") || !strings.Contains(string(raw), "AI生成内容") || strings.Contains(string(raw), "resume-test-secret") {
		t.Fatal("wrong output provenance or leaked secret")
	}
	sc.Title = "changed"
	if err := os.WriteFile(out+".script.json", asJSON(sc), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = runAnimCLI(context.Background(), []string{"--anim-resume", out}); err == nil || calls != 11 {
		t.Fatal("changed saved generation was silently accepted")
	}
}
