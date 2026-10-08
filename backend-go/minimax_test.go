package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiniMaxV2Contract(t *testing.T) {
	posts, gets, deletes := 0, 0, 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer video-only-key" {
			t.Error("wrong credential")
		}
		switch r.Method {
		case "POST":
			posts++
			if r.URL.Path != "/v2/video_generation" {
				t.Error("wrong create path", r.URL.Path)
			}
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["resolution"] != "768P" || body["generate_audio"] != nil || body["model"] != "MiniMax-H3" {
				t.Error("wrong MiniMax payload", body)
			}
			respond(w, 200, map[string]string{"task_id": "12345"})
		case "GET":
			gets++
			if r.URL.Path != "/v2/query/video_generation/12345" {
				t.Error("wrong query path", r.URL.Path)
			}
			respond(w, 200, map[string]any{"task": map[string]any{"id": "12345", "status": "succeeded", "model": "MiniMax-H3", "content": map[string]string{"url": "https://media.example/result.mp4"}, "usage": map[string]int{"total_tokens": 900, "completion_tokens": 800, "output_seconds": 5}}})
		case "DELETE":
			deletes++
			if r.URL.Path != "/v2/video_generation/12345" {
				t.Error("wrong cancel path", r.URL.Path)
			}
			respond(w, 200, map[string]string{"action": "cancelled"})
		}
	}))
	defer provider.Close()
	s := &Server{cfg: Config{VideoProvider: "minimax", VideoURL: provider.URL + "/v2/video_generation", VideoKey: "video-only-key", VideoModel: "MiniMax-H3", VideoDuration: "5"}, http: provider.Client()}
	ctx := context.Background()
	created, err := s.arkVideoRequest(ctx, "POST", "", map[string]any{"model": "MiniMax-H3", "content": []map[string]string{{"type": "text", "text": "floating wood"}}, "generate_audio": false, "duration": 5, "ratio": "16:9"})
	if err != nil || created.ID != "12345" {
		t.Fatal("create", created, err)
	}
	result, err := s.arkVideoRequest(ctx, "GET", created.ID, nil)
	if err != nil || result.Status != "succeeded" || result.Content.VideoURL != "https://media.example/result.mp4" || result.Usage.CompletionTokens != 800 || result.Usage.OutputSeconds != 5 {
		t.Fatal("query", result, err)
	}
	if _, err = s.arkVideoRequest(ctx, "DELETE", created.ID, nil); err != nil {
		t.Fatal(err)
	}
	if posts != 1 || gets != 1 || deletes != 1 {
		t.Fatal("unexpected request count")
	}
	for _, tc := range []struct {
		model, duration string
		valid           bool
	}{{"MiniMax-H3", "4", true}, {"MiniMax-H3", "3", false}, {"MiniMax-H3-Max", "4", false}, {"MiniMax-H3-Max", "5", true}, {"text-model", "5", false}} {
		s.cfg.VideoModel, s.cfg.VideoDuration = tc.model, tc.duration
		if s.videoConfigured() != tc.valid {
			t.Errorf("preflight %v", tc)
		}
	}
	s.cfg.VideoModel, s.cfg.VideoDuration = "MiniMax-H3", "5"
	s.cfg.VideoURL += " \\"
	if s.videoConfigured() {
		t.Fatal("curl continuation accepted as endpoint")
	}
}
