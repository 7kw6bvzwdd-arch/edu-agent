package main

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRemediationTeacherWaitPersistsHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))["reserved"]
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, TaskInput{Type: "courseware", Topic: "隔天审核", PreviewFirst: true, UseKB: true, IdempotencyKey: newID()}, 202))["id"].(string)
	tx, txErr := s.tx(context.Background(), 1)
	if txErr != nil {
		t.Fatal(txErr)
	}
	if _, err := tx.Exec(context.Background(), "UPDATE tasks SET status='awaiting_confirmation',stage='awaiting_confirmation',updated_at=now()-interval '2 days' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
	if got["status"] != "awaiting_confirmation" {
		t.Fatalf("teacher review must survive two days: %v", got["status"])
	}
	worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
	if got := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))["reserved"]; got != before {
		t.Fatal("cancel must release exactly one hold")
	}
}

func TestRemediationExpiredLeaseHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, TaskInput{Type: "courseware", Topic: "重启恢复", PreviewFirst: true, UseKB: true, IdempotencyKey: newID()}, 202))["id"].(string)
	tx, txErr := s.tx(context.Background(), 1)
	if txErr != nil {
		t.Fatal(txErr)
	}
	if _, err := tx.Exec(context.Background(), "UPDATE tasks SET status='researching',attempt=7,lease_until=now()-interval '1 second' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.recoverExpiredTasks(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
	if got["status"] != "pending" || got["charge_state"] != "reserved" {
		t.Fatalf("lease recovery: %v", got)
	}
	old := context.WithValue(context.Background(), generationAttemptKey{}, 7)
	if err := s.finish(old, 1, id, "failed", "old worker", "", nil); err != nil {
		t.Fatal(err)
	}
	got = worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
	if got["charge_state"] != "reserved" {
		t.Fatal("stale worker released reservation")
	}
	worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
	if err := s.requeueInterrupted(1, id, 8); err != nil {
		t.Fatal(err)
	}
	got = worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
	if got["status"] != "cancelled" {
		t.Fatal("restart resurrected user cancellation")
	}
}
func TestRemediationLoginRateLimitHTTP(t *testing.T) {
	s := worksServer(t)
	for i := 0; i < 12; i++ {
		worksCall(t, s, "POST", "/api/auth/login", "", map[string]string{"username": "missing-account", "password": "incorrect"}, 401)
	}
	worksCall(t, s, "POST", "/api/auth/login", "", map[string]string{"username": "missing-account", "password": "incorrect"}, 429)
}
func TestRemediationLiveSimulationHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	id := worksObject(t, worksCall(t, s, "POST", "/api/billing/orders", token, map[string]string{"sku": "topup"}, 201))["id"].(string)
	s.cfg.Mode = "live"
	worksCall(t, s, "POST", "/api/billing/orders/"+id+"/pay", token, nil, 403)
}

func TestRemediationAuthStoragePrivilegesHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	worksCall(t, s, "GET", "/api/auth/me", token, nil, 200)
	tx, err := s.tx(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var hash string
	if err = tx.QueryRow(context.Background(), "SELECT password_hash FROM users LIMIT 1").Scan(&hash); err == nil {
		t.Fatal("runtime can read raw credential hashes")
	}
}

func TestRemediationReviewPolicyHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	s.cfg.CoursewareReviewPolicy = "key"
	id := worksFixture(t, s, token, "courseware", "awaiting_preview", "课件核验状态", TaskInput{PreviewFirst: true, Detail: "auto", Subject: "历史"})
	d := sampleDeck(TaskInput{Topic: "浮力", Pages: 8, Theme: "ocean"})
	d.Slides = d.Slides[:3]
	d.Slides[0] = Slide{Title: "学习导入", Layout: "title"}
	d.Slides[1] = Slide{Title: "例题", Layout: "example", Answer: "正确答案"}
	assignSlideIDs(&d)
	d.Plan = &TeachingPlan{Summary: "review fixture"}
	d.PageChecks = map[string]PageQualityCheck{}
	for i, sl := range d.Slides {
		d.Plan.Pages = append(d.Plan.Pages, TeachingPage{Position: i + 1, Goal: sl.Title, Task: sl.Layout, Minutes: 3})
		d.PageChecks[sl.ID] = PageQualityCheck{Hash: slideContentHash(d, sl), Content: "skipped", Visual: "checked"}
	}
	tx, err := s.tx(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(context.Background(), "UPDATE tasks SET result=$1,mode='live' WHERE id=$2", asJSON(d), id); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.cfg.Mode = "live"
	v := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
	reviews := v["slide_reviews"].([]any)
	meta := reviews[0].(map[string]any)
	worksCall(t, s, "POST", "/api/tasks/"+id+"/slides/"+d.Slides[0].ID+"/review", token, map[string]any{"revision": v["revision"], "hash": meta["hash"], "approved": true}, 200)
	worksCall(t, s, "POST", "/api/tasks/"+id+"/slides/"+d.Slides[1].ID+"/review", token, map[string]any{"revision": v["revision"], "hash": reviews[1].(map[string]any)["hash"], "approved": true}, 409)
	s.cfg.CoursewareReviewPolicy = "all"
	worksCall(t, s, "POST", "/api/tasks/"+id+"/slides/"+d.Slides[1].ID+"/review", token, map[string]any{"revision": v["revision"], "hash": reviews[1].(map[string]any)["hash"], "approved": true}, 409)
}

func TestRemediationCredentialFunctionPrivilegesHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	worksCall(t, s, "GET", "/api/auth/me", token, nil, 200)
	tx, err := s.tx(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var exposed bool
	err = tx.QueryRow(context.Background(), "SELECT EXISTS(SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public' AND p.proname IN ('auth_credentials','auth_set_password','auth_session_create') AND has_function_privilege(current_user,p.oid,'EXECUTE'))").Scan(&exposed)
	if err != nil {
		t.Fatal(err)
	}
	if exposed {
		t.Fatal("runtime can export credential hashes or mutate credentials without proof")
	}
	if err = tx.QueryRow(context.Background(), "SELECT has_schema_privilege(current_user,'public','CREATE')").Scan(&exposed); err != nil {
		t.Fatal(err)
	}
	if exposed {
		t.Fatal("runtime can create objects in the credential function search path")
	}
}

func TestRemediationCredentialNullProofHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var accepted bool
	if err = tx.QueryRow(context.Background(), "SELECT auth_login('teacher',NULL,'null-proof',now()+interval '1 hour') IS NOT NULL").Scan(&accepted); err != nil {
		t.Fatal(err)
	}
	if accepted {
		t.Fatal("NULL password authenticated an account")
	}
	for _, query := range []string{"SELECT auth_change_password($1,NULL,'invalid')", "SELECT auth_issue_recovery($1,NULL,'invalid')"} {
		if err = tx.QueryRow(context.Background(), query, a.UserID).Scan(&accepted); err != nil {
			t.Fatal(err)
		}
		if accepted {
			t.Fatal("NULL proof mutated credentials")
		}
	}
	if err = tx.QueryRow(context.Background(), "SELECT auth_reset_password('teacher',NULL,'invalid','invalid')").Scan(&accepted); err != nil {
		t.Fatal(err)
	}
	if accepted {
		t.Fatal("NULL recovery proof mutated credentials")
	}
}
