package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRemediationAnimationQueueAgingHTTP(t *testing.T) {
	s := worksServer(t)
	script := directModelFixture(t)
	script.Scenes[0].Narration[0].Display = "同学们，今天我们一起观察一次函数的图像。"
	script.Scenes[0].Narration[0].Speech = script.Scenes[0].Narration[0].Display
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(asJSON(script))}}}})
	}))
	defer provider.Close()
	s.cfg.Mode = "live"
	s.cfg.AnimationLLMURL, s.cfg.AnimationLLMKey, s.cfg.AnimationLLMModel = provider.URL, "fixture", "fixture"
	s.cfg.TTS = ttsConfig{Provider: "volcengine", Protocol: "create", URL: provider.URL + "/tts", Key: "fixture", Model: "seed-audio-1.0", Unit: "sentence", Format: "mp3", SampleRate: 48000}
	token := worksLogin(t, s, "teacher")
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, TaskInput{Type: "animation", Topic: "一次函数改变k", IdempotencyKey: newID()}, 202))["id"].(string)
	tx, err := s.tx(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), "UPDATE tasks SET created_at=now()-interval '3 minutes',updated_at=now()-interval '3 minutes' WHERE id=$1", id)
	if err == nil {
		err = tx.Commit(context.Background())
	} else {
		tx.Rollback(context.Background())
	}
	if err != nil {
		t.Fatal(err)
	}
	s.cfg.Mode = "sample"
	course := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, TaskInput{Type: "courseware", Topic: "排队中的课件", UseKB: true, IdempotencyKey: newID()}, 202))["id"].(string)
	s.cfg.Mode = "live"
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
	if got["status"] != "anim_plan" {
		t.Fatalf("aged animation was starved: %v", got["status"])
	}
	worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
	worksCall(t, s, "POST", "/api/tasks/"+course+"/cancel", token, nil, 200)
}

func TestRemediationWorkersShutdownAndRenewalHTTP(t *testing.T) {
	s := worksServer(t)
	tokens := []string{worksLogin(t, s, "teacher"), worksLogin(t, s, "other")}
	ids := []string{}
	for i, token := range tokens {
		id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, TaskInput{Type: "courseware", Topic: "并发恢复验收", UseKB: true, IdempotencyKey: newID()}, 202))["id"].(string)
		ids = append(ids, id)
		tx, err := s.tx(context.Background(), int64(i+1))
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(context.Background(), "UPDATE tasks SET mode='live',input=jsonb_set(input,'{use_kb}','false') WHERE id=$1", id)
		if err == nil {
			err = tx.Commit(context.Background())
		} else {
			tx.Rollback(context.Background())
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	started := make(chan struct{}, 8)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		started <- struct{}{}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer provider.Close()
	s.cfg.Mode = "live"
	s.cfg.CoursewareLLMURL = provider.URL
	s.cfg.CoursewareLLMKey = "fixture"
	s.cfg.CoursewareLLMModel = "blocked-planning-fixture"
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	done := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { done <- s.workOne(ctx) }()
	}
	drained := false
	defer func() {
		stop()
		if drained {
			return
		}
		for i := 0; i < 2; i++ {
			select {
			case <-done:
			case <-time.After(20 * time.Second):
				t.Error("worker failed to drain")
			}
		}
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(35 * time.Second):
			t.Fatal("two tenants did not start concurrently")
		}
	}
	// Let a real sixty-second heartbeat run. Read task state through HTTP; the
	// administrator alone sets a short lease to distinguish renewal from no-op.
	for i, id := range ids {
		tx, err := s.tx(context.Background(), int64(i+1))
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(context.Background(), "UPDATE tasks SET lease_until=now()+interval '10 seconds' WHERE id=$1", id)
		if err == nil {
			err = tx.Commit(context.Background())
		} else {
			tx.Rollback(context.Background())
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(65 * time.Second)
	if err := s.recoverExpiredTasks(context.Background()); err != nil {
		t.Fatal(err)
	}
	for i, id := range ids {
		v := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, tokens[i], nil, 200))
		if v["status"] != "researching" || v["charge_state"] != "reserved" {
			t.Fatalf("heartbeat failed: %v", v["status"])
		}
	}
	request := filepath.Join(t.TempDir(), "shutdown.request")
	go watchShutdownFile(ctx, request, stop)
	if err := os.WriteFile(request, []byte("shutdown"), 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(4 * time.Second):
		t.Fatal("shutdown request not observed")
	}
	for i := 0; i < 2; i++ {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(20 * time.Second):
			t.Fatal("worker did not stop")
		}
	}
	drained = true
	for i, id := range ids {
		v := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, tokens[i], nil, 200))
		if v["status"] != "pending" || v["charge_state"] != "reserved" {
			t.Fatalf("shutdown lost reservation/checkpoint: %v", v["status"])
		}
		worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", tokens[i], nil, 200)
	}
}

func TestRemediationObservabilityAndStreamHTTP(t *testing.T) {
	s := &Server{mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /panic", func(http.ResponseWriter, *http.Request) { panic("fixture secret must not be returned") })
	s.mux.HandleFunc("GET /stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: ready\n\n"))
		w.(http.Flusher).Flush()
	})
	w := httptest.NewRecorder()
	s.serveObserved(w, httptest.NewRequest("GET", "/panic", nil))
	var v map[string]string
	json.Unmarshal(w.Body.Bytes(), &v)
	if w.Code != 500 || v["request_id"] == "" || v["request_id"] != w.Header().Get("X-Request-ID") {
		t.Fatal("panic did not preserve a correlated error")
	}
	w = httptest.NewRecorder()
	s.serveObserved(w, httptest.NewRequest("GET", "/stream", nil))
	if !w.Flushed || w.Body.String() != "data: ready\n\n" || w.Header().Get("X-Request-ID") == "" {
		t.Fatal("stream flush lost through middleware")
	}
}
