package main

import (
	"context"
	"encoding/json"
	"testing"
)

func TestContinuationKeepsWorkIdentityAndAccounting(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	in := TaskInput{Type: "courseware", Topic: "same work", Pages: 8, Theme: "ocean", UseKB: true, PreviewFirst: true, IdempotencyKey: newID()}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	initial := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	before, reserved := initial["balance"], initial["reserved"]
	for range 3 {
		if err := s.finish(context.Background(), a.TenantID, id, "failed", "interrupted", "", nil); err != nil {
			t.Fatal(err)
		}
		worksCall(t, s, "POST", "/api/tasks/"+id+"/restore", token, nil, 200)
		v := worksObject(t, worksCall(t, s, "POST", "/api/tasks/"+id+"/retry", token, nil, 202))
		if v["id"] != id {
			t.Fatal("continuation created another work", v)
		}
		worksCall(t, s, "POST", "/api/tasks/"+id+"/retry", token, nil, 202)
		bill := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
		if bill["reserved"] != reserved || bill["balance"] != before {
			t.Fatal("duplicate reservation", bill)
		}
	}
	worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
	var rows []map[string]any
	json.Unmarshal(worksCall(t, s, "GET", "/api/tasks?view=interrupted", token, nil, 200).Body.Bytes(), &rows)
	found := 0
	for _, row := range rows {
		if row["id"] == id {
			found++
		}
	}
	if found != 1 {
		t.Fatal("failed restored work missing from interrupted", rows)
	}
}

func TestOldGenerationCycleCannotOverwriteContinuation(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	in := TaskInput{Type: "courseware", Topic: "cycle guard", Pages: 8, Theme: "ocean", UseKB: true, PreviewFirst: true, IdempotencyKey: newID()}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	if err := s.finish(context.Background(), a.TenantID, id, "failed", "paused", "", nil); err != nil {
		t.Fatal(err)
	}
	worksCall(t, s, "POST", "/api/tasks/"+id+"/retry", token, nil, 202)
	old := context.WithValue(context.Background(), generationCycleKey{}, 0)
	if err := s.finish(old, a.TenantID, id, "failed", "late failure", "", nil); err != nil {
		t.Fatal(err)
	}
	v := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
	if v["charge_state"] != "reserved" || v["status"] == "failed" {
		t.Fatal("old worker released new reservation", v)
	}
	if err := s.saveCoursewareCheckpoint(old, a, id, Deck{}, nil); err == nil {
		t.Fatal("old checkpoint accepted")
	}
	worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(context.Background(), "UPDATE tasks SET lease_until=now()+interval '1 minute' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.queueDeletedCleanup(a.TenantID, id, true, 0)
	tx, err = s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	var leased bool
	err = tx.QueryRow(context.Background(), "SELECT lease_until>now() FROM tasks WHERE id=$1", id).Scan(&leased)
	tx.Rollback(context.Background())
	if err != nil || !leased {
		t.Fatal("old cleanup released current worker lease", err)
	}
	s.queueDeletedCleanup(a.TenantID, id, true, 1)
	tx, err = s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	err = tx.QueryRow(context.Background(), "SELECT lease_until IS NOT NULL FROM tasks WHERE id=$1", id).Scan(&leased)
	tx.Rollback(context.Background())
	if err != nil || leased {
		t.Fatal("current worker failed to release lease", err)
	}
}

func TestPlannedCoursewareCannotSettleUnchecked(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	in := TaskInput{Type: "courseware", Topic: "quality settlement guard", Pages: 8, Theme: "ocean", UseKB: true, PreviewFirst: true, IdempotencyKey: newID()}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	d := sampleDeck(in)
	d.Plan = &TeachingPlan{}
	for i := range d.Slides {
		d.Plan.Pages = append(d.Plan.Pages, TeachingPage{Position: i + 1, Goal: "质量检查", Minutes: 1})
	}
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(context.Background(), "UPDATE tasks SET result=$1 WHERE id=$2", asJSON(d), id); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.finish(context.Background(), a.TenantID, id, "succeeded", "unchecked", "", d); err == nil {
		t.Fatal("unchecked planned deck was settled")
	}
	v := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
	if v["charge_state"] != "reserved" {
		t.Fatal("failed quality guard changed accounting", v)
	}
	worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
}

func TestUsagePagesDeletionAndTotalsHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "other")
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for i := 0; i < 3; i++ {
		var id int64
		err = tx.QueryRow(context.Background(), "INSERT INTO usage_logs(tenant_id,member_id,operation,model,input_tokens,output_tokens,latency_ms,success,created_at) VALUES($1,$2,'test','fixture',10,5,100,true,'2026-10-10T04:00:00Z'::timestamptz+$3*interval '1 minute') RETURNING id", a.TenantID, a.MemberID, i).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	route := "/api/usage?from=2026-10-10T04:00:00Z&to=2026-10-10T04:03:00Z&limit=2"
	v := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if v["total"] != float64(3) || len(v["items"].([]any)) != 2 {
		t.Fatal("pagination failed", v)
	}
	next := worksObject(t, worksCall(t, s, "GET", route+"&offset=2", token, nil, 200))
	if len(next["items"].([]any)) != 1 {
		t.Fatal("next page failed", next)
	}
	worksCall(t, s, "POST", "/api/usage/delete", other, map[string]any{"ids": ids}, 404)
	worksCall(t, s, "POST", "/api/usage/delete", token, map[string]any{"all": true}, 400)
	worksCall(t, s, "GET", "/api/usage?from=bad", token, nil, 400)
	before := worksObject(t, worksCall(t, s, "GET", "/api/dashboard", token, nil, 200))["totals"]
	worksCall(t, s, "POST", "/api/usage/delete", token, map[string]any{"all": true, "from": "2026-10-10T04:00:00Z", "to": "2026-10-10T04:03:00Z"}, 200)
	after := worksObject(t, worksCall(t, s, "GET", "/api/dashboard", token, nil, 200))["totals"]
	if string(asJSON(before)) != string(asJSON(after)) || before == nil {
		t.Fatal("deletion lost usage totals", before, after)
	}
	v = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if v["total"] != float64(0) {
		t.Fatal("details not removed", v)
	}
}

