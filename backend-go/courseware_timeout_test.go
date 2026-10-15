package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProviderTimeoutIsNotReportedAsNetworkFailure(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	defer provider.Close()
	s := &Server{http: provider.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := s.postBytes(ctx, provider.URL, nil, "")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("task deadline lost: %v", err)
	}
	client := *s.http
	client.Timeout = 30 * time.Millisecond
	_, err = s.postBytesWithClient(context.Background(), &client, provider.URL, nil, "")
	var upstream providerCallError
	if !errors.As(err, &upstream) || !upstream.Temporary || !strings.Contains(err.Error(), "超时") {
		t.Fatalf("per-request timeout must remain retryable and explicit: %v", err)
	}
}

func TestExpansionDeadlineCoversBatchRetries(t *testing.T) {
	for _, pages := range []int{8, 12, 20} {
		batches := (pages - 3 + 2) / 3
		limit := coursewareExpansionLimit(pages)
		if limit < time.Duration(batches*2)*180*time.Second+2*time.Minute || limit > 40*time.Minute {
			t.Fatalf("%d pages have insufficient or unbounded stage time: %s", pages, limit)
		}
	}
}

type coursewareTransportFunc func(*http.Request) (*http.Response, error)

func (f coursewareTransportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestArkGLMCoursewareDisablesThinking(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	var actor Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &actor)
	var sent map[string]any
	s.http.Transport = coursewareTransportFunc(func(r *http.Request) (*http.Response, error) {
		sent = nil
		json.NewDecoder(r.Body).Decode(&sent)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"choices":[{"finish_reason":"stop","message":{"content":"{}"}}]}`))}, nil
	})
	for _, c := range []struct {
		host, model, operation string
		disabled               bool
	}{
		{"https://ark.cn-beijing.volces.com/api/v3", "glm-5-3-flash-260828", "courseware.expand", true},
		{"https://ark.cn-beijing.volces.com/api/v3", "glm-5-3-flash-260828", "courseware.plan_repair", true},
		{"https://ark.cn-beijing.volces.com/api/v3", "glm-5-3-flash-260828", "courseware.preview", true},
		{"https://other.example/v1", "glm-5-3-flash-260828", "courseware.expand", false},
		{"https://ark.cn-beijing.volces.com/api/v3", "another-model", "courseware.expand", false},
	} {
		_, err := s.chatWithProvider(context.Background(), actor, "", c.host, "test", c.model, c.operation, "test", 6000)
		if err != nil {
			t.Fatal(err)
		}
		thinking, _ := sent["thinking"].(map[string]any)
		if (thinking["type"] == "disabled") != c.disabled {
			t.Errorf("unexpected thinking mode for %s %s %s", c.host, c.model, c.operation)
		}
	}
}
