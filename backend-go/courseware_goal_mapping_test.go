package main

import (
	"context"
	"encoding/json"
	"testing"
)

// A fixed sample exercises the same HTTP draft/approval/export workflow as live
// plans; it is not evidence of real-model teaching quality.
func TestCoursewareLearningGoalMappingHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	in := TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "classroom", Detail: "auto", UseKB: true, IdempotencyKey: newID()}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	route := "/api/tasks/" + id
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	task := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	result := task["result"].(map[string]any)
	for _, invalid := range []string{"unknown", "duplicate", "assessment_without_teaching", "self_dependency", "forward_dependency", "cyclic_dependency"} {
		var draft map[string]any
		json.Unmarshal(asJSON(result), &draft)
		row := draft["plan"].(map[string]any)["pages"].([]any)[0].(map[string]any)
		switch invalid {
		case "unknown":
			row["goal_refs"] = []string{"missing-goal"}
		case "duplicate":
			row["goal_refs"] = []string{"g1", "g1"}
		case "assessment_without_teaching":
			row["goal_refs"], row["checks_goals"] = []string{}, []string{"g1"}
		case "self_dependency":
			row["depends_on"] = []int{1}
		case "forward_dependency":
			row["depends_on"] = []int{2}
		case "cyclic_dependency":
			row["depends_on"] = []int{2}
			draft["plan"].(map[string]any)["pages"].([]any)[1].(map[string]any)["depends_on"] = []int{1}
		}
		worksCall(t, s, "POST", route+"/draft", token, map[string]any{"revision": task["revision"], "result": draft}, 400)
	}
	plan := result["plan"].(map[string]any)
	goals, ok := plan["learning_goals"].([]any)
	if !ok || len(goals) == 0 {
		t.Fatal("new courseware has no structured learning goals")
	}
	taught, assessed := map[string]bool{}, map[string]bool{}
	for _, entry := range plan["pages"].([]any) {
		page := entry.(map[string]any)
		for _, ref := range page["goal_refs"].([]any) {
			taught[ref.(string)] = true
		}
		if refs, ok := page["checks_goals"].([]any); ok {
			for _, ref := range refs {
				assessed[ref.(string)] = true
			}
		}
		if deps, ok := page["depends_on"].([]any); ok {
			for _, dep := range deps {
				if dep.(float64) >= page["position"].(float64) {
					t.Fatal("prerequisite appears after dependent page")
				}
			}
		}
	}
	for _, entry := range goals {
		g := entry.(map[string]any)
		if g["core"] == true && (!taught[g["id"].(string)] || !assessed[g["id"].(string)]) {
			t.Fatal("core goal is not taught and assessed")
		}
	}
	for _, entry := range task["slide_reviews"].([]any) {
		m := entry.(map[string]any)
		worksCall(t, s, "POST", route+"/slides/"+m["id"].(string)+"/review", token, map[string]any{"revision": task["revision"], "hash": m["hash"], "approved": true}, 200)
	}
	worksCall(t, s, "POST", route+"/expand", token, map[string]any{"revision": task["revision"]}, 202)
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	task = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	result = task["result"].(map[string]any)
	check, ok := result["deck_check"].(map[string]any)
	if !ok || check["status"] != "sample" || check["hash"] == "" {
		t.Fatal("full deck lacks explicit sample cross-page check")
	}
	// Removing the sole assessment must retain the draft and expose lost coverage.
	pages := result["slides"].([]any)
	kept := []any{}
	for _, entry := range pages {
		if entry.(map[string]any)["layout"] != "exercise" {
			kept = append(kept, entry)
		}
	}
	result["slides"] = kept
	worksCall(t, s, "POST", route+"/draft", token, map[string]any{"revision": task["revision"], "result": result}, 200)
	task = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	result = task["result"].(map[string]any)
	if result["deck_check"].(map[string]any)["status"] != "blocked" {
		t.Fatal("deleted assessment still claims goal coverage")
	}
	worksCall(t, s, "POST", route+"/cancel", token, nil, 200)
}
