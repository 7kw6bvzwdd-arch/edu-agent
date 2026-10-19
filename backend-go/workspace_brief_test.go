package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWorkspaceBriefToolRoutingHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	actor := worksObject(t, worksCall(t, s, "GET", "/api/auth/me", token, nil, 200))
	tx, err := s.tx(context.Background(), int64(actor["tenant_id"].(float64)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(context.Background(), "UPDATE members SET permissions=$1 WHERE id=$2", []string{"courseware.generate", "animation.render"}, int64(actor["member_id"].(float64))); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"reply":"可以先制作规则意识讲解方案。","tool":"animation","action":"generate","settings":{"topic":"规则意识","subject":"道德与法治"}}`}}}})
	}))
	defer provider.Close()
	s.cfg.Mode = "live"
	s.cfg.CoursewareLLMURL = provider.URL
	s.cfg.CoursewareLLMKey = "test"
	s.cfg.CoursewareLLMModel = "test"
	body := map[string]any{"input": TaskInput{Topic: "一次函数", UseWeb: true}, "messages": []map[string]string{{"role": "user", "content": "帮我制作一次函数教学动画"}}}
	worksCall(t, s, "POST", "/api/workspace/brief", "", body, 401)
	response := worksCall(t, s, "POST", "/api/workspace/brief", token, body, 200)
	if !strings.Contains(response.Body.String(), `"tool":"animation"`) || !strings.Contains(response.Body.String(), `"action":"generate"`) || !strings.Contains(response.Body.String(), `"subject":"道德与法治"`) {
		t.Fatal(response.Body.String())
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before {
		t.Fatal("conversation reserved credits")
	}
	body["tool"] = "shell"
	worksCall(t, s, "POST", "/api/workspace/brief", token, body, 400)
}
func TestWorkspaceToolDecisionRejectsUnknownTools(t *testing.T) {
	for _, tool := range []string{"shell", "https://example.com", "animation/delete"} {
		if err := validateWorkspaceDecision(Actor{Role: "admin"}, tool, "generate"); err == nil {
			t.Fatal("accepted unknown tool", tool)
		}
	}
	if err := validateWorkspaceDecision(Actor{Permissions: []string{"animation.render"}}, "animation", "generate"); err != nil {
		t.Fatal(err)
	}
	if err := validateWorkspaceDecision(Actor{Permissions: []string{"animation.render"}}, "courseware", "generate"); err == nil {
		t.Fatal("accepted unauthorized tool")
	}
	if err := validateWorkspaceDecision(Actor{Role: "admin"}, "animation", "delete"); err == nil {
		t.Fatal("accepted unknown action")
	}
}

func TestWorkspaceBriefPermissionHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	actor := worksObject(t, worksCall(t, s, "GET", "/api/auth/me", token, nil, 200))
	update := func(permissions []string) {
		tx, err := s.tx(context.Background(), int64(actor["tenant_id"].(float64)))
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		if _, err = tx.Exec(context.Background(), "UPDATE members SET permissions=$1 WHERE id=$2", permissions, int64(actor["member_id"].(float64))); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	tool := "animation"
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"reply":"需求已记录","tool":"` + tool + `","action":"discuss"}`}}}})
	}))
	defer provider.Close()
	s.cfg.Mode = "live"
	s.cfg.CoursewareLLMURL = provider.URL
	s.cfg.CoursewareLLMKey = "test"
	s.cfg.CoursewareLLMModel = "test"
	body := map[string]any{"input": TaskInput{Topic: "一次函数", UseWeb: true}, "messages": []map[string]string{{"role": "user", "content": "制作一次函数动画"}}}
	update([]string{"animation.render"})
	worksCall(t, s, "POST", "/api/workspace/brief", token, body, 200)
	body["tool"] = "courseware"
	worksCall(t, s, "POST", "/api/workspace/brief", token, body, 403)
	delete(body, "tool")
	tool = "courseware"
	worksCall(t, s, "POST", "/api/workspace/brief", token, body, 403)
	tool = "shell"
	worksCall(t, s, "POST", "/api/workspace/brief", token, body, 422)
	update([]string{})
	worksCall(t, s, "POST", "/api/workspace/brief", token, body, 403)
}

