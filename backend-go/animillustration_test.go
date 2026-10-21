package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTMLAuthoredPictureWorkflowHTTP(t *testing.T) {
	s := worksServer(t)
	s.cfg.Mode = "live"
	script := directModelFixture(t)
	script.Scenes[0].Narration[0].Display = "同学们，今天我们一起观察这幅图。"
	script.Scenes[0].Narration[0].Speech = script.Scenes[0].Narration[0].Display
	script.Scenes[0].HTML = strings.Replace(script.Scenes[0].HTML, "</svg>", `</svg><img data-teaching-picture alt="教师示意图" width="160" height="100">`, 1)

	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 80, 40))); err != nil {
		t.Fatal(err)
	}
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageBytes.Bytes())
	picture := teachingTestPicture(t)
	picture.DataURL = dataURL
	picture.AssetID = worksObject(t, mediaUpload(t, s, worksLogin(t, s, "teacher"), "teaching.png", imageBytes.Bytes(), 201))["asset_id"].(string)
	picture.DataURL = ""

	llmCalls, ttsCalls, renderCalls := 0, 0, 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tts" {
			ttsCalls++
			json.NewEncoder(w).Encode(map[string]any{"audio": base64.StdEncoding.EncodeToString(testMP3(250)), "subtitle": map[string]any{"sentences": []any{map[string]any{"text": "", "start_time": 240, "end_time": 5600}}}})
			return
		}
		llmCalls++
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error("invalid model request")
		}
		if strings.Contains(string(asJSON(request)), "data:image") {
			t.Error("image binary leaked to text provider")
		}
		if !strings.Contains(string(asJSON(request)), "data-teaching-picture") {
			t.Error("model request lost the direct image-slot contract")
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(asJSON(script))}}}})
	}))
	defer provider.Close()
	renderer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/render/authored-animation-check" {
			renderCalls++
			var payload map[string]any
			json.NewDecoder(r.Body).Decode(&payload)
			if payload["picture"] != dataURL {
				t.Error("uploaded picture was not hydrated into the isolated browser check")
			}
			json.NewEncoder(w).Encode(map[string]string{"version": "authored-animation-v1"})
			return
		}
		http.NotFound(w, r)
	}))
	defer renderer.Close()
	s.cfg.RenderURL = renderer.URL
	s.cfg.AnimationLLMURL, s.cfg.AnimationLLMKey, s.cfg.AnimationLLMModel = provider.URL, "fixture-key", "deepseek-flash"
	s.cfg.TTS = ttsConfig{Provider: "volcengine", Protocol: "create", URL: provider.URL + "/tts", Key: "fixture-key", Model: "seed-audio-1.0", Unit: "sentence", Format: "mp3", SampleRate: 48000}

	token := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "other")
	admin := worksLogin(t, s, "admin")
	before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	in := TaskInput{Type: "animation", Subject: "地理", Topic: "教师配图边界测试", Requirements: "根据教师示意图说明观察到的路线", AnimationPicture: picture, AnimationMinSeconds: 30, AnimationMaxSeconds: 180, IdempotencyKey: newID()}
	worksCall(t, s, "POST", "/api/tasks", "", in, 401)
	bad := *picture
	bad.DataURL = "https://example.invalid/picture.png"
	in.AnimationPicture = &bad
	worksCall(t, s, "POST", "/api/tasks", token, in, 400)
	in.AnimationPicture = picture
	for _, foreign := range []string{other, admin} {
		worksCall(t, s, "POST", "/api/tasks", foreign, in, 404)
	}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	path := "/api/tasks/" + id + "/animation"
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	state := worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
	if state["status"] != "anim_plan" || llmCalls != 1 || ttsCalls != 0 {
		t.Fatalf("plan stage: status=%v model=%d tts=%d", state["status"], llmCalls, ttsCalls)
	}
	for _, foreign := range []string{other, admin} {
		worksCall(t, s, "POST", path+"/set-picture", foreign, map[string]any{"revision": state["revision"], "picture": picture}, 404)
	}
	worksCall(t, s, "POST", path+"/set-picture", token, map[string]any{"revision": state["revision"], "picture": &bad}, 400)
	oldRevision := state["revision"]
	updated := *picture
	updated.Caption = "更新后的教师配图说明"
	worksCall(t, s, "POST", path+"/set-picture", token, map[string]any{"revision": oldRevision, "picture": &updated}, 200)
	worksCall(t, s, "POST", path+"/confirm-plan", token, map[string]any{"revision": oldRevision}, 409)
	state = worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
	worksCall(t, s, "POST", path+"/confirm-plan", token, map[string]any{"revision": state["revision"]}, 202)
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	state = worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
	if state["status"] != "anim_representative" {
		t.Fatal("representative stage was not built", state["status"], state["problems"])
	}
	worksCall(t, s, "POST", path+"/confirm-representative", token, map[string]any{"revision": state["revision"], "hash": state["representative_hash"]}, 202)
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	state = worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
	if state["status"] != "anim_acceptance" {
		t.Fatal("full authored build was not created", state["status"], state["problems"])
	}
	preview := worksCall(t, s, "GET", path+"/file", token, nil, 200).Body.String()
	if !strings.Contains(preview, dataURL) || renderCalls < 2 || ttsCalls != 10 {
		t.Fatalf("picture missing from offline delivery or incomplete build: render=%d tts=%d", renderCalls, ttsCalls)
	}
	worksCall(t, s, "POST", path+"/accept", token, map[string]any{"revision": state["revision"], "hash": state["build_hash"]}, 200)
	delivery := worksCall(t, s, "GET", "/api/tasks/"+id+"/download/html", token, nil, 200).Body.String()
	if !strings.Contains(delivery, dataURL) {
		t.Fatal("accepted HTML delivery omitted the hydrated teacher picture")
	}
	after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	if before["balance"].(float64)-after["balance"].(float64) != 40 || after["reserved"] != before["reserved"] {
		t.Fatal("picture workflow did not settle once", after)
	}
	for _, foreign := range []string{other, admin} {
		worksCall(t, s, "GET", "/api/tasks/"+id+"/download/html", foreign, nil, 404)
	}
}
