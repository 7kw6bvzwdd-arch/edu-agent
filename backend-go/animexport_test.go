package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAnimationExportRecoveryHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	id := worksFixture(t, s, token, "animation", "succeeded", "导出恢复边界", TaskInput{AnimationEngine: "html"})
	source := filepath.Join(s.cfg.Storage, id+".html")
	b := []byte("explicit export transport fixture")
	if e := os.WriteFile(source, b, 0600); e != nil {
		t.Fatal(e)
	}
	seed := func(job animExport, expired bool) {
		t.Helper()
		tx, e := s.tx(context.Background(), a.TenantID)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(context.Background())
		_, e = tx.Exec(context.Background(), `UPDATE tasks SET html_path=$1,animation=$2,revision=1,animation_export=$3,animation_export_lease_until=CASE WHEN $4 THEN now()-interval '1 minute' ELSE NULL END WHERE id=$5`, source, asJSON(animWorkflow{AcceptedHash: animDigest(b), AcceptedRevision: 1}), asJSON(job), expired, id)
		if e != nil {
			t.Fatal(e)
		}
		_, e = tx.Exec(context.Background(), `INSERT INTO artifact_revisions(tenant_id,task_id,revision,result,pptx_path,mp4_path,html_path) VALUES($1,$2,1,'{}','','',$3) ON CONFLICT(tenant_id,task_id,revision) DO NOTHING`, a.TenantID, id, source)
		if e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(context.Background()); e != nil {
			t.Fatal(e)
		}
	}
	seed(animExport{}, false)
	path := "/api/tasks/" + id + "/animation"
	body := map[string]any{"revision": 1}
	state := func() map[string]any {
		return worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))["export"].(map[string]any)
	}
	before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	script := filepath.Join(t.TempDir(), "renderer.mjs")
	t.Setenv("ANIMATION_EXPORT_SCRIPT", script)
	worksCall(t, s, "POST", path+"/export", token, body, 503)
	if e := os.WriteFile(script, []byte("throw Error('explicit failed renderer fixture')"), 0600); e != nil {
		t.Fatal(e)
	}
	for attempt := 1; attempt <= 3; attempt++ {
		worksCall(t, s, "POST", path+"/export", token, body, 202)
		if worked, e := s.workAnimationExport(context.Background(), a.TenantID); e != nil || !worked {
			t.Fatal(worked, e)
		}
		job := state()
		if job["status"] != "failed" || job["attempts"] != float64(attempt) {
			t.Fatal(job)
		}
	}
	worksCall(t, s, "POST", path+"/export", token, body, 409)
	worksCall(t, s, "GET", "/api/tasks/"+id+"/download/html", token, nil, 200)
	// Infrastructure interruption restores this attempt and automatically queues
	// the accepted revision. Ordinary renderer failures still consume the budget.
	seed(animExport{Status: "running", Revision: 1, Hash: animDigest(b), Attempt: 2, Token: newID()}, true)
	if e := s.recoverExpiredTasks(context.Background()); e != nil {
		t.Fatal(e)
	}
	if job := state(); job["status"] != "pending" || job["attempts"] != float64(1) {
		t.Fatal(job)
	}
	if _, e := s.workAnimationExport(context.Background(), a.TenantID); e != nil {
		t.Fatal(e)
	}
	if job := state(); job["status"] != "failed" || job["attempts"] != float64(2) {
		t.Fatal(job)
	}
	// Running cancellation must release its process before a replacement can queue.
	if e := os.WriteFile(script, []byte("setTimeout(()=>{},30000)"), 0600); e != nil {
		t.Fatal(e)
	}
	worksCall(t, s, "POST", path+"/export", token, body, 202)
	done := make(chan error, 1)
	go func() { _, e := s.workAnimationExport(context.Background(), a.TenantID); done <- e }()
	deadline := time.Now().Add(10 * time.Second)
	for state()["status"] != "running" {
		if time.Now().After(deadline) {
			t.Fatal("export never started")
		}
		time.Sleep(50 * time.Millisecond)
	}
	worksCall(t, s, "POST", path+"/cancel-export", token, body, 200)
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("cancelled renderer did not exit")
	}
	if job := state(); job["status"] != "cancelled" || job["attempts"] != float64(3) {
		t.Fatal(job)
	}
	worksCall(t, s, "GET", "/api/tasks/"+id+"/download/mp4", token, nil, 404)
	worksCall(t, s, "POST", path+"/export", token, body, 409)
	after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	if after["balance"] != before["balance"] || after["reserved"] != before["reserved"] {
		t.Fatal("export recovery charged credits")
	}
	worksCall(t, s, "DELETE", "/api/tasks/"+id, token, nil, 200)
}

