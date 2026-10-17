package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTMLAnimationWorkflowHTTP(t *testing.T)     { testHTMLAnimationWorkflow(t, "linear") }
func TestHTMLGeometryWorkflowHTTP(t *testing.T)      { testHTMLAnimationWorkflow(t, "geometry") }
func TestHTMLGeneralLessonWorkflowHTTP(t *testing.T) { testHTMLAnimationWorkflow(t, "lesson") }
func testHTMLAnimationWorkflow(t *testing.T, kind string) {
	if kind == "lesson" {
		t.Setenv("ANIMATION_EXPORT_SCRIPT", filepath.Join("..", "animation-video", "export.mjs"))
	}
	s := worksServer(t)
	s.cfg.Mode = "live"
	var script AnimScript
	if kind == "geometry" {
		script = loadGeometryFixture(t)
	} else {
		script = loadLinearKFixture(t)
	}
	llmCalls, ttsCalls := 0, 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tts" {
			ttsCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{"audio": base64.StdEncoding.EncodeToString(testMP3(250)), "subtitle": map[string]any{"sentences": []any{map[string]any{"text": "", "start_time": 240, "end_time": 5600}}}})
			return
		}
		llmCalls++
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(asJSON(script))}}}})
	}))
	defer provider.Close()
	s.cfg.AnimationLLMURL, s.cfg.AnimationLLMKey, s.cfg.AnimationLLMModel = provider.URL, "test-key", "deepseek-flash"
	s.cfg.TTS = ttsConfig{Provider: "volcengine", Protocol: "create", URL: provider.URL + "/tts", Key: "test-key", Model: "seed-audio-1.0", Unit: "sentence", Format: "mp3", SampleRate: 48000}
	if kind == "lesson" {
		script.Archetype = "lesson.explain@1"
		script.Title = "历史中的时间顺序"
		for i := range script.Scenes {
			scene := &script.Scenes[i]
			scene.Title = "梳理事件"
			scene.Intent = "理解先后顺序"
			for j := range scene.Narration {
				text := fmt.Sprintf("同学们，我们从第%d幕的第%d个问题开始，先观察事件，再梳理时间顺序。", i+1, j+1)
				scene.Narration[j].Display = text
				scene.Narration[j].Speech = text
			}
			scene.Objects = []AnimObject{{ID: "idea", Type: "card", Slot: "main", Text: "观察与比较", Items: []string{"先明确事件", "再梳理先后"}}}
			scene.Actions = []AnimAction{{ID: "reveal", Op: "show", Object: "idea", At: AnimTrigger{Sentence: scene.Narration[0].ID}}}
		}
	}
	token := worksLogin(t, s, "teacher")
	// Each workflow has its own public simulated purchase; combined runs must
	// not consume a preceding workflow's seed balance.
	order := worksObject(t, worksCall(t, s, "POST", "/api/billing/orders", token, map[string]any{"sku": "basic"}, 201))
	worksCall(t, s, "POST", "/api/billing/orders/"+order["id"].(string)+"/pay", token, map[string]any{}, 200)
	other := worksLogin(t, s, "other")
	admin := worksLogin(t, s, "admin")
	in := TaskInput{Type: "animation", Topic: "一次函数改变k", Requirements: "保持b不变", IdempotencyKey: newID()}
	if kind == "geometry" {
		in.Topic = "矩形平移与直角三角形旋转"
		in.Requirements = "观察平移和旋转前后面积不变"
	}
	if kind == "lesson" {
		in.Subject = "历史"
		in.Topic = "梳理历史事件的先后顺序"
		in.Requirements = "用时间线讲解，不编造史料"
	}
	worksCall(t, s, "POST", "/api/tasks", "", in, 401)
	before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	if again := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"]; again != id {
		t.Fatal("creation not idempotent")
	}
	stored := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
	if stored["input"].(map[string]any)["animation_engine"] != "html" {
		t.Fatal("missing default engine")
	}
	path := "/api/tasks/" + id + "/animation"
	for _, foreign := range []string{admin, other} {
		worksCall(t, s, "GET", path, foreign, nil, 404)
		worksCall(t, s, "POST", path+"/confirm-plan", foreign, map[string]any{}, 404)
		worksCall(t, s, "GET", path+"/file", foreign, nil, 404)
	}
	billing := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	if billing["reserved"] != before["reserved"] {
		t.Fatal("planning reserved points")
	}
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	state := worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
	if state["status"] != "anim_plan" || llmCalls != 1 || ttsCalls != 0 {
		t.Fatalf("plan stage %v calls %d/%d", state["status"], llmCalls, ttsCalls)
	}
	worksCall(t, s, "POST", path+"/accept", token, map[string]any{"revision": state["revision"]}, 409)
	badPlan := map[string]any{}
	_ = json.Unmarshal(asJSON(script), &badPlan)
	badPlan["html"] = "<script>bad</script>"
	worksCall(t, s, "POST", path+"/revise-plan", token, map[string]any{"revision": state["revision"], "request_key": newID(), "script": badPlan}, 400)
	worksCall(t, s, "POST", path+"/revise-plan", token, map[string]any{"revision": state["revision"], "request_key": newID(), "script": script}, 202)
	state = worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
	body := map[string]any{"revision": state["revision"]}
	worksCall(t, s, "POST", path+"/confirm-plan", token, body, 202)
	worksCall(t, s, "POST", path+"/confirm-plan", token, body, 202)
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	state = worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
	if state["status"] != "anim_representative" || ttsCalls != 3 {
		t.Fatalf("representative stage %v tts %d", state["status"], ttsCalls)
	}
	preview := worksCall(t, s, "GET", path+"/file", token, nil, 200).Body.String()
	if !strings.Contains(preview, "data:audio/mpeg") || strings.Contains(preview, "test-key") {
		t.Fatal("preview missing or leaked key")
	}
	body = map[string]any{"revision": state["revision"], "hash": state["representative_hash"]}
	worksCall(t, s, "POST", path+"/confirm-representative", token, body, 202)
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	state = worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
	if state["status"] != "anim_acceptance" || ttsCalls != 10 {
		t.Fatalf("full stage %v tts %d", state["status"], ttsCalls)
	}
	if kind == "lesson" {
		for _, foreign := range []string{admin, other} {
			worksCall(t, s, "POST", path+"/export", foreign, map[string]any{"revision": state["revision"]}, 404)
		}
		worksCall(t, s, "POST", path+"/export", "", map[string]any{}, 401)
		worksCall(t, s, "POST", path+"/export", token, map[string]any{"revision": state["revision"]}, 409)
	}
	// Changing an unrelated scene preserves the representative approval, but invalidates full acceptance.
	oldFull := map[string]any{"revision": state["revision"], "hash": state["build_hash"]}
	// An unavailable/corrupted package cannot be charged.
	ainfo := worksObject(t, worksCall(t, s, "GET", "/api/auth/me", token, nil, 200))
	tx, err := s.tx(context.Background(), int64(ainfo["tenant_id"].(float64)))
	if err != nil {
		t.Fatal(err)
	}
	owned, err := animRead(context.Background(), tx, id, int64(ainfo["member_id"].(float64)), false)
	tx.Rollback(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	packageBytes, err := os.ReadFile(owned.Flow.PreviewPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(owned.Flow.PreviewPath, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	worksCall(t, s, "POST", path+"/accept", token, oldFull, 409)
	if err = os.WriteFile(owned.Flow.PreviewPath, packageBytes, 0600); err != nil {
		t.Fatal(err)
	}
	repHash := state["representative_hash"]
	script.Scenes[3].Title = "小结与思考"
	worksCall(t, s, "POST", path+"/revise-plan", token, map[string]any{"revision": state["revision"], "request_key": newID(), "script": script}, 202)
	worksCall(t, s, "POST", path+"/accept", token, oldFull, 409)
	state = worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
	worksCall(t, s, "POST", path+"/confirm-plan", token, map[string]any{"revision": state["revision"]}, 202)
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	state = worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
	if state["status"] != "anim_acceptance" || state["representative_hash"] != repHash || ttsCalls != 10 {
		t.Fatal("unrelated plan change redoes representative/audio", state["status"])
	}
	body = map[string]any{"revision": state["revision"], "hash": state["build_hash"]}
	var acceptWG sync.WaitGroup
	codes := make(chan int, 2)
	for i := 0; i < 2; i++ {
		acceptWG.Add(1)
		go func() {
			defer acceptWG.Done()
			r := httptest.NewRequest("POST", path+"/accept", bytes.NewReader(asJSON(body)))
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			codes <- w.Code
		}()
	}
	acceptWG.Wait()
	close(codes)
	for code := range codes {
		if code != 200 {
			t.Fatalf("concurrent accept returned %d", code)
		}
	}
	billing = worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	if billing["balance"].(float64) != before["balance"].(float64)-40 || billing["reserved"] != before["reserved"] {
		t.Fatal("wrong settlement", billing)
	}
	delivered := worksCall(t, s, "GET", "/api/tasks/"+id+"/download/html", token, nil, 200).Body.String()
	if kind == "geometry" && !strings.Contains(delivered, "geo.transform@1") {
		t.Fatal("geometry task produced a different archetype")
	}
	if !strings.Contains(worksCall(t, s, "GET", "/api/tasks?view=works&type=animation", token, nil, 200).Body.String(), id) {
		t.Fatal("delivered animation missing from my works")
	}
	state = worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
	if dir := os.Getenv("ANIMATION_TEST_OUTPUT"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		b := worksCall(t, s, "GET", "/api/tasks/"+id+"/download/html", token, nil, 200).Body.Bytes()
		if err := os.WriteFile(filepath.Join(dir, "http-fixture.html"), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if kind == "lesson" {
		worksCall(t, s, "POST", path+"/export", token, map[string]any{"revision": state["accepted_revision"]}, 202)
		worksCall(t, s, "POST", path+"/export", token, map[string]any{"revision": state["accepted_revision"]}, 202)
		exported := worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))["export"].(map[string]any)
		if exported["status"] != "pending" {
			t.Fatal("MP4 was not queued against accepted revision", exported)
		}
		worksCall(t, s, "POST", path+"/cancel-export", token, map[string]any{"revision": state["accepted_revision"]}, 200)
		after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
		if after["balance"] != billing["balance"] || after["reserved"] != billing["reserved"] {
			t.Fatal("export charged again")
		}
	}
	// Local editing preserves other scene data and only consumes the allowance on acceptance.
	edit := map[string]any{"revision": state["revision"], "scene_id": "summary", "instruction": "把小结标题改为总结", "request_key": newID()}
	script.Scenes[3].Title = "总结"
	originalHTML := worksCall(t, s, "GET", "/api/tasks/"+id+"/download/html", token, nil, 200).Body.String()
	worksCall(t, s, "POST", path+"/edit", token, edit, 202)
	worksCall(t, s, "POST", path+"/edit", token, edit, 202)
	concurrent := map[string]any{"revision": state["revision"], "scene_id": "summary", "instruction": "另一次修改", "request_key": newID()}
	worksCall(t, s, "POST", path+"/edit", token, concurrent, 409)
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	state = worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
	if got := worksCall(t, s, "GET", "/api/tasks/"+id+"/download/html", token, nil, 200).Body.String(); got != originalHTML {
		t.Fatal("unaccepted edit replaced delivery")
	}
	if state["status"] != "anim_edit_acceptance" || ttsCalls != 10 {
		t.Fatal("edit should reuse unchanged audio")
	}
	worksCall(t, s, "GET", "/api/tasks/"+id+"/download/html", token, nil, 200)
	worksCall(t, s, "POST", path+"/accept", token, map[string]any{"revision": state["revision"], "hash": "stale"}, 409)
	body = map[string]any{"revision": state["revision"], "hash": state["build_hash"]}
	worksCall(t, s, "POST", path+"/accept", token, body, 200)
	state = worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
	edit["revision"] = state["revision"]
	if kind == "lesson" && state["export"].(map[string]any)["status"] != "" {
		t.Fatal("new accepted revision retained old video status")
	}
	edit["request_key"] = newID()
	worksCall(t, s, "POST", path+"/edit", token, edit, 409)
	acceptedHTML := worksCall(t, s, "GET", "/api/tasks/"+id+"/download/html", token, nil, 200).Body.String()
	script.Version = "invalid"
	worksCall(t, s, "POST", path+"/edit", token, map[string]any{"revision": state["revision"], "scene_id": "intro", "instruction": "修改标题", "request_key": newID()}, 202)
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	state = worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
	if state["status"] != "failed" || state["charge_state"] != "settled" || state["free_used"].(map[string]any)["intro"] == true {
		t.Fatal("failed edit changed settled charge or allowance")
	}
	if got := worksCall(t, s, "GET", "/api/tasks/"+id+"/download/html", token, nil, 200).Body.String(); got != acceptedHTML {
		t.Fatal("failed edit lost delivery")
	}
	worksCall(t, s, "POST", path+"/discard-edit", token, map[string]any{"revision": state["revision"]}, 202)
	worksCall(t, s, "DELETE", "/api/tasks/"+id, token, nil, 200)
	worksCall(t, s, "GET", path+"/file", token, nil, 404)
}

func TestHTMLAnimationRecoveryHTTP(t *testing.T) {
	s := worksServer(t)
	s.cfg.Mode = "live"
	script := loadLinearKFixture(t)
	var calls atomic.Int32
	var broken atomic.Bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tts" {
			n := calls.Add(1)
			if broken.Load() && n == 2 {
				w.WriteHeader(503)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"audio": base64.StdEncoding.EncodeToString(testMP3(250)), "subtitle": map[string]any{"sentences": []any{map[string]any{"start_time": 240, "end_time": 5600}}}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(asJSON(script))}}}})
	}))
	defer provider.Close()
	s.cfg.AnimationLLMURL, s.cfg.AnimationLLMKey, s.cfg.AnimationLLMModel = provider.URL, "test-key", "deepseek-flash"
	s.cfg.TTS = ttsConfig{Provider: "volcengine", Protocol: "create", URL: provider.URL + "/tts", Key: "test-key", Model: "seed-audio-1.0", Unit: "sentence", Format: "mp3", SampleRate: 48000}
	token := worksLogin(t, s, "teacher")
	actor := worksObject(t, worksCall(t, s, "GET", "/api/auth/me", token, nil, 200))
	tid := int64(actor["tenant_id"].(float64))
	mid := int64(actor["member_id"].(float64))
	ctx := context.Background()
	setBalance := func(n int64) {
		tx, e := s.tx(ctx, tid)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		if _, e = tx.Exec(ctx, "UPDATE members SET balance=$1 WHERE id=$2", n, mid); e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
	}
	// Each workflow settles credits; this recovery fixture owns its starting balance.
	setBalance(100)
	original := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))["balance"].(float64)
	setBalance(0)
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, TaskInput{Type: "animation", AnimationEngine: "html", Topic: "一次函数", IdempotencyKey: newID()}, 202))["id"].(string)
	path := "/api/tasks/" + id + "/animation"
	work := func() {
		t.Helper()
		c, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		if e := s.workOne(c); e != nil {
			t.Fatal(e)
		}
	}
	state := func() map[string]any { return worksObject(t, worksCall(t, s, "GET", path, token, nil, 200)) }
	work()
	v := state()
	worksCall(t, s, "POST", path+"/confirm-plan", token, map[string]any{"revision": v["revision"]}, 402)
	setBalance(int64(original))
	broken.Store(true)
	worksCall(t, s, "POST", path+"/confirm-plan", token, map[string]any{"revision": v["revision"]}, 202)
	work()
	v = state()
	if v["status"] != "failed" || v["charge_state"] != "released" || calls.Load() != 2 {
		t.Fatal("failed phase did not release", v)
	}
	used := v["calls"].(map[string]any)["tts"]
	if used != float64(2) {
		t.Fatal("call budget did not persist")
	}
	broken.Store(false)
	worksCall(t, s, "POST", path+"/resume", token, map[string]any{"revision": v["revision"]}, 202)
	work()
	v = state()
	if v["status"] != "anim_representative" || calls.Load() != 4 {
		t.Fatalf("checkpoint not reused: %v/%d", v["status"], calls.Load())
	}
	worksCall(t, s, "POST", path+"/confirm-representative", token, map[string]any{"revision": v["revision"], "hash": "old-hash"}, 409)
	// Cancelling an awaiting stage preserves its exact build and each released ledger cycle.
	worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
	v = state()
	worksCall(t, s, "POST", path+"/resume", token, map[string]any{"revision": v["revision"]}, 202)
	v = state()
	worksCall(t, s, "POST", path+"/confirm-representative", token, map[string]any{"revision": v["revision"], "hash": v["representative_hash"]}, 202)
	work()
	v = state()
	if calls.Load() != 11 {
		t.Fatal("restored workflow repeated audio", calls.Load())
	}
	worksCall(t, s, "POST", path+"/accept", token, map[string]any{"revision": v["revision"], "hash": v["build_hash"]}, 200)
	account := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	if account["balance"] != original-40 || account["reserved"] != float64(0) {
		t.Fatal("recovery accounting wrong", account)
	}
	tx, e := s.tx(ctx, tid)
	if e != nil {
		t.Fatal(e)
	}
	var reserve, release, settle int
	e = tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE kind='reserve'),count(*) FILTER(WHERE kind='release'),count(*) FILTER(WHERE kind='settle') FROM ledger WHERE member_id=$1 AND (reference=$2 OR reference LIKE $3)`, mid, id, id+":html:%").Scan(&reserve, &release, &settle)
	tx.Rollback(ctx)
	if e != nil || reserve != 3 || release != 2 || settle != 1 {
		t.Fatalf("ledger history changed %d/%d/%d %v", reserve, release, settle, e)
	}
	// An unreserved plan can be cancelled, resumed, and expires without touching balances.
	id2 := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, TaskInput{Type: "animation", AnimationEngine: "html", Topic: "一次函数取消测试", IdempotencyKey: newID()}, 202))["id"].(string)
	p2 := "/api/tasks/" + id2 + "/animation"
	worksCall(t, s, "POST", "/api/tasks/"+id2+"/cancel", token, nil, 200)
	v2 := worksObject(t, worksCall(t, s, "GET", p2, token, nil, 200))
	worksCall(t, s, "POST", p2+"/resume", token, map[string]any{"revision": v2["revision"]}, 202)
	work()
	tx, e = s.tx(ctx, tid)
	if e != nil {
		t.Fatal(e)
	}
	_, e = tx.Exec(ctx, "UPDATE tasks SET updated_at=now()-interval '25 hours' WHERE id=$1", id2)
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	work()
	v2 = worksObject(t, worksCall(t, s, "GET", p2, token, nil, 200))
	if v2["status"] != "failed" || v2["charge_state"] != "unreserved" {
		t.Fatal("plan expiry mishandled", v2)
	}
	// Refill this isolated test account after the earlier settlement.
	setBalance(100)
	// A restarted request retains its TTS cap and cannot spend another provider call.
	v2 = worksObject(t, worksCall(t, s, "GET", p2, token, nil, 200))
	worksCall(t, s, "POST", p2+"/resume", token, map[string]any{"revision": v2["revision"]}, 202)
	v2 = worksObject(t, worksCall(t, s, "GET", p2, token, nil, 200))
	tx, e = s.tx(ctx, tid)
	if e != nil {
		t.Fatal(e)
	}
	_, e = tx.Exec(ctx, `UPDATE tasks SET animation=jsonb_set(animation,'{calls,tts}',to_jsonb($1::int),true) WHERE id=$2`, animCallLimits()["tts"], id2)
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	worksCall(t, s, "POST", p2+"/confirm-plan", token, map[string]any{"revision": v2["revision"]}, 202)
	beforeCalls := calls.Load()
	work()
	v2 = worksObject(t, worksCall(t, s, "GET", p2, token, nil, 200))
	if v2["status"] != "failed" || calls.Load() != beforeCalls {
		t.Fatal("provider cap bypassed")
	}
	worksCall(t, s, "DELETE", "/api/tasks/"+id2, token, nil, 200)
}

func TestAnimationSpeechBoundaries(t *testing.T) {
	start, end, e := animSpeechBounds(ttsResult{DurationMS: 6240, Sentences: []ttsSentence{{StartTime: 240, EndTime: 5880}}})
	if e != nil || start != 240 || end != 5880 {
		t.Fatal("wrong speech bounds", start, end, e)
	}
	if _, _, e = animSpeechBounds(ttsResult{DurationMS: 6240}); e == nil {
		t.Fatal("missing timestamps accepted")
	}
}

func TestHTMLAnimationCancelledWorkerHTTP(t *testing.T) {
	s := worksServer(t)
	s.cfg.Mode = "live"
	script := loadLinearKFixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	var count atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if count.Add(1) == 1 {
			close(started)
			<-release
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(asJSON(script))}}}})
	}))
	defer provider.Close()
	s.cfg.AnimationLLMURL, s.cfg.AnimationLLMKey, s.cfg.AnimationLLMModel = provider.URL, "test-key", "deepseek-flash"
	s.cfg.TTS = ttsConfig{Provider: "volcengine", Protocol: "create", URL: provider.URL, Key: "test-key", Model: "seed-audio-1.0", Unit: "sentence", Format: "mp3"}
	token := worksLogin(t, s, "teacher")
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, TaskInput{Type: "animation", AnimationEngine: "html", Topic: "一次函数取消中的请求", IdempotencyKey: newID()}, 202))["id"].(string)
	path := "/api/tasks/" + id + "/animation"
	done := make(chan error, 1)
	go func() {
		ctx, c := context.WithTimeout(context.Background(), 45*time.Second)
		defer c()
		done <- s.workOne(ctx)
	}()
	select {
	case <-started:
	case <-time.After(20 * time.Second):
		close(release)
		t.Fatal("provider not reached")
	}
	worksCall(t, s, "POST", path+"/cancel", token, map[string]any{}, 200)
	close(release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	v := worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
	if v["status"] != "cancelled" {
		t.Fatal("stale worker revived cancelled task")
	}
	worksCall(t, s, "POST", path+"/resume", token, map[string]any{"revision": v["revision"]}, 202)
	if e := s.workOne(context.Background()); e != nil {
		t.Fatal(e)
	}
	v = worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
	if v["status"] != "anim_plan" {
		t.Fatal(fmt.Sprint("completed cancelled worker left stale lease: ", v["status"]))
	}
	worksCall(t, s, "POST", path+"/cancel", token, map[string]any{}, 200)
}

// Explicit opt-in: real provider calls, isolated database and synthetic teacher approvals.
// This proves integration only; it does not certify classroom quality or human acceptance.
func TestHTMLAnimationLiveProbe(t *testing.T) { testHTMLAnimationLiveProbe(t, false) }
func TestHTMLGeometryLiveProbe(t *testing.T)  { testHTMLAnimationLiveProbe(t, true) }
func testHTMLAnimationLiveProbe(t *testing.T, geometry bool) {
	if os.Getenv("RUN_ANIMATION_LIVE_PROBE") != "1" {
		t.Skip("explicit real-provider opt-in required")
	}
	s := worksServer(t)
	s.cfg.Mode = "live"
	s.cfg.AnimationLLMURL = os.Getenv("ANIMATION_LLM_BASE_URL")
	s.cfg.AnimationLLMKey = os.Getenv("ANIMATION_LLM_API_KEY")
	s.cfg.AnimationLLMModel = os.Getenv("ANIMATION_LLM_MODEL")
	s.cfg.TTS = ttsConfigFromEnv()
	if !s.htmlAnimationConfigured() {
		t.Fatal("live configuration missing")
	}
	token := worksLogin(t, s, "teacher")
	start := time.Now()
	in := TaskInput{Type: "animation", AnimationEngine: "html", Topic: "一次函数中k的符号与绝对值", Requirements: "四幕，60至90秒，保持b等于二不变，正区间和负区间分别演示。代表幕展示正斜率从一变三，有观察停顿和等待点击思考。每幕两至三句旁白。", IdempotencyKey: newID()}
	artifact := "live-linear-k.html"
	if geometry {
		in.Topic = "矩形平移与直角三角形旋转"
		in.Requirements = "四幕，60至90秒。三乘二矩形向右平移三个单位；三乘二直角三角形绕原点逆时针旋转九十度；观察边长和面积不变。代表幕选择矩形平移。保留观察和等待点击思考。所有幕坐标范围横轴负八至八、纵轴负六至六，每幕两到三句旁白。"
		artifact = "live-geometry.html"
	}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	path := "/api/tasks/" + id + "/animation"
	phase := func(want string) map[string]any {
		t.Helper()
		ctx, c := context.WithTimeout(context.Background(), 10*time.Minute)
		defer c()
		if err := s.workOne(ctx); err != nil {
			t.Fatal("live worker failed")
		}
		v := worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
		if v["status"] != want {
			t.Fatalf("live phase: got %v want %s; calls=%v problems=%v", v["status"], want, v["calls"], v["problems"])
		}
		t.Logf("phase=%s elapsed=%s calls=%v", want, time.Since(start).Round(time.Second), v["calls"])
		return v
	}
	v := phase("anim_plan")
	worksCall(t, s, "POST", path+"/confirm-plan", token, map[string]any{"revision": v["revision"]}, 202)
	v = phase("anim_representative")
	worksCall(t, s, "POST", path+"/confirm-representative", token, map[string]any{"revision": v["revision"], "hash": v["representative_hash"]}, 202)
	v = phase("anim_acceptance")
	worksCall(t, s, "POST", path+"/accept", token, map[string]any{"revision": v["revision"], "hash": v["build_hash"]}, 200)
	dir := os.Getenv("ANIMATION_TEST_OUTPUT")
	if dir == "" {
		t.Fatal("set ANIMATION_TEST_OUTPUT for real artifacts")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	b := worksCall(t, s, "GET", "/api/tasks/"+id+"/download/html", token, nil, 200).Body.Bytes()
	if err := os.WriteFile(filepath.Join(dir, artifact), b, 0600); err != nil {
		t.Fatal(err)
	}
	v["verification"] = "real provider integration; synthetic teacher approvals, manual audio/content review pending"
	v["elapsed_seconds"] = time.Since(start).Seconds()
	v["model"] = s.cfg.AnimationLLMModel
	v["tts_protocol"] = s.cfg.TTS.protocol()
	if err := os.WriteFile(filepath.Join(dir, "live-report.json"), asJSON(v), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestAnimationLocalHashIgnoresUnrelatedScenes(t *testing.T) {
	s := &Server{cfg: Config{TTS: ttsConfig{Protocol: "create", Model: "seed-audio-1.0"}}}
	sc := loadLinearKFixture(t)
	f := animWorkflow{Script: sc, VoiceConfig: s.animVoiceFingerprint(), Audio: map[string]animAudio{}}
	for _, scene := range sc.Scenes {
		for _, n := range scene.Narration {
			f.Audio[animDigest([]string{f.VoiceConfig, n.Speech})] = animAudio{Path: "private-path", Hash: animDigest(n.Speech), MS: 6000, Start: 240, End: 5600}
		}
	}
	f.RepHash = s.animRepresentativeHash(f)
	f.RepConfirmed = f.RepHash
	original := f.RepHash
	s.cfg.TTS.Delivery = "classroom-v1"
	if s.animRepresentativeHash(f) != original {
		t.Fatal("new classroom delivery invalidated existing confirmed audio")
	}
	f.Script.Scenes[3].Title = "总结"
	s.animInvalidateBuild(&f)
	if f.RepConfirmed != original {
		t.Fatal("unrelated scene invalidated representative approval")
	}
	f.Script.Scenes[1].Title = "改变代表幕标题"
	s.animInvalidateBuild(&f)
	if f.RepConfirmed != "" || f.RepHash == original {
		t.Fatal("changed local build retained approval")
	}
}