// Clarification is an HTTP contract; the provider is isolated at its API boundary.
func TestWorkspaceBriefClarifiesBeforeGeneratingHTTP(t *testing.T) {
	s := worksServer(t)
	s.cfg.Mode = "live"
	token := worksLogin(t, s, "teacher")
	actor := worksObject(t, worksCall(t, s, "GET", "/api/auth/me", token, nil, 200))
	tx, err := s.tx(context.Background(), int64(actor["tenant_id"].(float64)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(context.Background(), "UPDATE members SET permissions=$1 WHERE id=$2", []string{"courseware.generate"}, int64(actor["member_id"].(float64))); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	tasks := worksCall(t, s, "GET", "/api/tasks", token, nil, 200).Body.String()
	responseAction := "generate"
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		prompt := ""
		for _, message := range request.Messages {
			prompt += message.Content
		}
		if !strings.Contains(prompt, "后续只回应本轮新增或修改") || !strings.Contains(prompt, "追问和选项只放questions") {
			t.Error("brief did not instruct incremental replies with separate questions")
		}
		result := map[string]any{"reply": "已了解你想讲浮力，先确认这节课的用途。", "tool": "courseware", "action": responseAction, "settings": map[string]string{"topic": "浮力"}, "requirements": "面向基础薄弱学生，保留实验观察，避免先堆公式。", "questions": []any{map[string]any{"question": "这节课主要用于哪种教学？", "options": []string{"新课探究", "复习巩固", "习题讲评"}}}, "assumptions": []string{"课时暂按45分钟，可修改"}}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(asJSON(result))}}}})
	}))
	defer provider.Close()
	s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = provider.URL, "test", "test"
	body := map[string]any{"input": TaskInput{Topic: "浮力", UseWeb: true}, "messages": []map[string]string{{"role": "user", "content": "想做浮力课件，学生基础弱，想多做实验"}}}
	v := worksObject(t, worksCall(t, s, "POST", "/api/workspace/brief", token, body, 200))
	if v["action"] != "discuss" {
		t.Fatal("unanswered clarification started production", v)
	}
	if v["requirements"] != "面向基础薄弱学生，保留实验观察，避免先堆公式。" || len(v["questions"].([]any)) != 1 || len(v["assumptions"].([]any)) != 1 {
		t.Fatal("understanding and choices were lost", v)
	}
	body["intent"] = "generate"
	v = worksObject(t, worksCall(t, s, "POST", "/api/workspace/brief", token, body, 200))
	if v["action"] != "generate" || len(v["questions"].([]any)) != 0 {
		t.Fatal("explicit direct generation did not bypass optional questions", v)
	}
	responseAction = "discuss"
	v = worksObject(t, worksCall(t, s, "POST", "/api/workspace/brief", token, body, 200))
	if v["action"] != "discuss" {
		t.Fatal("explicit start bypassed model refusal for unsupported scope", v)
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before || worksCall(t, s, "GET", "/api/tasks", token, nil, 200).Body.String() != tasks {
		t.Fatal("clarification endpoint changed tasks or billing")
	}
}

