package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestCoursewareProviderBudgetPersistsAcrossRecoveryHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	in := TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "ocean", UseKB: true, PreviewFirst: true, IdempotencyKey: newID()}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	route := "/api/tasks/" + id
	defer worksCall(t, s, "POST", route+"/cancel", token, nil, 200)
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	// Budget exhaustion in a saved draft must retain its continuation. An empty
	// fixture can legitimately be evicted by the twelve-record retention policy
	// when earlier tests have already filled that member's pending collection.
	draft := Deck{Title: "浮力", Slides: []Slide{{ID: newID(), Title: "已保存的浮力概念", Bullets: []string{"浮力方向竖直向上"}, Notes: "当前课件已有有效内容，供应商异常时应保留这一页与续作入口。"}}}
	_, err = tx.Exec(context.Background(), "UPDATE tasks SET mode='live',status='researching',courseware_provider_calls=$1,result=$3 WHERE id=$2", coursewareProviderLimit(8)-1, id, asJSON(draft))
	if err != nil {
		tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.cfg.Mode = "live"
	if err = s.reserveCoursewareProviderCall(context.Background(), a, id); err != nil {
		t.Fatal("last permitted call rejected", err)
	}
	if err = s.reserveCoursewareProviderCall(context.Background(), a, id); err == nil {
		t.Fatal("budget exceeded")
	}
	var exhausted coursewareBudgetExhausted
	if !errors.As(err, &exhausted) || coursewareCanResume(pageReviewUnavailable{err}) {
		t.Fatal("budget exhaustion loops forever", err)
	}
	if _, err = s.coursewarePageRequest(context.Background(), a, id, in, nil, "", 1); !errors.As(err, &exhausted) || coursewareCanResume(err) {
		t.Fatal("page request hid exhausted budget", err)
	}
	in.Detail = "default"
	if _, err = s.planCourseware(context.Background(), a, id, in, nil); !errors.As(err, &exhausted) || coursewareCanResume(err) {
		t.Fatal("planning hid exhausted budget", err)
	}
	// Deleting details or resetting per-stage automatic failures cannot buy calls.
	tx, err = s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), "DELETE FROM usage_logs WHERE task_id=$1", id)
	if err == nil {
		_, err = tx.Exec(context.Background(), "UPDATE tasks SET automatic_failures=0 WHERE id=$1", id)
	}
	if err != nil {
		tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = s.reserveCoursewareProviderCall(context.Background(), a, id); !errors.As(err, &exhausted) {
		t.Fatal("counter reset on recovery", err)
	}
	if err = s.finish(context.Background(), a.TenantID, id, "failed", exhausted.Error(), "", nil); err != nil {
		t.Fatal(err)
	}
	worksCall(t, s, "POST", route+"/retry", token, map[string]any{"idempotency_key": "budget-explicit-retry"}, 202)
	tx, err = s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	var calls int
	err = tx.QueryRow(context.Background(), "SELECT courseware_provider_calls FROM tasks WHERE id=$1", id).Scan(&calls)
	tx.Rollback(context.Background())
	if err != nil || calls != 0 {
		t.Fatal("explicit new generation cycle cannot resume after configuration repair", err, calls)
	}
	tx, err = s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), "UPDATE tasks SET status='cancelled' WHERE id=$1", id)
	if err != nil {
		tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = s.reserveCoursewareProviderCall(context.Background(), a, id); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled worker could contact supplier", err)
	}
	if coursewareProviderLimit(15) <= coursewareProviderLimit(8) || coursewareProviderLimit(1000) != coursewareProviderLimit(100) {
		t.Fatal("page-based safety bound invalid")
	}
}
