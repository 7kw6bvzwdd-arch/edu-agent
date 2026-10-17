package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHTMLAnimationDefaultRouting(t *testing.T) {
	s := &Server{cfg: Config{Mode: "live"}}
	r := httptest.NewRequest("POST", "/api/tasks", bytes.NewReader(asJSON(TaskInput{Type: "animation", Topic: "一次函数", IdempotencyKey: "default-html"})))
	w := httptest.NewRecorder()
	api(func(w http.ResponseWriter, r *http.Request) error { return s.createTask(w, r, Actor{Role: "admin"}) }).ServeHTTP(w, r)
	if w.Code != 503 || !bytes.Contains(w.Body.Bytes(), []byte("HTML动画需要")) {
		t.Fatalf("default animation must use HTML readiness: %d %s", w.Code, w.Body.String())
	}
}

func TestLegacyAnimationReadOnlyHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	admin, other := worksLogin(t, s, "admin"), worksLogin(t, s, "other")
	var a Actor
	if err := json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	for _, engine := range []string{"manim", "ark"} {
		id := worksFixture(t, s, token, "animation", "succeeded", "历史动画", TaskInput{AnimationEngine: engine})
		path := "/api/tasks/" + id
		// Existing artifact bytes test access and retention, not media generation.
		mp4 := filepath.Join(s.cfg.Storage, id+".mp4")
		frames := filepath.Join(s.cfg.Storage, "animation-quality", id)
		if err := os.MkdirAll(frames, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(mp4, []byte("historical-video"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(frames, "frame-0.jpg"), []byte("historical-frame"), 0600); err != nil {
			t.Fatal(err)
		}
		tx, err := s.tx(context.Background(), a.TenantID)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(context.Background(), `UPDATE tasks SET mp4_path=$1,result=$2 WHERE id=$3`, mp4, asJSON(map[string]any{"title": "历史动画", "slides": []any{map[string]any{"title": "旧分镜", "scene": map[string]any{"goal": "保留历史内容"}}}, "animation_quality": map[string]any{"status": "accepted", "frames": []any{map[string]any{"index": 0, "second": 1}}}}), id)
		if err != nil {
			tx.Rollback(context.Background())
			t.Fatal(err)
		}
		if err = tx.Commit(context.Background()); err != nil {
			t.Fatal(err)
		}
		before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
		for _, suffix := range []string{"", "/download/mp4", "/frames/0"} {
			worksCall(t, s, "GET", path+suffix, "", nil, 401)
			for _, foreign := range []string{admin, other} {
				worksCall(t, s, "GET", path+suffix, foreign, nil, 404)
			}
			worksCall(t, s, "GET", path+suffix, token, nil, 200)
		}
		if got := worksCall(t, s, "GET", path+"/download/mp4", token, nil, 200).Body.String(); got != "historical-video" {
			t.Fatal("historical artifact changed")
		}
		for _, foreign := range []string{admin, other} {
			worksCall(t, s, "POST", path+"/accept-animation", foreign, map[string]any{"revision": 0, "accept": true}, 404)
		}
		worksCall(t, s, "POST", path+"/accept-animation", token, map[string]any{"revision": 0, "accept": true}, 410)
		worksCall(t, s, "POST", path+"/confirm", token, map[string]any{}, 503)
		if after := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String(); after != before {
			t.Fatal("history access changed billing")
		}
		worksCall(t, s, "DELETE", path, token, nil, 200)
		for _, suffix := range []string{"", "/download/mp4", "/frames/0"} {
			worksCall(t, s, "GET", path+suffix, token, nil, 404)
		}
	}
}

func TestHTMLAnimationSourceSnapshotHTTP(t *testing.T) {
	s := worksServer(t)
	token, other := worksLogin(t, s, "teacher"), worksLogin(t, s, "other")
	own := worksFixture(t, s, token, "courseware", "succeeded", "一次函数", TaskInput{})
	foreign := worksFixture(t, s, other, "courseware", "succeeded", "一次函数", TaskInput{})
	pending := worksFixture(t, s, token, "courseware", "awaiting_confirmation", "一次函数", TaskInput{})
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	sources := []Source{{ID: "S1", Title: "本人课件资料", Content: "一次函数中保持b不变，比较k。"}}
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), "UPDATE tasks SET sources=$1 WHERE id=$2", asJSON(sources), own)
	if err != nil {
		tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.cfg.Mode = "live"
	s.cfg.AnimationLLMURL, s.cfg.AnimationLLMKey, s.cfg.AnimationLLMModel = "http://127.0.0.1:1", "test-key", "test-model"
	s.cfg.TTS = ttsConfig{Provider: "volcengine", Protocol: "create", URL: "http://127.0.0.1:1/tts", Key: "test-key", Model: "seed-audio-1.0", Unit: "sentence", Format: "mp3", SampleRate: 48000}
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	for _, source := range []string{foreign, pending, "missing-source"} {
		worksCall(t, s, "POST", "/api/tasks", token, TaskInput{Type: "animation", Topic: "一次函数", SourceTaskID: source, IdempotencyKey: newID()}, 404)
	}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, TaskInput{Type: "animation", Topic: "一次函数", SourceTaskID: own, IdempotencyKey: newID()}, 202))["id"].(string)
	got := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
	var saved []Source
	if err := json.Unmarshal(asJSON(got["sources"]), &saved); err != nil {
		t.Fatal(err)
	}
	if string(asJSON(saved)) != string(asJSON(sources)) {
		t.Fatal("source snapshot missing")
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before {
		t.Fatal("planning changed billing")
	}
	worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
	worksCall(t, s, "DELETE", "/api/tasks/"+id, token, nil, 200)
}

func TestLegacyAnimationUnacceptedNeverSettlesHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, TaskInput{Type: "courseware", Topic: "旧动画预留迁移测试", UseKB: true, Pages: 8, IdempotencyKey: newID()}, 202))["id"].(string)
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	ctx := context.Background()
	tx, err := s.tx(ctx, a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `UPDATE tasks SET type='animation',status='awaiting_acceptance',stage='awaiting_acceptance',input=$1,result=$2 WHERE id=$3`, asJSON(TaskInput{Type: "animation", AnimationEngine: "manim", Topic: "历史动画"}), asJSON(map[string]any{"animation_quality": map[string]any{"status": "awaiting_acceptance"}}), id)
	if err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	reserved := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	worksCall(t, s, "POST", "/api/tasks/"+id+"/accept-animation", token, map[string]any{"revision": 0, "accept": true}, 410)
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != reserved {
		t.Fatal("retired acceptance changed billing")
	}
	worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
	worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
	after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	if after["balance"] != before["balance"] || after["reserved"] != before["reserved"] {
		t.Fatal("legacy cancellation did not release once")
	}
	worksCall(t, s, "DELETE", "/api/tasks/"+id, token, nil, 200)
}
