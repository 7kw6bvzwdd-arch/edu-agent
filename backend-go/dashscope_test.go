package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDashScopeVideoContract(t *testing.T) {
	posts, gets := 0, 0
	status := "SUCCEEDED"
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer video-only" {
			t.Error("wrong credential")
		}
		if r.Method == "POST" {
			posts++
			if r.URL.Path != "/api/v1/services/aigc/video-generation/video-synthesis" || r.Header.Get("X-DashScope-Async") != "enable" {
				t.Error("wrong asynchronous endpoint")
			}
			var body map[string]any
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				t.Fatal("invalid JSON")
			}
			input, _ := body["input"].(map[string]any)
			params, _ := body["parameters"].(map[string]any)
			if body["model"] != "wan2.7-t2v" || input["prompt"] != "教学测试" || params["duration"] != float64(5) || params["resolution"] != "720P" || params["ratio"] != "16:9" || params["prompt_extend"] != true || params["watermark"] != true {
				t.Error("wrong Wan payload", body)
			}
			if body["content"] != nil || body["generate_audio"] != nil {
				t.Error("Ark fields leaked")
			}
			respond(w, 200, map[string]any{"output": map[string]string{"task_id": "wan-test", "task_status": "PENDING"}})
		} else {
			gets++
			if r.Method != "GET" || r.URL.Path != "/api/v1/tasks/wan-test" {
				t.Error("wrong query endpoint", r.Method, r.URL.Path)
			}
			respond(w, 200, map[string]any{"output": map[string]string{"task_id": "wan-test", "task_status": status, "video_url": "https://media.example/video.mp4"}, "usage": map[string]int{"output_video_duration": 5}})
		}
	}))
	defer provider.Close()
	s := &Server{cfg: Config{VideoProvider: "dashscope", VideoURL: provider.URL + "/api/v1", VideoKey: "video-only", VideoModel: "wan2.7-t2v", VideoDuration: "5"}, http: provider.Client()}
	if !s.videoConfigured() {
		t.Fatal("Wan configuration rejected")
	}
	body := map[string]any{"model": "wan2.7-t2v", "content": []map[string]string{{"type": "text", "text": "教学测试"}}, "duration": 5, "ratio": "16:9", "generate_audio": false}
	v, err := s.arkVideoRequest(context.Background(), "POST", "", body)
	if err != nil || v.ID != "wan-test" || v.Status != "queued" {
		t.Fatal("create failed", err, v)
	}
	for remote, local := range map[string]string{"PENDING": "queued", "RUNNING": "running", "SUCCEEDED": "succeeded", "FAILED": "failed", "CANCELED": "cancelled", "UNKNOWN": "expired"} {
		status = remote
		v, err = s.arkVideoRequest(context.Background(), "GET", v.ID, nil)
		if err != nil || v.Status != local || v.Usage.OutputSeconds != 5 {
			t.Fatal("status/usage mapping failed", remote, v, err)
		}
	}
	_, err = s.arkVideoRequest(context.Background(), "DELETE", "wan-test", nil)
	if err == nil || posts != 1 || gets != 6 {
		t.Fatal("unsupported cancellation sent a request")
	}
	for _, base := range []string{provider.URL, provider.URL + "/api/v1", provider.URL + "/api/v1/services/aigc/video-generation/video-synthesis"} {
		s.cfg.VideoURL = base
		endpoint, err := s.videoEndpoint("GET", "wan-test")
		if err != nil || endpoint != provider.URL+"/api/v1/tasks/wan-test" {
			t.Fatal("endpoint normalization", base, endpoint, err)
		}
	}
	s.cfg.VideoURL = provider.URL + "/compatible-mode/v1"
	if s.videoConfigured() {
		t.Fatal("text API base accepted as video API")
	}
	s.cfg.VideoURL = provider.URL
	s.cfg.VideoModel = "wan2.6-i2v-flash"
	if s.videoConfigured() {
		t.Fatal("image model accepted without input image")
	}
}

func TestDashScopeRejectedSubmitIsNotRetriedOrLeaked(t *testing.T) {
	posts := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts++
		w.WriteHeader(503)
		w.Write([]byte("private prompt and secret-token"))
	}))
	defer provider.Close()
	s := &Server{cfg: Config{VideoProvider: "dashscope", VideoURL: provider.URL, VideoKey: "secret-token"}, http: provider.Client()}
	_, err := s.arkVideoRequest(context.Background(), "POST", "", map[string]any{"model": "wan2.7-t2v", "content": []map[string]string{{"type": "text", "text": "test"}}, "duration": 5, "ratio": "16:9"})
	if err == nil || posts != 1 || strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), "private prompt") {
		t.Fatal("paid retry or private error leak", posts, err)
	}
}

func TestDashScopeHTTPDelivery(t *testing.T) { cloudVideoHTTPDelivery(t, "dashscope") }
