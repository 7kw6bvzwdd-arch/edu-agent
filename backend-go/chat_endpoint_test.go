package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChatRejectsResponsesProtocol(t *testing.T) {
	for _, address := range []string{"https://example.com/api/v3/responses", "https://example.com/api/v3/responses/?version=test"} {
		endpoint, err := chatEndpoint(address)
		if err == nil || endpoint != "" || !strings.Contains(err.Error(), "Chat Completions") {
			t.Fatalf("Responses endpoint should produce actionable configuration error, got %q, %v", endpoint, err)
		}
	}
}

func TestChatAcceptsBaseOrCompleteEndpoint(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	var actor Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &actor)
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/v3/chat/completions" || r.URL.Query().Get("version") != "test" {
			w.WriteHeader(404)
			return
		}
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("provider request contract changed")
		}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"reply":"ok"}`}}}})
	}))
	defer provider.Close()
	for _, suffix := range []string{"/api/v3?version=test", "/api/v3/?version=test", "/api/v3/chat/completions?version=test", "/api/v3/chat/completions/?version=test"} {
		content, err := s.chatWithProvider(context.Background(), actor, "", provider.URL+suffix, "test-key", "test-model", "courseware.brief", "test", 1000)
		if err != nil || content != `{"reply":"ok"}` {
			t.Errorf("endpoint %s: %v", suffix, err)
		}
	}
	if calls != 4 {
		t.Fatal("unexpected provider requests", calls)
	}
}
