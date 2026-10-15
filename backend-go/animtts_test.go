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
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
)

// testMP3 builds n silent MPEG-2 Layer III frames (24 kHz, 64 kbps, 576 samples = 24 ms each).
func testMP3(n int) []byte {
	frame := make([]byte, 576/8*64000/24000) // 192 bytes
	copy(frame, []byte{0xff, 0xf3, 0x84, 0xc4})
	b := []byte("ID3\x04\x00\x00\x00\x00\x00\x02xx") // 2-byte tag body
	for i := 0; i < n; i++ {
		b = append(b, frame...)
	}
	return b
}

func TestMP3DurationFromFrames(t *testing.T) {
	ms, e := mp3DurationMS(testMP3(125))
	if e != nil || ms != 3000 {
		t.Fatalf("got %d %v", ms, e)
	}
	if _, e := mp3DurationMS([]byte("not audio")); e == nil {
		t.Fatal("garbage must fail")
	}
}

func fakeTTS(t *testing.T, handler func(w http.ResponseWriter, body map[string]any, call int32)) (*httptest.Server, *int32) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "secret-key" || r.Header.Get("X-Api-Resource-Id") != "seed-tts-2.0" || r.Header.Get("X-Api-Request-Id") == "" {
			t.Errorf("headers: %v", r.Header)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		handler(w, body, atomic.AddInt32(&calls, 1))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestSynthesizeSpeechStreamsChunks(t *testing.T) {
	audio := testMP3(50)
	srv, _ := fakeTTS(t, func(w http.ResponseWriter, body map[string]any, _ int32) {
		req := body["req_params"].(map[string]any)
		if req["text"] != "k 等于 3" || req["speaker"] != "zh_female_test" || req["audio_params"].(map[string]any)["format"] != "mp3" {
			t.Errorf("body %v", body)
		}
		w.Header().Set("X-Tt-Logid", "log-1")
		for _, part := range [][]byte{audio[:500], audio[500:]} {
			fmt.Fprintf(w, `{"code":0,"message":"","data":%q}`+"\n", base64.StdEncoding.EncodeToString(part))
			w.(http.Flusher).Flush()
		}
		fmt.Fprint(w, `{"code":0,"data":"","sentence":{"text":"k 等于 3","words":[{"word":"k","startTime":0.1,"endTime":0.3}]}}`+"\n")
		fmt.Fprint(w, `{"code":20000000,"message":"OK","data":""}`+"\n")
	})
	c := ttsConfig{Provider: "volcengine", URL: srv.URL, Key: "secret-key", ResourceID: "seed-tts-2.0", Speaker: "zh_female_test", Format: "mp3", Unit: "sentence", SampleRate: 24000}
	r, e := synthesizeSpeech(context.Background(), srv.Client(), c, "k 等于 3", true)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(r.Audio, audio) || r.Chunks != 2 || r.DurationMS != 1200 || len(r.Words) != 1 || r.LogID != "log-1" {
		t.Fatalf("result chunks=%d ms=%d words=%v log=%s", r.Chunks, r.DurationMS, r.Words, r.LogID)
	}
}

func TestSynthesizeRetriesAreBoundedAndKeyNeverLeaks(t *testing.T) {
	srv, calls := fakeTTS(t, func(w http.ResponseWriter, _ map[string]any, call int32) {
		if call == 1 {
			w.WriteHeader(503)
			fmt.Fprint(w, `{"code":55000000,"message":"server busy"}`)
			return
		}
		fmt.Fprintf(w, `{"code":0,"data":%q}`+"\n"+`{"code":20000000,"data":""}`, base64.StdEncoding.EncodeToString(testMP3(10)))
	})
	c := ttsConfig{Provider: "volcengine", URL: srv.URL, Key: "secret-key", ResourceID: "seed-tts-2.0", Speaker: "s", Format: "mp3", Unit: "sentence", Retries: 2}
	r, attempts, e := synthesizeWithRetry(context.Background(), srv.Client(), c, "你好")
	if e != nil || attempts != 2 || *calls != 2 || r.DurationMS != 240 {
		t.Fatalf("transient retry: attempts=%d calls=%d err=%v", attempts, *calls, e)
	}

	srv2, calls2 := fakeTTS(t, func(w http.ResponseWriter, _ map[string]any, _ int32) {
		w.WriteHeader(500)
		fmt.Fprint(w, `{"code":1,"message":"down"}`)
	})
	c.URL = srv2.URL
	_, attempts, e = synthesizeWithRetry(context.Background(), srv2.Client(), c, "你好")
	if e == nil || attempts != 3 || *calls2 != 3 || strings.Contains(e.Error(), "secret-key") {
		t.Fatalf("exhausted retries: attempts=%d calls=%d err=%v", attempts, *calls2, e)
	}

	// Business errors inside the stream are not retried.
	srv3, calls3 := fakeTTS(t, func(w http.ResponseWriter, _ map[string]any, _ int32) {
		fmt.Fprint(w, `{"code":45000000,"message":"speaker not granted"}`)
	})
	c.URL = srv3.URL
	if _, _, e = synthesizeWithRetry(context.Background(), srv3.Client(), c, "你好"); e == nil || *calls3 != 1 || !strings.Contains(e.Error(), "speaker not granted") {
		t.Fatalf("non-retryable: calls=%d err=%v", *calls3, e)
	}
}

func TestTTSConfigRequiresSentenceUnit(t *testing.T) {
	c := ttsConfig{Provider: "volcengine", URL: "u", Key: "k", ResourceID: "r", Speaker: "s", Format: "mp3", Unit: "sentence"}
	if !c.configured() {
		t.Fatal("complete config")
	}
	c.Unit = "segment"
	if c.configured() || !strings.Contains(strings.Join(c.missing(), ","), "ANIMATION_TTS_UNIT") {
		t.Fatal("segment synthesis is not implemented and must not silently run")
	}
}

// Render boundary: validated script -> real single-file offline HTML.
func TestAnimPackageIsSelfContained(t *testing.T) {
	sc := loadLinearKFixture(t)
	audio, ms := map[string][]byte{}, map[string]int{}
	for _, s := range sc.Scenes {
		for _, n := range s.Narration {
			audio[n.ID], ms[n.ID] = testMP3(100), 2400
		}
	}
	tl, p := buildAnimTimeline(sc, ms)
	if len(p) > 0 {
		t.Fatal(p)
	}
	out := string(buildAnimPackage(animPackageInput{Script: sc, Timeline: tl, Audio: audio, Format: "mp3", Voice: animVoiceTTS, Fixture: true}))
	csp := regexp.MustCompile(`http-equiv="Content-Security-Policy" content="([^"]+)"`).FindStringSubmatch(out)
	if csp == nil || !strings.HasPrefix(csp[1], "default-src 'none'") || strings.Contains(csp[1], "unsafe") || !strings.Contains(csp[1], cspHash(animPlayerJSLF)) {
		t.Fatalf("csp %v", csp)
	}
	if strings.Contains(out, "http://") || strings.Contains(out, "https://") || strings.Contains(out, "//cdn") {
		t.Fatal("package must not reference network resources")
	}
	if !strings.Contains(out, "<script>"+animPlayerJSLF+"</script>") || !strings.Contains(out, "<style>"+animPlayerCSSLF+"</style>") {
		t.Fatal("hashed inline code must match embedded assets byte for byte")
	}
	if !strings.Contains(out, "data:audio/mpeg;base64,") || !strings.Contains(out, "固定样例脚本") || !strings.Contains(out, "AI合成语音") || strings.Contains(out, "无真实语音") || strings.Contains(out, ">AI生成内容<") {
		t.Fatal("fixture script with real TTS: sample-script label plus synthesized-voice label, never 'AI生成内容'")
	}
	if model := string(buildAnimPackage(animPackageInput{Script: sc, Timeline: tl, Audio: audio, Format: "mp3", Voice: animVoiceTTS})); !strings.Contains(model, `<meta name="aigc" content="AI生成内容">`) || strings.Contains(model, "固定样例脚本") {
		t.Fatal("model-generated package must carry the AIGC marking")
	}
	m := regexp.MustCompile(`(?s)<script type="application/json" id="anim-data">(.*?)</script>`).FindStringSubmatch(out)
	var data struct {
		Script   AnimScript   `json:"script"`
		Timeline AnimTimeline `json:"timeline"`
	}
	if m == nil || json.Unmarshal([]byte(m[1]), &data) != nil || len(validateAnimScript(data.Script)) > 0 || data.Timeline.TotalMS != tl.TotalMS {
		t.Fatal("embedded scene data must round-trip and stay valid")
	}
	sample := string(buildAnimPackage(animPackageInput{Script: sc, Timeline: tl, Format: "mp3", Voice: animVoiceEstimated, Fixture: true}))
	if strings.Contains(string(buildAnimPackage(animPackageInput{Script: sc, Timeline: tl, Format: "mp3", Voice: animVoiceTTS, Fixture: true})), "\r") {
		t.Fatal("inline assets must be LF so their CSP hashes match what the browser executes")
	}
	if !strings.Contains(sample, "样例模式：无真实语音") || strings.Contains(sample, "data:audio") || strings.Contains(sample, `name="aigc"`) {
		t.Fatal("sample package must be labelled and carry no audio")
	}
	if os.Getenv("ANIM_PACKAGE_OUT") != "" {
		os.WriteFile(os.Getenv("ANIM_PACKAGE_OUT"), []byte(out), 0o644)
	}
}