// Explicit opt-in exercises the real provider with an isolated member and no
// production worker or task creation. No credentials are logged.
func TestLiveWorkspaceBriefStreamHTTP(t *testing.T) {
	if os.Getenv("TEST_LIVE_NEEDS") != "1" {
		t.Skip("live provider opt-in required")
	}
	s := worksServer(t)
	token := worksLogin(t, s, "admin")
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	s.cfg.Mode = "live"
	s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = os.Getenv("COURSEWARE_LLM_BASE_URL"), os.Getenv("COURSEWARE_LLM_API_KEY"), os.Getenv("COURSEWARE_LLM_MODEL")
	s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = os.Getenv("LLM_BASE_URL"), os.Getenv("LLM_API_KEY"), os.Getenv("LLM_MODEL")
	body := map[string]any{"stream": true, "intent": "discuss", "input": TaskInput{Subject: "物理", Grade: "八年级", LessonMinutes: 45, UseWeb: true}, "messages": []map[string]string{{"role": "user", "content": "制作二次函数课件"}}}
	response := worksCall(t, s, "POST", "/api/workspace/brief", token, body, 200).Body.String()
	if strings.Contains(response, "event: error") || !strings.Contains(response, "event: done") {
		t.Fatalf("stream did not complete: %s", response)
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before {
		t.Fatal("discussion changed points")
	}
	t.Log("Real streaming brief completed, requirements clarified, no task created")
}

func TestLiveDeepSeekCoursewareRender(t *testing.T) {
	if os.Getenv("TEST_LIVE_NEEDS") != "1" {
		t.Skip("live provider opt-in required")
	}
	s := worksServer(t)
	token := worksLogin(t, s, "admin")
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	s.cfg.Mode = "live"
	s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = os.Getenv("COURSEWARE_LLM_BASE_URL"), os.Getenv("COURSEWARE_LLM_API_KEY"), os.Getenv("COURSEWARE_LLM_MODEL")
	if s.cfg.CoursewareLLMModel != "deepseek-flash" {
		t.Fatal("DeepSeek generation configuration required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	in := TaskInput{Type: "courseware", Topic: "二次函数的顶点与对称轴", Subject: "数学", Grade: "九年级", Pages: 1, Theme: "ocean", Requirements: "只制作一页概念讲解。用y=(x-1)^2+2说明顶点(1,2)和对称轴x=1；条件与结论准确，不要封面、练习和整套目录。"}
	raw, err := s.agentChat(ctx, a, "", "courseware", "courseware.generate", coursewarePrompt(in, nil), 3000)
	if err != nil {
		t.Fatal(err)
	}
	var d Deck
	if err = decodeCourseware(raw, &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Slides) != 1 {
		t.Fatal("expected exactly one generated page")
	}
	d.Theme = in.Theme
	if err = validateDeckMinimum(d, 1); err != nil {
		t.Fatal(err)
	}
	artifact, err := s.renderPPTX(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(artifact), int64(len(artifact)))
	if err != nil {
		t.Fatal("renderer did not return a real PPTX")
	}
	found := false
	for _, file := range archive.File {
		if file.Name == "ppt/slides/slide1.xml" {
			found = true
		}
	}
	if !found {
		t.Fatal("PPTX is missing generated slide")
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before {
		t.Fatal("probe changed member points")
	}
	t.Logf("DeepSeek generated one real teaching page and renderer produced valid PPTX (%d bytes); no user task created", len(artifact))
}

func TestLiveWorkspaceNeedsHTTP(t *testing.T) {
	if os.Getenv("TEST_LIVE_NEEDS") != "1" {
		t.Skip("live provider opt-in required")
	}
	s := worksServer(t)
	s.cfg.Mode = "live"
	token := worksLogin(t, s, "admin")
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = os.Getenv("COURSEWARE_LLM_BASE_URL"), os.Getenv("COURSEWARE_LLM_API_KEY"), os.Getenv("COURSEWARE_LLM_MODEL")
	s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = os.Getenv("LLM_BASE_URL"), os.Getenv("LLM_API_KEY"), os.Getenv("LLM_MODEL")
	in := TaskInput{Topic: "浮力", Subject: "物理", Grade: "八年级", LessonMinutes: 45, UseWeb: true}
	messages := []map[string]string{}
	for index, content := range []string{"想做一个浮力课件", "八年级复习课，学生分不清物体体积和排液体积；没有实验器材，用给定数据讲解，不讲密度公式。", "不要追问了，按45分钟直接制作。"} {
		messages = append(messages, map[string]string{"role": "user", "content": content})
		body := map[string]any{"input": in, "messages": messages}
		if index == 2 {
			body["intent"] = "generate"
		}
		v := worksObject(t, worksCall(t, s, "POST", "/api/workspace/brief", token, body, 200))
		questions, _ := v["questions"].([]any)
		if index == 0 && (v["action"] != "discuss" || len(questions) == 0) {
			t.Fatal("vague request skipped contextual clarification", v)
		}
		if index == 2 && (v["action"] != "generate" || len(questions) != 0) {
			t.Fatal("direct generation did not respect teacher choice", v)
		}
		req, _ := v["requirements"].(string)
		if index > 0 && (!strings.Contains(req, "体积") || !strings.Contains(req, "数据")) {
			t.Fatal("confirmed teaching constraints were lost", v)
		}
		t.Logf("turn=%d action=%v reply=%v requirements=%v questions=%v assumptions=%v", index+1, v["action"], v["reply"], req, v["questions"], v["assumptions"])
		in.Requirements = req
		patch := map[string]any{}
		json.Unmarshal(asJSON(in), &patch)
		for key, value := range v["settings"].(map[string]any) {
			patch[key] = value
		}
		json.Unmarshal(asJSON(patch), &in)
		history := fmt.Sprint(v["reply"])
		for _, q := range questions {
			history += "\n" + q.(map[string]any)["question"].(string)
		}
		messages = append(messages, map[string]string{"role": "assistant", "content": history})
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before {
		t.Fatal("live discussion changed credits")
	}
}
