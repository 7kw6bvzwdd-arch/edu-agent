package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"
)

// Advance only the persisted queue delay; never invoke the user's retry route.
func makeAutomaticRetryReady(t *testing.T, s *Server, id string) {
	t.Helper()
	tx, err := s.tx(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(context.Background(), "UPDATE tasks SET next_attempt_at=now() WHERE id=$1", id)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAutomaticResumeClassification(t *testing.T) {
	for _, c := range []struct {
		err  error
		want bool
	}{
		{coursewareRepairExhausted{Page: 4}, true},
		{providerCallError{Status: 503, Temporary: true}, true},
		{providerCallError{Status: 500}, true},
		{context.DeadlineExceeded, true},
		{pageReviewUnavailable{errors.New("审核返回不完整")}, true},
		{pageReviewUnavailable{providerCallError{Status: 401}}, false},
		{pageReviewUnavailable{}, false},
		{context.Canceled, false},
		{providerCallError{Status: 401}, false},
	} {
		if got := coursewareCanResume(c.err); got != c.want {
			t.Errorf("%v: resume=%v want=%v", c.err, got, c.want)
		}
	}
	if coursewareResumeDelay(1) != 15*time.Second || coursewareResumeDelay(2) != 30*time.Second || coursewareResumeDelay(100) != time.Hour {
		t.Fatal("retry is unbounded or lacks backoff")
	}
}

func TestAutomaticResumeQueueAndLeaseHTTP(t *testing.T) {
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
	_, err = tx.Exec(context.Background(), "UPDATE tasks SET status='researching',attempt=1,lease_until=now()-interval '1 second' WHERE id=$1", id)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	old := context.WithValue(context.Background(), generationCycleKey{}, 0)
	old = context.WithValue(old, generationAttemptKey{}, 1)
	if err = s.recoverExpiredTasks(context.Background()); err != nil {
		t.Fatal(err)
	}
	v := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if v["status"] != "pending" || v["stage"] != "auto_recovering" || v["charge_state"] != "reserved" {
		t.Fatal("expired worker requires manual resume", v["status"], v["error"])
	}
	// Not yet due: no generation, no charge, no duplicate event.
	tx, err = s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(context.Background(), "UPDATE tasks SET next_attempt_at=now()+interval '1 hour' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}

	events := len(v["events"].([]any))
	if err = s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	v = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if v["status"] != "pending" || len(v["events"].([]any)) != events {
		t.Fatal("queue ignored its persisted delay")
	}
	if err = s.finish(old, a.TenantID, id, "failed", "late worker", "", nil); err != nil {
		t.Fatal(err)
	}
	v = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if v["charge_state"] != "reserved" {
		t.Fatal("old worker released resumed reservation")
	}
	makeAutomaticRetryReady(t, s, id)
	if err = s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	v = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if v["status"] != "awaiting_preview" {
		t.Fatal("automatic queue did not reach teacher preview", v["status"], v["error"])
	}
	// The old attempt cannot overwrite a newer attempt even in the same cycle.
	if err = s.saveCoursewareCheckpoint(old, a, id, Deck{Slides: []Slide{{Title: "late"}}}, nil); err == nil {
		t.Fatal("old checkpoint overwrote resumed work")
	}
	tx, err = s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	var reserve, release int
	err = tx.QueryRow(context.Background(), "SELECT count(*) FILTER(WHERE kind='reserve'),count(*) FILTER(WHERE kind='release') FROM ledger WHERE reference=$1 OR reference LIKE $1||':%'", id).Scan(&reserve, &release)
	tx.Rollback(context.Background())
	if err != nil || reserve != 1 || release != 0 {
		t.Fatal("automatic resume changed billing", reserve, release, err)
	}
	worksCall(t, s, "POST", route+"/cancel", token, nil, 200)
	if resumed, err := s.resumeCoursewareAutomatically(old, a, id, "researching", in, context.DeadlineExceeded); err != nil || !resumed {
		t.Fatal("late recovery should be a no-op", resumed, err)
	}
	if err = s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	v = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if v["status"] != "cancelled" || v["charge_state"] != "released" {
		t.Fatal("automatic recovery resurrected cancellation")
	}
}

func TestReviewOutageAutomaticallyRechecksSavedPageHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	in := TaskInput{Type: "courseware", Topic: "浮力", Subject: "物理", Detail: "concise", LessonMinutes: 45, Theme: "ocean", UseKB: true, PreviewFirst: true, IdempotencyKey: newID()}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	route := "/api/tasks/" + id
	defer worksCall(t, s, "POST", route+"/cancel", token, nil, 200)
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	plan, err := s.planCourseware(context.Background(), a, id, in, nil)
	if err != nil {
		t.Fatal(err)
	}
	in.Pages = len(plan.Pages)
	generated, reviewed := 0, 0
	outage := true
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model    string
			Messages []struct{ Content any }
		}
		json.NewDecoder(r.Body).Decode(&req)
		raw := ""
		if req.Model == "generation" {
			generated++
			var page, end int
			fmt.Sscanf(regexp.MustCompile(`本批只返回第\d+至第\d+页`).FindString(req.Messages[len(req.Messages)-1].Content.(string)), "本批只返回第%d至第%d页", &page, &end)
			if page < 1 || page > in.Pages || page != end {
				t.Error("nonlocal generation")
				w.WriteHeader(400)
				return
			}
			d := sampleDeck(in)
			d.Slides = d.Slides[page-1 : page]
			raw = string(asJSON(d))
		} else {
			reviewed++
			if outage {
				raw = `{}`
			} else {
				raw = `{"issues":[]}`
			}
		}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": raw}}}})
	}))
	defer provider.Close()
	s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "fixture", "generation"
	s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = provider.URL, "fixture", "review"
	s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = provider.URL, "fixture", "vision"
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), "UPDATE tasks SET mode='live',result=$1 WHERE id=$2", asJSON(Deck{Plan: plan, Generation: &CoursewareCheckpoint{Phase: "preview"}}), id)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	v := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	var saved Deck
	json.Unmarshal(asJSON(v["result"]), &saved)
	if v["status"] != "pending" || v["charge_state"] != "reserved" || len(saved.Slides) != 1 || generated != 1 {
		t.Fatal("review outage requires click or loses generated page", v["status"], v["error"], generated, v["events"])
	}
	hash := slideContentHash(saved, saved.Slides[0])
	if pageCheckPassed(saved.PageChecks[saved.Slides[0].ID], false) {
		t.Fatal("unavailable review passed")
	}
	outage = false
	makeAutomaticRetryReady(t, s, id)
	if err = s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	v = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	var done Deck
	json.Unmarshal(asJSON(v["result"]), &done)
	if v["status"] != "awaiting_preview" || generated != 3 || reviewed != 7 || hash != slideContentHash(done, done.Slides[0]) {
		t.Fatal("review recovery regenerated saved page or failed teacher preview", v["status"], v["error"], generated, reviewed)
	}
}
