package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTMLIllustrationPictureHTTP(t *testing.T) {
	s := worksServer(t)
	s.cfg.Mode = "live"
	raw, e := os.ReadFile(filepath.Join("..", "fixtures", "animation-v2-illustrated.json"))
	if e != nil {
		t.Fatal(e)
	}
	var script map[string]any
	json.Unmarshal(raw, &script)
	script["representative"] = script["scenes"].([]any)[0].(map[string]any)["id"]
	scene := script["scenes"].([]any)[0].(map[string]any)
	scene["objects"].([]any)[0].(map[string]any)["kind"] = "uploaded"
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tts" {
			json.NewEncoder(w).Encode(map[string]any{"audio": base64.StdEncoding.EncodeToString(testMP3(250)), "subtitle": map[string]any{"sentences": []any{map[string]any{"start_time": 240, "end_time": 5600}}}})
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if strings.Contains(string(asJSON(body)), "data:image") {
			t.Error("Image binary leaked to text provider")
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(asJSON(script))}}}})
	}))
	defer provider.Close()
	s.cfg.AnimationLLMURL, s.cfg.AnimationLLMKey, s.cfg.AnimationLLMModel = provider.URL, "fixture-key", "deepseek-flash"
	s.cfg.TTS = ttsConfig{Provider: "volcengine", Protocol: "create", URL: provider.URL + "/tts", Key: "fixture-key", Model: "seed-audio-1.0", Unit: "sentence", Format: "mp3", SampleRate: 48000}
	token := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "other")
	admin := worksLogin(t, s, "admin")
	before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	picture := teachingTestPicture(t)
	img := image.NewRGBA(image.Rect(0, 0, 80, 40))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{228, 65, 63, 255}), image.Point{}, draw.Src)
	var pngData bytes.Buffer
	if e := png.Encode(&pngData, img); e != nil {
		t.Fatal(e)
	}
	picture.DataURL = "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngData.Bytes())
	in := map[string]any{"type": "animation", "animation_engine": "html", "subject": "地理", "topic": "配图边界测试样例", "requirements": "根据教师图示讲解水循环", "animation_picture": picture, "idempotency_key": newID()}
	worksCall(t, s, "POST", "/api/tasks", "", in, 401)
	bad := *picture
	bad.DataURL = "https://example.invalid/picture.png"
	in["animation_picture"] = &bad
	worksCall(t, s, "POST", "/api/tasks", token, in, 400)
	in["animation_picture"] = picture
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	path := "/api/tasks/" + id + "/animation"
	if e = s.workOne(context.Background()); e != nil {
		t.Fatal(e)
	}
	state := worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
	if state["status"] != "anim_plan" {
		t.Fatal(state)
	}
	for _, foreign := range []string{other, admin} {
		worksCall(t, s, "POST", path+"/set-picture", foreign, map[string]any{"revision": state["revision"], "picture": picture}, 404)
	}
	worksCall(t, s, "POST", path+"/set-picture", token, map[string]any{"revision": state["revision"], "picture": &bad}, 400)
	worksCall(t, s, "POST", path+"/set-picture", token, map[string]any{"revision": state["revision"]}, 409)
	picture.Caption = "更新后的教师图示"
	worksCall(t, s, "POST", path+"/set-picture", token, map[string]any{"revision": state["revision"], "picture": picture}, 200)
	worksCall(t, s, "POST", path+"/confirm-plan", token, map[string]any{"revision": state["revision"]}, 409)
	state = worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
	worksCall(t, s, "POST", path+"/confirm-plan", token, map[string]any{"revision": state["revision"]}, 202)
	if e = s.workOne(context.Background()); e != nil {
		t.Fatal(e)
	}
	state = worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
	if state["status"] != "anim_representative" {
		t.Fatal(state)
	}
	worksCall(t, s, "POST", path+"/set-picture", token, map[string]any{"revision": state["revision"], "picture": picture}, 409)
	worksCall(t, s, "POST", path+"/confirm-representative", token, map[string]any{"revision": state["revision"], "hash": state["representative_hash"]}, 202)
	if e = s.workOne(context.Background()); e != nil {
		t.Fatal(e)
	}
	state = worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
	if state["status"] != "anim_acceptance" {
		t.Fatal(state)
	}
	html := worksCall(t, s, "GET", path+"/file", token, nil, 200).Body.String()
	if dir := os.Getenv("ANIMATION_TEST_OUTPUT"); dir != "" {
		if e := os.MkdirAll(dir, 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(filepath.Join(dir, "picture-http.html"), []byte(html), 0600); e != nil {
			t.Fatal(e)
		}
	}
	if !strings.Contains(html, picture.DataURL) || !strings.Contains(html, "classroom-illustrated-v1") {
		t.Fatal("Missing offline picture or classroom renderer")
	}
	for _, foreign := range []string{other, admin} {
		worksCall(t, s, "GET", path+"/file", foreign, nil, 404)
	}
	worksCall(t, s, "POST", path+"/accept", token, map[string]any{"revision": state["revision"], "hash": state["build_hash"]}, 200)
	worksCall(t, s, "GET", "/api/tasks/"+id+"/download/html", token, nil, 200)
	after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	if after["balance"].(float64) != before["balance"].(float64)-40 {
		t.Fatal("Picture workflow did not settle exactly once")
	}
}
