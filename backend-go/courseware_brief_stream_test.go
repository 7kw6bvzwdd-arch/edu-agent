package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The real HTTP stream must reach the teacher before the provider finishes.
// A completed response split into delayed characters would fail this check.
func TestCoursewareBriefStreamsBeforeCompletionHTTP(t *testing.T) {
	s := worksServer(t)
	s.cfg.Mode = "live"
	token := worksLogin(t, s, "teacher")
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	tasks := worksCall(t, s, "GET", "/api/tasks", token, nil, 200).Body.String()
	release := make(chan struct{})
	defer close(release)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		json.NewDecoder(r.Body).Decode(&request)
		if request["stream"] != true || request["model"] != "deepseek-flash" || r.Header.Get("Authorization") != "Bearer brief-test" {
			t.Error("must request the courseware provider's actual stream")
		}
		thinking, _ := request["thinking"].(map[string]any)
		if thinking["type"] != "disabled" {
			t.Error("DeepSeek dialogue must reserve its bounded output budget for the streamed answer")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", asJSON(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": `{"reply":"先从生活情境开始`}}}}))
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
			return
		case <-time.After(5 * time.Second):
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", asJSON(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": `，再安排练习。","settings":{"topic":"二次函数","subject":"数学","grade":"九年级","lesson_minutes":40,"student_level":"已学一次函数"}}`}, "finish_reason": "stop"}}}))
		fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":20,\"completion_tokens\":40}}\n\ndata: [DONE]\n\n")
	}))
	defer provider.Close()
	s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = provider.URL, "brief-test", "deepseek-flash"
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	body := map[string]any{"stream": true, "input": TaskInput{Type: "courseware", UseWeb: true}, "messages": []map[string]string{{"role": "user", "content": "给九年级做二次函数，40分钟，学生学过一次函数"}}}
	request, _ := http.NewRequest("POST", server.URL+"/api/courseware/brief", bytes.NewReader(asJSON(body)))
	request.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("expected an authenticated SSE response, got %d %s", response.StatusCode, response.Header.Get("Content-Type"))
	}
	scanner := bufio.NewScanner(response.Body)
	seenDelta, seenTable, seenDone := false, false, false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: delta") && !seenDelta {
			seenDelta = true
			release <- struct{}{}
		}
		if strings.HasPrefix(line, "event: block") {
			seenTable = true
		}
		if strings.HasPrefix(line, "event: done") {
			seenDone = true
		}
		if strings.HasPrefix(line, "data: ") && strings.Contains(line, `"settings"`) {
			var event struct {
				Settings struct {
					Topic, Subject, Grade string
					Minutes               int `json:"lesson_minutes"`
				}
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
				t.Fatal(err)
			}
			if event.Settings.Topic != "二次函数" || event.Settings.Grade != "九年级" || event.Settings.Minutes != 40 {
				t.Fatal("conversation did not produce usable teaching settings", line)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if !seenDelta || !seenTable || !seenDone {
		t.Fatal("missing progressive text, requirements table or completion", seenDelta, seenTable, seenDone)
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before || worksCall(t, s, "GET", "/api/tasks", token, nil, 200).Body.String() != tasks {
		t.Fatal("conversation created a work or reserved points")
	}
}

func TestWorkspaceBriefProviderLimitHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "admin")
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	tasks := worksCall(t, s, "GET", "/api/tasks", token, nil, 200).Body.String()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, 429, map[string]any{"error": map[string]string{"code": "SetLimitExceeded", "message": "private-provider-message-must-not-leak"}})
	}))
	defer provider.Close()
	s.cfg.Mode = "live"
	s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = provider.URL, "private-test-key", "brief-test"
	for _, stream := range []bool{true, false} {
		for _, path := range []string{"/api/workspace/brief", "/api/courseware/brief"} {
			body := map[string]any{"stream": stream, "input": TaskInput{Topic: "二次函数", Subject: "物理", Grade: "八年级"}, "messages": []map[string]string{{"role": "user", "content": "制作二次函数课件"}}}
			want := 503
			if stream {
				want = 200
			}
			result := worksCall(t, s, "POST", path, token, body, want).Body.String()
			if !strings.Contains(result, "SetLimitExceeded") || !strings.Contains(result, "用量上限") || !strings.Contains(result, "成员制作积分") || strings.Contains(result, "private-") {
				t.Fatalf("provider limit must be actionable and redacted: %s", result)
			}
			if stream && (!strings.Contains(result, "event: error") || strings.Contains(result, "event: done") || strings.Contains(result, "展开教学设置")) {
				t.Fatal("provider limit cannot suggest bypassing through the same unavailable model", result)
			}
		}
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before || worksCall(t, s, "GET", "/api/tasks", token, nil, 200).Body.String() != tasks {
		t.Fatal("failed discussion changed tasks or points")
	}
}

