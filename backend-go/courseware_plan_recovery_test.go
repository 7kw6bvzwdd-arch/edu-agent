package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInvalidTeachingPlanCanRecoverWithoutTeacherClick(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	id := worksFixture(t, s, token, "courseware", "succeeded", "规划输出回归", TaskInput{})
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req struct{ Messages []struct{ Content string } }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if len(req.Messages) < 2 || !strings.Contains(req.Messages[1].Content, coursewareCompositionSkill) {
			t.Error("actual planning request must include the shared teaching composition skill")
		}
		minutes := 30.0
		if calls > 2 {
			minutes = 5
		}
		p := TeachingPlan{Summary: "概念、应用、练习", Pages: []TeachingPage{{Position: 1, Goal: "理解知识", Task: "概念", Minutes: minutes, Preview: true}, {Position: 2, Goal: "应用", Task: "例题", Minutes: minutes, Preview: true}, {Position: 3, Goal: "检查", Task: "练习", Minutes: minutes, Preview: true}}}
		p.LearningGoals = []LearningGoal{{ID: "g1", Text: "理解并应用概念", Core: true}}
		for i := range p.Pages {
			p.Pages[i].GoalRefs = []string{"g1"}
		}
		p.Pages[2].ChecksGoals = []string{"g1"}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(asJSON(p))}}}})
	}))
	defer provider.Close()
	s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "fixture", "generation"
	in := TaskInput{Detail: "default", LessonMinutes: 45, Topic: "浮力", Subject: "物理"}
	if _, err := s.planCourseware(context.Background(), a, id, in, nil); err == nil || !coursewareCanResume(err) {
		t.Fatal("invalid model plan became a terminal teacher action", err)
	}
	p, err := s.planCourseware(context.Background(), a, id, in, nil)
	if err != nil || p == nil || calls != 3 {
		t.Fatal("same task cannot repair plan", err, calls)
	}
}
