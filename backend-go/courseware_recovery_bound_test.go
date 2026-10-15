package main

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPersistentCoursewareOutageStopsAndRetainsPagesHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	in := TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "ocean", UseKB: true, PreviewFirst: true, IdempotencyKey: newID()}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	route := "/api/tasks/" + id
	d := sampleDeck(in)
	d.Slides = d.Slides[:1]
	tx, err := s.tx(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), "UPDATE tasks SET result=$1,status='rendering',automatic_failures=8,lease_until=now()-interval '1 second' WHERE id=$2", asJSON(d), id)
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
	if v["status"] != "failed" || v["charge_state"] != "released" || string(asJSON(saved.Slides)) != string(asJSON(d.Slides)) {
		t.Fatal("persistent outage still queued or lost saved pages", v["status"], v["charge_state"])
	}
	if err = s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	tx, err = s.tx(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var reserves, releases, settles int
	err = tx.QueryRow(context.Background(), "SELECT count(*) FILTER(WHERE kind='reserve'),count(*) FILTER(WHERE kind='release'),count(*) FILTER(WHERE kind='settle') FROM ledger WHERE reference=$1 OR reference LIKE $1||':%'", id).Scan(&reserves, &releases, &settles)
	if err != nil || reserves != 1 || releases != 1 || settles != 0 {
		t.Fatal("outage settlement not exactly once", reserves, releases, settles, err)
	}
}