// The prepared package is explicitly a fixture; the exporter must produce real MP4 bytes.
func TestAnimationExportActualMP4HTTP(t *testing.T) {
	source := os.Getenv("ANIMATION_EXPORT_TEST_SOURCE")
	if source == "" {
		t.Skip("requires prepared package and installed video renderer")
	}
	b, e := os.ReadFile(source)
	if e != nil {
		t.Fatal(e)
	}
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "other")
	admin := worksLogin(t, s, "admin")
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	id := worksFixture(t, s, token, "animation", "succeeded", "通用讲解导出验收", TaskInput{AnimationEngine: "html"})
	html := filepath.Join(s.cfg.Storage, id+".html")
	if e = os.WriteFile(html, b, 0600); e != nil {
		t.Fatal(e)
	}
	flow := animWorkflow{AcceptedHash: animDigest(b), AcceptedRevision: 1}
	tx, e := s.tx(context.Background(), a.TenantID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = tx.Exec(context.Background(), "UPDATE tasks SET html_path=$1,animation=$2,revision=1 WHERE id=$3", html, asJSON(flow), id)
	if e == nil {
		_, e = tx.Exec(context.Background(), "INSERT INTO artifact_revisions(tenant_id,task_id,revision,result,pptx_path,mp4_path,html_path) VALUES($1,$2,1,'{}','','',$3)", a.TenantID, id, html)
	}
	if e != nil {
		tx.Rollback(context.Background())
		t.Fatal(e)
	}
	if e = tx.Commit(context.Background()); e != nil {
		t.Fatal(e)
	}
	path := "/api/tasks/" + id + "/animation"
	download := "/api/tasks/" + id + "/download/mp4"
	before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	worksCall(t, s, "POST", path+"/export", token, map[string]any{"revision": 1}, 202)
	worked, e := s.workAnimationExport(context.Background(), a.TenantID)
	if e != nil || !worked {
		t.Fatal(worked, e)
	}
	state := worksObject(t, worksCall(t, s, "GET", path, token, nil, 200))
	if state["export"].(map[string]any)["status"] != "succeeded" {
		t.Fatal(state["export"])
	}
	for _, foreign := range []string{other, admin} {
		worksCall(t, s, "GET", download, foreign, nil, 404)
		worksCall(t, s, "GET", "/api/tasks/"+id+"/revisions/1/download/mp4", foreign, nil, 404)
	}
	worksCall(t, s, "GET", download, "", nil, 401)
	video := worksCall(t, s, "GET", download, token, nil, 200)
	if len(video.Body.Bytes()) < 1000 || string(video.Body.Bytes()[4:8]) != "ftyp" {
		t.Fatal("not an actual MP4")
	}
	worksCall(t, s, "GET", "/api/tasks/"+id+"/revisions/1/download/mp4", token, nil, 200)
	worksCall(t, s, "POST", path+"/export", token, map[string]any{"revision": 1}, 202)
	after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	if after["balance"] != before["balance"] || after["reserved"] != before["reserved"] {
		t.Fatal("export charged again")
	}
	worksCall(t, s, "DELETE", "/api/tasks/"+id, token, nil, 200)
	worksCall(t, s, "GET", download, token, nil, 404)
}