func TestInterruptedCapacityProtectsDraftsAndBulkIsolation(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "low")
	other := worksLogin(t, s, "other")
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	ids := []string{}
	for i := 0; i < 14; i++ {
		id := worksFixture(t, s, token, "courseware", "failed", "capacity", TaskInput{})
		ids = append(ids, id)
	}
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), "UPDATE tasks SET result=$1 WHERE id=ANY($2::text[])", asJSON(sampleDeck(TaskInput{Pages: 8, Theme: "ocean", Topic: "草稿"})), ids)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	json.Unmarshal(worksCall(t, s, "GET", "/api/tasks?view=interrupted", token, nil, 200).Body.Bytes(), &rows)
	if len(rows) != 12 {
		t.Fatal("interrupted limit", len(rows))
	}
	var held []map[string]any
	json.Unmarshal(worksCall(t, s, "GET", "/api/tasks?view=works", token, nil, 200).Body.Bytes(), &held)
	if len(held) != 2 || held[0]["capacity_hold"] != true {
		t.Fatal("valuable drafts lost", held)
	}
	worksCall(t, s, "POST", "/api/tasks/delete", other, map[string]any{"ids": ids[:2]}, 404)
	worksCall(t, s, "POST", "/api/tasks/delete", token, map[string]any{"ids": []string{ids[0], newID()}}, 404)
	worksCall(t, s, "GET", "/api/tasks/"+ids[0], token, nil, 200)
	worksCall(t, s, "POST", "/api/tasks/delete", token, map[string]any{"ids": ids[:3]}, 200)
	worksCall(t, s, "GET", "/api/tasks/"+ids[0], token, nil, 404)
	json.Unmarshal(worksCall(t, s, "GET", "/api/tasks?view=interrupted", token, nil, 200).Body.Bytes(), &rows)
	if len(rows) != 11 {
		t.Fatal("capacity did not promote preserved drafts", len(rows))
	}
}

func TestUsageRetentionAndRevisionCapacity(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "admin")
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(context.Background(), `INSERT INTO usage_logs(tenant_id,member_id,operation,model,latency_ms,success,created_at)
 SELECT $1,$2,'retention','fixture',100,true,now()-i*interval '1 second' FROM generate_series(1,5001) i`, a.TenantID, a.MemberID)
	if err != nil {
		t.Fatal(err)
	}
	var details, allCalls int
	if err = tx.QueryRow(context.Background(), "SELECT count(*) FROM usage_logs WHERE member_id=$1", a.MemberID).Scan(&details); err != nil {
		t.Fatal(err)
	}
	if details != 5000 {
		t.Fatal("detail cap", details)
	}
	if err = tx.QueryRow(context.Background(), "SELECT calls FROM usage_totals WHERE member_id=$1 AND period='all'", a.MemberID).Scan(&allCalls); err != nil {
		t.Fatal(err)
	}
	if allCalls < 5001 {
		t.Fatal("retention changed total", allCalls)
	}
	_, err = tx.Exec(context.Background(), "UPDATE usage_logs SET created_at=now()-interval '91 days' WHERE member_id=$1", a.MemberID)
	if err != nil {
		t.Fatal(err)
	}
	if err = pruneUsageRetention(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(context.Background(), "SELECT count(*) FROM usage_logs WHERE member_id=$1", a.MemberID).Scan(&details); err != nil || details != 0 {
		t.Fatal("age cap", details, err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	id := worksFixture(t, s, token, "courseware", "succeeded", "bounded history", TaskInput{})
	tx, err = s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	for i := 1; i <= 25; i++ {
		if _, err = tx.Exec(context.Background(), "UPDATE tasks SET revision=$1 WHERE id=$2", i, id); err != nil {
			t.Fatal(err)
		}
		if err = archiveRevision(context.Background(), tx, a.TenantID, id); err != nil {
			t.Fatal(err)
		}
	}
	var versions int
	if err = tx.QueryRow(context.Background(), "SELECT count(*) FROM artifact_revisions WHERE task_id=$1", id).Scan(&versions); err != nil || versions != 21 {
		t.Fatal("history cap", versions, err)
	}
	var first bool
	if err = tx.QueryRow(context.Background(), "SELECT EXISTS(SELECT 1 FROM artifact_revisions WHERE task_id=$1 AND revision=1)", id).Scan(&first); err != nil || !first {
		t.Fatal("first version lost", err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
}
