package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestConflictingCoursewareCompletesHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		d := sampleDeck(TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "forest"})
		d.Slides[6].Diagram = "buoyancy"
		d.Slides[6].Chart = d.Slides[4].Chart
		d.Slides[6].Figure = d.Slides[7].Figure
		d.Slides[7].Chart = d.Slides[4].Chart
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(asJSON(d))}}}})
	}))
	defer provider.Close()
	s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "test", "test"
	input := TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "forest", UseKB: true, IdempotencyKey: newID()}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, input, 202))["id"].(string)
	route := "/api/tasks/" + id
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	task := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if task["status"] != "awaiting_confirmation" {
		t.Fatalf("conflict halted task: %v", task["error"])
	}
	var deck Deck
	json.Unmarshal(asJSON(task["result"]), &deck)
	if len(deck.Slides) != 12 || deck.Slides[6].Table == nil || deck.Slides[9].Diagram != "buoyancy" || calls != 1 {
		t.Fatal("media lost or unnecessary model call", len(deck.Slides), calls)
	}
	worksCall(t, s, "POST", route+"/confirm", token, map[string]any{"result": deck, "revision": task["revision"], "accept_review": true}, 202)
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	task = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if task["status"] != "succeeded" || task["charge_state"] != "settled" {
		t.Fatal("did not complete", task["error"])
	}
	b := worksCall(t, s, "GET", route+"/download/pptx", token, nil, 200).Body.Bytes()
	if len(b) < 1000 || string(b[:2]) != "PK" {
		t.Fatal("not real PPTX")
	}
}

func TestCoursewareRetryIsolationAndReservationHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "other")
	admin := worksLogin(t, s, "admin")
	in := TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "forest", UseKB: true, IdempotencyKey: newID()}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	d := sampleDeck(in)
	if err := s.finish(context.Background(), a.TenantID, id, "failed", "服务暂时不可用", "", d); err != nil {
		t.Fatal(err)
	}
	route := "/api/tasks/" + id + "/retry"
	worksCall(t, s, "POST", route, other, nil, 404)
	worksCall(t, s, "POST", route, admin, nil, 404)
	before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	responses := make(chan *httptest.ResponseRecorder, 4)
	for i := 0; i < 4; i++ {
		go func() {
			r := httptest.NewRequest("POST", route, nil)
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			responses <- w
		}()
	}
	var retry string
	for i := 0; i < 4; i++ {
		w := <-responses
		if w.Code != 202 {
			t.Fatalf("concurrent retry: %d %s", w.Code, w.Body.String())
		}
		id := worksObject(t, w)["id"].(string)
		if retry != "" && retry != id {
			t.Fatal("duplicate retry")
		}
		retry = id
	}
	v := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+retry, token, nil, 200))
	if v["status"] != "expand_pending" || !strings.Contains(string(asJSON(v["result"])), "浮力") {
		t.Fatal("draft not reused")
	}
	after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	if before["balance"] != after["balance"] || after["reserved"].(float64)-before["reserved"].(float64) != 20 {
		t.Fatal("retry reservation incorrect")
	}
	worksCall(t, s, "POST", "/api/tasks/"+retry+"/cancel", token, nil, 200)
	worksCall(t, s, "POST", "/api/tasks/"+retry+"/retry", token, nil, 409)
	worksCall(t, s, "DELETE", "/api/tasks/"+retry, token, nil, 200)
	if retry != id {
		t.Fatal("continuation must retain work identity")
	}
	worksCall(t, s, "POST", route, token, nil, 404)
}

func TestCoursewareRetryInsufficientQuotaHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "low")
	id := worksFixture(t, s, token, "courseware", "failed", "额度检查", TaskInput{UseKB: true})
	var actor Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &actor)
	tx, err := s.tx(context.Background(), actor.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(context.Background(), "UPDATE tasks SET charge_state='released' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	worksCall(t, s, "POST", "/api/tasks/"+id+"/retry", token, nil, 402)
	if after := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String(); after != before {
		t.Fatal("rejected retry changed billing")
	}
}

func TestCoursewareRetryFromOldFailedAttemptContinuesLatestFailure(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	in := TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "forest", UseKB: true, PreviewFirst: true, IdempotencyKey: newID()}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	var actor Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &actor)
	if err := s.finish(context.Background(), actor.TenantID, id, "failed", "第一次暂停", "", nil); err != nil {
		t.Fatal(err)
	}
	child := worksObject(t, worksCall(t, s, "POST", "/api/tasks/"+id+"/retry", token, nil, 202))["id"].(string)
	if err := s.finish(context.Background(), actor.TenantID, child, "failed", "第二次暂停", "", nil); err != nil {
		t.Fatal(err)
	}
	before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	next := worksObject(t, worksCall(t, s, "POST", "/api/tasks/"+id+"/retry", token, nil, 202))["id"].(string)
	if next != id || next != child {
		t.Fatal("continuation must reuse the same work")
	}
	if again := worksObject(t, worksCall(t, s, "POST", "/api/tasks/"+id+"/retry", token, nil, 202))["id"].(string); again != next {
		t.Fatal("repeated click created another reservation")
	}
	after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	if after["reserved"].(float64)-before["reserved"].(float64) != 20 {
		t.Fatal("continuation reservation incorrect")
	}
	worksCall(t, s, "POST", "/api/tasks/"+next+"/cancel", token, nil, 200)
}

func TestCoursewareRecoveryRejectsUnknownCitations(t *testing.T) {
	d := sampleDeck(TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "forest"})
	d.Slides[2].Notes += " [S1]"
	if reusableCourseware(d, nil) || !reusableCourseware(d, []Source{{ID: "S1"}}) {
		t.Fatal("recovery must retain original citation validation")
	}
}

func TestCoursewareArrangementPreservesContent(t *testing.T) {
	d := sampleDeck(TaskInput{Type: "courseware", Topic: "浮力", Pages: 20, Theme: "forest"})
	table, figure, chart := d.Slides[6].Table, d.Slides[19].Figure, d.Slides[4].Chart
	for i := range d.Slides {
		d.Slides[i].Table, d.Slides[i].Figure, d.Slides[i].Chart, d.Slides[i].Diagram = table, figure, chart, "buoyancy"
		// This fixture isolates media separation. Dense text is covered separately.
		d.Slides[i].Bullets = d.Slides[i].Bullets[:min(2, len(d.Slides[i].Bullets))]
	}
	before := string(asJSON(d))
	got := arrangeCourseware(d)
	if len(got.Slides) != 80 || string(asJSON(d)) != before || !reflect.DeepEqual(arrangeCourseware(got), got) {
		t.Fatal("splitting lost pages, mutated the input, or was not idempotent")
	}
	if err := validateDeck(got); err != nil {
		t.Fatal(err)
	}
	for i, original := range d.Slides {
		parts := got.Slides[i*4 : (i+1)*4]
		if parts[0].Table != table || parts[1].Figure != figure || parts[2].Chart == nil || parts[3].Diagram != "buoyancy" ||
			parts[0].Title != original.Title || !reflect.DeepEqual(parts[0].Bullets, original.Bullets) || parts[0].Formula != original.Formula || parts[0].Focus != original.Focus {
			t.Fatal("original teaching content or media lost")
		}
		for _, part := range parts {
			if part.Notes != original.Notes {
				t.Fatal("teacher notes lost")
			}
		}
	}
}
