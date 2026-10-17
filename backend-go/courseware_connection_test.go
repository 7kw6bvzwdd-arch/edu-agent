package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The configured gateway advertises HTTP/2 but rejects full teaching requests.
// Exercise the authenticated HTTP boundary, including the actual upstream stream.
func TestCoursewareGatewayHTTP1CompatibilityHTTP(t *testing.T) {
	calls := 0
	provider := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.ProtoMajor != 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		var request struct {
			Stream bool `json:"stream"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Error("invalid provider request")
			return
		}
		result := `{"reply":"先确认这堂课用于新授还是复习。","tool":"courseware","action":"discuss","settings":{"topic":"二次函数","subject":"数学"},"requirements":"制作二次函数课件","questions":[],"assumptions":[]}`
		if request.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", asJSON(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": result}, "finish_reason": "stop"}}}))
		} else {
			respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": result}}}})
		}
	}))
	provider.EnableHTTP2 = true
	provider.StartTLS()
	defer provider.Close()
	original := http.DefaultTransport
	transport := original.(*http.Transport).Clone()
	transport.TLSClientConfig = provider.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	transport.ForceAttemptHTTP2 = true
	http.DefaultTransport = transport
	defer func() { http.DefaultTransport = original; transport.CloseIdleConnections() }()
	s := worksServer(t)
	defer s.http.CloseIdleConnections()
	token := worksLogin(t, s, "teacher")
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	tasks := worksCall(t, s, "GET", "/api/tasks", token, nil, 200).Body.String()
	s.cfg.Mode = "live"
	s.cfg.CoursewareLLMURL = provider.URL
	s.cfg.CoursewareLLMKey = "test"
	s.cfg.CoursewareLLMModel = "deepseek-flash"
	for _, route := range []string{"/api/workspace/brief", "/api/courseware/brief"} {
		for _, stream := range []bool{true, false} {
			payload := map[string]any{"stream": stream, "intent": "discuss", "input": TaskInput{Type: "courseware", Topic: "二次函数", Subject: "数学", Grade: "九年级", LessonMinutes: 45}, "messages": []map[string]string{{"role": "user", "content": "制作二次函数课件"}}}
			response := worksCall(t, s, "POST", route, token, payload, 200).Body.String()
			if stream {
				if !strings.Contains(response, "event: delta") || !strings.Contains(response, "event: done") || strings.Contains(response, "event: error") {
					t.Fatalf("%s stream failed: %s", route, response)
				}
			} else if !strings.Contains(response, `"mode":"live"`) {
				t.Fatalf("%s did not return a live reply", route)
			}
		}
	}
	if calls != 4 {
		t.Fatalf("provider calls=%d, want four without automatic replay", calls)
	}
	if http.DefaultTransport != transport || !transport.ForceAttemptHTTP2 || transport.Protocols != nil {
		t.Fatal("changed the shared transport")
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before || worksCall(t, s, "GET", "/api/tasks", token, nil, 200).Body.String() != tasks {
		t.Fatal("conversation changed billing or tasks")
	}
}
