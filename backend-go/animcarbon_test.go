package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A fixture provider deliberately returns the text-only output reported by the teacher.
func TestAnimationRejectsTextOnlyCarbonHTTP(t *testing.T) {
	s := worksServer(t)
	s.cfg.Mode = "live"
	raw, err := os.ReadFile("../fixtures/animation-v2-lesson.json")
	if err != nil {
		t.Fatal(err)
	}
	var script AnimScript
	if err = json.Unmarshal(raw, &script); err != nil {
		t.Fatal(err)
	}
	script.Title = "碳单质的多样性"
	for i := range script.Scenes {
		scene := &script.Scenes[i]
		scene.Title, scene.Intent = "比较碳结构", "比较结构与性质"
		for j := range scene.Objects {
			scene.Objects[j].Text = "金刚石与石墨"
			scene.Objects[j].Items = []string{"金刚石较硬", "石墨层间易滑动"}
		}
		for j := range scene.Narration {
			scene.Narration[j].Display = "同学们，比较金刚石和石墨，观察碳原子排列与性质的联系。"
			scene.Narration[j].Speech = scene.Narration[j].Display
		}
	}
	raw = asJSON(script)
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(raw)}}}})
	}))
	defer provider.Close()
	s.cfg.AnimationLLMURL, s.cfg.AnimationLLMKey, s.cfg.AnimationLLMModel = provider.URL, "fixture", "fixture"
	s.cfg.TTS = ttsConfig{Provider: "volcengine", Protocol: "create", URL: provider.URL + "/tts", Key: "fixture", Model: "seed-audio-1.0", Unit: "sentence", Format: "mp3", SampleRate: 48000}
	token := worksLogin(t, s, "teacher")
	before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, map[string]any{"type": "animation", "animation_engine": "html", "subject": "化学", "topic": "碳单质的多样性：结构决定性质", "requirements": "动态比较金刚石、石墨和C60结构", "idempotency_key": newID()}, 202))["id"].(string)
	if err = s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	state := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id+"/animation", token, nil, 200))
	if state["status"] != "failed" {
		t.Fatal("text-only carbon output was accepted as a teaching animation")
	}
	if calls != 2 {
		t.Fatalf("bounded repair calls = %d, want 2", calls)
	}
	after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	if before["balance"] != after["balance"] || before["reserved"] != after["reserved"] {
		t.Fatal("invalid plan changed quota")
	}
}

// Legacy scene policies still protect saved component animations. New task
// authoring is checked through the html.direct@1 contract.
func TestAnimationLegacyVisualPlanCompatibility(t *testing.T) {
	raw, err := os.ReadFile("../fixtures/animation-v2-science.json")
	if err != nil {
		t.Fatal(err)
	}
	var script AnimScript
	if err = json.Unmarshal(raw, &script); err != nil {
		t.Fatal(err)
	}
	script.Representative = "molecule"
	if problems := animAuthorProblems(script, TaskInput{Topic: "凸透镜、分子结构与植物细胞结构"}); len(problems) != 0 {
		t.Fatalf("valid relevant legacy representative rejected: %v", problems)
	}
	for i := range script.Scenes {
		script.Scenes[i].Objects = []AnimObject{{ID: "visual", Type: "scientific", Slot: "main", Params: map[string]float64{"progress": 0}, Scientific: &ScientificDiagram{Kind: "scene", Title: "位置示意", Scene: &DrawingScene{DurationMS: 2500, Objects: []DrawingObject{{ID: "ball", Type: "ellipse", X: .5, Y: .5, Width: .1, Height: .1}}, Actions: []DrawingAction{{Object: "ball", Op: "move", X: f64(.5), Y: f64(.5), DurationMS: 2500}}}}}}
	}
	problems := animAuthorProblems(script, TaskInput{Topic: "观察物体的位置变化", Requirements: "物体实际运动"})
	if len(problems) == 0 || !strings.Contains(strings.Join(problems, "\n"), "未满足教学图形动作") {
		t.Fatalf("same-position legacy motion bypassed validation: %v", problems)
	}
}

func TestAnimationLegacyTextPlanHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	raw, err := os.ReadFile("../fixtures/animation-v2-lesson.json")
	if err != nil {
		t.Fatal(err)
	}
	var script AnimScript
	if err = json.Unmarshal(raw, &script); err != nil {
		t.Fatal(err)
	}
	before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	for _, step := range []struct{ status, action string }{{"anim_plan", "confirm-plan"}, {"anim_representative", "confirm-representative"}, {"anim_acceptance", "accept"}} {
		in := TaskInput{AnimationEngine: "html", Topic: "碳单质的多样性：结构决定性质"}
		id := worksFixture(t, s, token, "animation", step.status, in.Topic, in)
		var a Actor
		json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
		tx, err := s.tx(context.Background(), a.TenantID)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(context.Background(), "UPDATE tasks SET animation=$1,revision=1 WHERE id=$2", asJSON(animWorkflow{Script: script, RepHash: "legacy", BuildHash: "legacy"}), id)
		if err != nil {
			tx.Rollback(context.Background())
			t.Fatal(err)
		}
		if err = tx.Commit(context.Background()); err != nil {
			t.Fatal(err)
		}
		worksCall(t, s, "POST", "/api/tasks/"+id+"/animation/"+step.action, token, map[string]any{"revision": 1, "hash": "legacy"}, 400)
	}
	after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	if before["balance"] != after["balance"] || before["reserved"] != after["reserved"] {
		t.Fatal("legacy text plan changed quota")
	}
}

func TestAnimationSavedVisualPreviewHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	raw, err := os.ReadFile("../fixtures/animation-v2-carbon.json")
	if err != nil {
		t.Fatal(err)
	}
	var script AnimScript
	if err = json.Unmarshal(raw, &script); err != nil {
		t.Fatal(err)
	}
	id := worksFixture(t, s, token, "animation", "anim_plan", script.Title, TaskInput{AnimationEngine: "html", Topic: script.Title})
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	// Saved reviewed bytes remain available even when the renderer is offline.
	bundle := scienceRender{Image: "data:image/png;base64,c2F2ZWQ=", Animation: "data:image/gif;base64,c2F2ZWQ=", Version: "science-visuals-v1"}
	data := asJSON(bundle)
	path := filepath.Join(s.cfg.Storage, "saved-visual.json")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	flow := animWorkflow{Script: script, Visuals: map[string]animVisualAsset{animScienceKey(script.Scenes[0].Objects[0].Scientific): {Path: path, Hash: animDigest(data)}}}
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), "UPDATE tasks SET animation=$1,revision=1 WHERE id=$2", asJSON(flow), id)
	if err != nil {
		tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.cfg.RenderURL = "http://127.0.0.1:1"
	endpoint := "/api/tasks/" + id + "/animation/preview-visual"
	body := map[string]any{"revision": 1, "scene_id": script.Scenes[0].ID}
	result := worksObject(t, worksCall(t, s, "POST", endpoint, token, body, 200))
	if result["animation"] != bundle.Animation {
		t.Fatal("saved preview changed reviewed bytes")
	}
	if err = os.WriteFile(path, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	worksCall(t, s, "POST", endpoint, token, body, 422)
}
