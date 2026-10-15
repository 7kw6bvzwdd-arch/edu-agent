package main

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// Opt in explicitly: this uses the configured provider and incurs model usage,
// but keeps tasks, balances and artifacts in the isolated test database/storage.
func TestLiveCoursewareProgress(t *testing.T) {
	if os.Getenv("TEST_LIVE_COURSEWARE") != "1" {
		t.Skip("requires explicit live model opt-in")
	}
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = os.Getenv("LLM_BASE_URL"), os.Getenv("LLM_API_KEY"), os.Getenv("LLM_MODEL")
	s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = os.Getenv("COURSEWARE_LLM_BASE_URL"), os.Getenv("COURSEWARE_LLM_API_KEY"), os.Getenv("COURSEWARE_LLM_MODEL")
	in := TaskInput{Type: "courseware", Topic: "浮力", Subject: "物理", Grade: "八年级", Pages: 12, Theme: "classroom", UseKB: true, PreviewFirst: true, IdempotencyKey: newID()}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	route := "/api/tasks/" + id
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	preview := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if preview["status"] != "awaiting_preview" {
		t.Fatalf("fixture preview: %v (%v)", preview["status"], preview["error"])
	}
	locked := preview["result"].(map[string]any)["slides"]
	t.Log("Sample preview completed; testing real model expansion from three approved fixture pages")
	for _, entry := range preview["slide_reviews"].([]any) {
		meta := entry.(map[string]any)
		worksCall(t, s, "POST", route+"/slides/"+meta["id"].(string)+"/review", token, map[string]any{"revision": preview["revision"], "hash": meta["hash"], "approved": true}, 200)
	}
	worksCall(t, s, "POST", route+"/expand", token, map[string]any{"revision": preview["revision"]}, 202)
	s.cfg.Mode = "live"
	tx, err := s.tx(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(context.Background(), "UPDATE tasks SET mode='live' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	task := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if task["status"] != "awaiting_confirmation" {
		t.Fatalf("live expansion: %v (%v)", task["status"], task["error"])
	}
	var d Deck
	json.Unmarshal(asJSON(task["result"]), &d)
	// The renderer may add continuation pages to keep diagrams readable.
	if len(d.Slides) < 12 {
		t.Fatalf("live expansion has only %d pages", len(d.Slides))
	}
	if !reflect.DeepEqual(locked, task["result"].(map[string]any)["slides"].([]any)[:3]) {
		t.Fatal("live expansion changed the three approved slides")
	}
	if issues := coursewareQualityIssues(d, 12); len(issues) != 0 {
		t.Fatal("live teaching checks:", issues)
	}
	stages := map[string]bool{}
	for _, row := range task["events"].([]any) {
		stages[row.(map[string]any)["stage"].(string)] = true
	}
	for _, stage := range []string{"batch_complete_6", "batch_complete_9", "batch_complete_12"} {
		if !stages[stage] {
			t.Errorf("missing live progress %s", stage)
		}
	}
	artifact := worksCall(t, s, "GET", route+"/preview-pptx", token, nil, 200).Body.Bytes()
	if len(artifact) < 1000 || string(artifact[:2]) != "PK" {
		t.Fatal("live result is not a PPTX")
	}
	t.Logf("Live expansion generated %d pages including layout continuations (%d PPTX bytes); progress events verified", len(d.Slides), len(artifact))
	worksCall(t, s, "POST", route+"/cancel", token, nil, 200)
}
