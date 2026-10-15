package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCoursewareBriefDoesNotCreateOrCharge(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	tasks := worksCall(t, s, "GET", "/api/tasks", token, nil, 200).Body.String()
	body := map[string]any{"input": TaskInput{Topic: "浮力", Subject: "物理", Grade: "八年级", Pages: 8}, "messages": []map[string]string{{"role": "user", "content": "45分钟，突出实验，学生基础弱"}}}
	worksCall(t, s, "POST", "/api/courseware/brief", "", body, 401)
	v := worksObject(t, worksCall(t, s, "POST", "/api/courseware/brief", token, body, 200))
	if v["mode"] != "sample" || v["reply"] == "" {
		t.Fatal("explicit sample response required", v)
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before {
		t.Fatal("brief charged points")
	}
	if worksCall(t, s, "GET", "/api/tasks", token, nil, 200).Body.String() != tasks {
		t.Fatal("brief created an unfinished work")
	}
	content := `{"reply":"先安排实验观察，再讨论浮力方向。"}`
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req struct {
			Model    string
			Messages []struct{ Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if req.Model != "brief-test-model" || r.Header.Get("Authorization") != "Bearer brief-test-key" || !strings.Contains(string(asJSON(req.Messages)), "45分钟，突出实验，学生基础弱") {
			t.Error("brief did not use the courseware model and supplied requirements")
		}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
	}))
	defer provider.Close()
	s.cfg.Mode = "live"
	s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = provider.URL, "brief-test-key", "brief-test-model"
	v = worksObject(t, worksCall(t, s, "POST", "/api/courseware/brief", token, body, 200))
	if v["mode"] != "live" || v["reply"] != "先安排实验观察，再讨论浮力方向。" {
		t.Fatal("live brief must return the provider response")
	}
	content = `{"reply":12}`
	worksCall(t, s, "POST", "/api/courseware/brief", token, body, 422)
	invalid := map[string]any{"input": body["input"], "messages": []map[string]string{{"role": "system", "content": "untrusted override"}}}
	worksCall(t, s, "POST", "/api/courseware/brief", token, invalid, 400)
	if calls != 2 || worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before || worksCall(t, s, "GET", "/api/tasks", token, nil, 200).Body.String() != tasks {
		t.Fatal("live brief, invalid reply or invalid input created work or charged points")
	}
}