func TestCoursewareBriefStreamFailureAndAccessHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	tasks := worksCall(t, s, "GET", "/api/tasks", token, nil, 200).Body.String()
	body := map[string]any{"stream": true, "intent": "generate", "input": TaskInput{Topic: "浮力", Subject: "物理", Grade: "八年级", UseKB: true}, "messages": []map[string]string{{"role": "user", "content": "直接制作，40分钟"}}}
	worksCall(t, s, "POST", "/api/courseware/brief", "", body, 401)
	foreign := map[string]any{"stream": true, "input": TaskInput{Topic: "浮力", ReferenceID: "another-members-private-reference"}, "messages": body["messages"]}
	worksCall(t, s, "POST", "/api/courseware/brief", token, foreign, 404)
	settings := `{"topic":"浮力","lesson_minutes":40}`
	truncated := false
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		content := `{"reply":"先看三页预览。","settings":` + settings + `}`
		fmt.Fprintf(w, "data: %s\n\n", asJSON(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": content}, "finish_reason": "stop"}}}))
		if !truncated {
			fmt.Fprint(w, "data: [DONE]\n\n")
		}
	}))
	defer provider.Close()
	s.cfg.Mode = "live"
	s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = provider.URL, "test", "stream-test"
	for _, tc := range []struct {
		name, settings string
		truncated      bool
	}{
		{"unverified_file_reference", `{"topic":"浮力","material_id":"private-document"}`, false},
		{"invalid_duration", `{"lesson_minutes":1}`, false},
		{"lost_connection", `{"topic":"浮力","lesson_minutes":40}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings, truncated = tc.settings, tc.truncated
			response := worksCall(t, s, "POST", "/api/courseware/brief", token, body, 200).Body.String()
			if !strings.Contains(response, "event: error") || strings.Contains(response, "event: done") || strings.Contains(response, "event: block") || strings.Contains(response, "private-document") {
				t.Fatal("incomplete or untrusted settings escaped the stream validation")
			}
		})
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before || worksCall(t, s, "GET", "/api/tasks", token, nil, 200).Body.String() != tasks {
		t.Fatal("failed stream changed work or quota")
	}
}

func TestCoursewareBriefDisconnectCancelsProviderHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	canceled := make(chan struct{})
	started := make(chan struct{})
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", asJSON(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": `{"reply":"正在整理`}}}}))
		w.(http.Flusher).Flush()
		close(started)
		select {
		case <-r.Context().Done():
			close(canceled)
		case <-time.After(8 * time.Second):
		}
	}))
	defer provider.Close()
	s.cfg.Mode = "live"
	s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = provider.URL, "test", "stream-test"
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	body := map[string]any{"stream": true, "input": TaskInput{Topic: "浮力"}, "messages": []map[string]string{{"role": "user", "content": "先讨论需求"}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/api/courseware/brief", bytes.NewReader(asJSON(body)))
	r.Header.Set("Authorization", "Bearer "+token)
	client := http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("provider did not start")
	}
	cancel()
	io.Copy(io.Discard, res.Body)
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("stopping conversation did not cancel the model request")
	}
}

func TestCoursewareBriefChineseSummaryLimitHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	input := TaskInput{Topic: "二次函数", Requirements: strings.Repeat("教学要求", 2000)}
	body := map[string]any{"stream": true, "input": input, "messages": []map[string]string{{"role": "user", "content": "继续讨论需求"}}}
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	if response := worksCall(t, s, "POST", "/api/courseware/brief", token, body, 200).Body.String(); !strings.Contains(response, "event: done") {
		t.Fatal("valid 8000-character Chinese summary was rejected")
	}
	input.Requirements += "字"
	body["input"] = input
	worksCall(t, s, "POST", "/api/courseware/brief", token, body, 400)
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before {
		t.Fatal("summary validation changed quota")
	}
}
