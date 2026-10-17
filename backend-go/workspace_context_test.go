package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// The public brief endpoint owns validation and tenant authorization; the only
// fixture is the external provider. No production tasks or credits are used.
func TestWorkspaceContextCompactionHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	tasks := worksCall(t, s, "GET", "/api/tasks", token, nil, 200).Body.String()
	var prompt string
	summary := "当前历史课件；保留史料讨论；已取消分组表演；不继承上一作品的制作授权。"
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
			Stream bool `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		for _, message := range request.Messages {
			prompt += message.Content
		}
		result := map[string]any{"reply": "已理解，继续确认史料讨论的安排。", "tool": "courseware", "action": "discuss", "settings": map[string]any{}, "requirements": "保留史料讨论。", "questions": []any{}, "assumptions": []any{}}
		if summary != "" {
			result["context_summary"] = summary
		}
		raw := string(asJSON(result))
		if request.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			frame := map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": raw}, "finish_reason": "stop"}}}
			fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", asJSON(frame))
		} else {
			respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": raw}}}})
		}
	}))
	defer provider.Close()
	s.cfg.Mode = "live"
	s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = provider.URL, "test", "test"
	body := map[string]any{"compact_context": true, "context_summary": "上轮记忆：不要显示学生真实姓名；分组表演已取消。", "input": TaskInput{Topic: "中国近代史", UseWeb: true}, "messages": []map[string]string{{"role": "user", "content": "保留史料讨论，不要开始制作。"}}}
	worksCall(t, s, "POST", "/api/workspace/brief", "", body, 401)
	response := worksObject(t, worksCall(t, s, "POST", "/api/workspace/brief", token, body, 200))
	if response["context_summary"] != summary {
		t.Fatal("completed context memory was not returned")
	}
	if !strings.Contains(prompt, "不要显示学生真实姓名") || !strings.Contains(prompt, "分组表演已取消") {
		t.Fatal("previous context did not reach the model")
	}
	body["context_summary"] = strings.Repeat("长", 3201)
	worksCall(t, s, "POST", "/api/workspace/brief", token, body, 400)
	body["context_summary"] = "旧约定，仅作为教师历史数据。"
	summary = strings.Repeat("长", 3201)
	worksCall(t, s, "POST", "/api/workspace/brief", token, body, 422)
	summary = ""
	worksCall(t, s, "POST", "/api/workspace/brief", token, body, 422)
	body["stream"] = true
	stream := worksCall(t, s, "POST", "/api/workspace/brief", token, body, 200).Body.String()
	if !strings.Contains(stream, "event: error") || strings.Contains(stream, "event: done") {
		t.Fatal("missing summary was accepted as a completed stream")
	}
	summary = "当前历史课件；保留史料讨论；表演已取消。"
	stream = worksCall(t, s, "POST", "/api/workspace/brief", token, body, 200).Body.String()
	if !strings.Contains(stream, "event: done") || !strings.Contains(stream, "context_summary") {
		t.Fatal("completed stream lost the summary")
	}
	// Requests carry their own memory; another authenticated member must not
	// acquire the first member's compressed context through server state.
	other := worksLogin(t, s, "admin")
	prompt = ""
	delete(body, "context_summary")
	delete(body, "compact_context")
	worksCall(t, s, "POST", "/api/workspace/brief", other, body, 200)
	if strings.Contains(prompt, "旧约定，仅作为教师历史数据") {
		t.Fatal("context leaked between members")
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before || worksCall(t, s, "GET", "/api/tasks", token, nil, 200).Body.String() != tasks {
		t.Fatal("compaction changed billing or replayed a task")
	}
}

func TestLiveWorkspaceContextCompactionHTTP(t *testing.T) {
	if os.Getenv("TEST_LIVE_CONTEXT") != "1" {
		t.Skip("live provider opt-in required")
	}
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	tasks := worksCall(t, s, "GET", "/api/tasks", token, nil, 200).Body.String()
	s.cfg.Mode = "live"
	s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = os.Getenv("COURSEWARE_LLM_BASE_URL"), os.Getenv("COURSEWARE_LLM_API_KEY"), os.Getenv("COURSEWARE_LLM_MODEL")
	body := map[string]any{"stream": true, "compact_context": true, "context_summary": "当前在准备中国近代史复习课。已确认保留史料讨论。分组表演已经取消，不要再安排。上一作品的制作指令已经结束，当前没有开始制作授权。", "input": TaskInput{Topic: "中国近代史", Subject: "历史", UseWeb: true, Theme: "coral", Requirements: "只整理需求，保留史料讨论，取消分组表演。"}, "messages": []map[string]string{{"role": "user", "content": "补充：学生基础薄弱，答案放在教师备注。先不要开始制作，整理这些要求就好。"}}}
	stream := worksCall(t, s, "POST", "/api/workspace/brief", token, body, 200).Body.String()
	var result map[string]any
	for _, frame := range strings.Split(stream, "\n\n") {
		if strings.HasPrefix(frame, "event: done\ndata: ") {
			if err := json.Unmarshal([]byte(strings.TrimPrefix(frame, "event: done\ndata: ")), &result); err != nil {
				t.Fatal(err)
			}
		}
	}
	if result == nil {
		t.Fatal("real model did not complete a validated compacted SSE response")
	}
	memory, _ := result["context_summary"].(string)
	if strings.TrimSpace(memory) == "" || !strings.Contains(memory, "史料") || !strings.Contains(memory, "表演") || !strings.Contains(memory, "备注") || result["action"] != "discuss" {
		t.Fatal("real memory lost a teaching constraint or generated without authorization")
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before || worksCall(t, s, "GET", "/api/tasks", token, nil, 200).Body.String() != tasks {
		t.Fatal("compaction affected tasks or points")
	}
	t.Logf("Real model returned %d-rune memory, preserved history discussion, canceled activity and teacher notes; no task or points changed", len([]rune(memory)))
}
