package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUnitExplanationGetsFocusedReviewHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	id := worksFixture(t, s, token, "courseware", "succeeded", "单位说明", TaskInput{})
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	calls, unavailable := 0, false
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&payload)
		answer := `{"issues":[]}`
		if strings.Contains(payload.Messages[1].Content, "你是物理单位换算审核员") {
			answer = `{"issues":[{"object":"结果小1000倍","category":"answer","certainty":"confirmed","severity":"error","evidence":"cm³数值直接当m³会大1000000倍","repair":"修正倍率，保留10 N答案"}]}`
			if unavailable {
				answer = `{}`
			}
		}
		respond(w, 200, map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": answer}}}})
	}))
	defer provider.Close()
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = provider.URL, "test-key", "test-review"
	d := Deck{Slides: []Slide{{Notes: "V用cm³直接代入，结果小1000倍。"}}}
	issues, status, err := s.reviewTeachingPage(context.Background(), a, id, TaskInput{}, d)
	if err != nil || status != "checked" || blockingPageIssues(issues) == "" || calls != 2 {
		t.Fatal("general pass hid unit error", issues, status, err, calls)
	}
	unavailable = true
	if _, status, err = s.reviewTeachingPage(context.Background(), a, id, TaskInput{}, d); err == nil || status != "unavailable" {
		t.Fatal("failed numerical check accepted", status, err)
	}
	prior := calls
	d.Slides[0].Notes = "浸没后浮力方向竖直向上。"
	if _, _, err = s.reviewTeachingPage(context.Background(), a, id, TaskInput{}, d); err != nil || calls != prior+1 {
		t.Fatal("unrelated page received extra check", err, calls)
	}
}

func TestVisibleExerciseClassificationDoesNotCountAsAQuestion(t *testing.T) {
	bad := Slide{Layout: "exercise", Title: "分层作业", Bullets: []string{"A、B组必做：判断读图与两点法作图", "C组选做：由两点求解析式"}, Notes: "判断y=3x-1等；求过(0,-1)、(1,1)的一次函数。"}
	if len(exerciseVisibilityIssues(Deck{Slides: []Slide{bad}})) != 1 {
		t.Fatal("classification headings passed as complete homework")
	}
	for _, bullets := range [][]string{{"判断y=3x-1是否为一次函数，写出k、b。"}, {"解释完全浸没后浮力为何不随深度增加。"}, {"为什么并联电路一个支路断开，另一支路仍有电流？"}} {
		good := bad
		good.Bullets = bullets
		if len(exerciseVisibilityIssues(Deck{Slides: []Slide{good}})) > 0 {
			t.Fatal("specific question rejected", bullets)
		}
	}
	bad.Graph = &FunctionGraph{}
	if len(exerciseVisibilityIssues(Deck{Slides: []Slide{bad}})) > 0 {
		t.Fatal("program must defer meaningful diagrams to auditor")
	}
}
