package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestCoursewareHistoryRestoreHTTP(t *testing.T) {
	s := worksServer(t)
	owner, admin, other := worksLogin(t, s, "teacher"), worksLogin(t, s, "admin"), worksLogin(t, s, "other")
	in := TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "forest", PreviewFirst: true, UseKB: true, IdempotencyKey: newID()}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", owner, in, 202))["id"].(string)
	// This persisted legacy draft must retain a real reservation for delivery.
	in.Detail = ""
	tx, err := s.tx(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(context.Background(), "UPDATE tasks SET input=$1,result=$2,status='awaiting_confirmation',stage='awaiting_confirmation' WHERE id=$3", asJSON(in), asJSON(sampleDeck(in)), id); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	route := "/api/tasks/" + id
	draft := worksObject(t, worksCall(t, s, "GET", route, owner, nil, 200))
	var media Deck
	if err := json.Unmarshal(asJSON(draft["result"]), &media); err != nil {
		t.Fatal(err)
	}
	media.Theme = "classroom"
	media.Slides[0].Picture = teachingTestPicture(t)
	media.Slides[0].Focus = "观察图示，提出一个问题。"
	media.Slides[0].Emphasis = []string{"观察"}
	expectedMedia := string(asJSON(media.Slides))
	worksCall(t, s, "POST", route+"/draft", owner, map[string]any{"result": media, "revision": draft["revision"]}, 200)
	draft = worksObject(t, worksCall(t, s, "GET", route, owner, nil, 200))
	var persisted Deck
	json.Unmarshal(asJSON(draft["result"]), &persisted)
	if persisted.Slides[0].Picture == nil || persisted.Slides[0].Focus != media.Slides[0].Focus {
		t.Fatal("saved draft lost media")
	}
	expectedMedia = string(asJSON(persisted.Slides))
	for _, entry := range draft["slide_reviews"].([]any) {
		m := entry.(map[string]any)
		worksCall(t, s, "POST", route+"/slides/"+m["id"].(string)+"/review", owner, map[string]any{"revision": draft["revision"], "hash": m["hash"], "approved": true}, 200)
	}
	worksCall(t, s, "POST", route+"/confirm", owner, map[string]any{"result": draft["result"], "revision": draft["revision"], "accept_review": true}, 202)
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	original := worksObject(t, worksCall(t, s, "GET", route, owner, nil, 200))
	var stored Deck
	json.Unmarshal(asJSON(original["result"]), &stored)
	if string(asJSON(stored.Slides)) != expectedMedia {
		t.Fatal("media lost after confirmation and rendering")
	}
	v := int(original["revision"].(float64))
	history := route + "/revisions"
	version := fmt.Sprintf("%s/%d", history, v)
	file := worksCall(t, s, "GET", route+"/download/pptx", owner, nil, 200).Body.Bytes()
	var rows []map[string]any
	json.Unmarshal(worksCall(t, s, "GET", history, owner, nil, 200).Body.Bytes(), &rows)
	if len(rows) != 2 || rows[0]["revision"] != float64(v) || rows[1]["revision"] != float64(v-2) || rows[0]["pptx_path"] != nil {
		t.Fatal("history list", rows)
	}
	for _, token := range []string{admin, other} {
		worksCall(t, s, "GET", history, token, nil, 404)
		worksCall(t, s, "GET", version, token, nil, 404)
		worksCall(t, s, "GET", version+"/download/pptx", token, nil, 404)
		worksCall(t, s, "POST", version+"/restore", token, map[string]any{"revision": v}, 404)
	}
	worksCall(t, s, "GET", history+"/-1", owner, nil, 400)
	worksCall(t, s, "GET", history+"/999999", owner, nil, 404)
	worksCall(t, s, "GET", history+"?offset=-1", owner, nil, 400)
	before := worksCall(t, s, "GET", "/api/billing", owner, nil, 200).Body.String()
	result := original["result"].(map[string]any)
	result["title"] = "教师手动修订"
	result["lesson"] = "保留原版以便恢复"
	result["slides"].([]any)[0].(map[string]any)["focus"] = "手工修订的课堂提问"
	result["slides"].([]any)[0].(map[string]any)["picture"].(map[string]any)["caption"] = "手工修订图片说明"
	worksCall(t, s, "POST", route+"/edit", owner, map[string]any{"revision": v, "result": result}, 200)
	archived := worksObject(t, worksCall(t, s, "GET", version, owner, nil, 200))
	if archived["result"].(map[string]any)["title"] == result["title"] {
		t.Fatal("old version overwritten")
	}
	if !bytes.Equal(file, worksCall(t, s, "GET", version+"/download/pptx", owner, nil, 200).Body.Bytes()) {
		t.Fatal("historical download changed")
	}
	worksCall(t, s, "POST", version+"/restore", owner, map[string]any{"revision": v}, 409)
	worksCall(t, s, "POST", version+"/restore", owner, map[string]any{"revision": v + 1}, 200)
	restored := worksObject(t, worksCall(t, s, "GET", route, owner, nil, 200))
	json.Unmarshal(asJSON(restored["result"]), &stored)
	if string(asJSON(stored.Slides)) != expectedMedia || stored.Theme != "classroom" {
		t.Fatal("restoration lost picture, focus, emphasis, table or figure")
	}
	if restored["revision"] != float64(v+2) || restored["result"].(map[string]any)["title"] != archived["result"].(map[string]any)["title"] {
		t.Fatal("restore did not append new revision")
	}
	worksCall(t, s, "POST", version+"/restore", owner, map[string]any{"revision": v + 1}, 409)
	if restored["result"].(map[string]any)["evaluation"] != nil {
		t.Fatal("stale evaluation restored")
	}
	if !bytes.Contains(worksCall(t, s, "GET", route+"/download/pptx", owner, nil, 200).Body.Bytes()[:2], []byte("PK")) {
		t.Fatal("no restored PPTX")
	}
	if after := worksCall(t, s, "GET", "/api/billing", owner, nil, 200).Body.String(); after != before {
		t.Fatal("free restore changed ledger")
	}
	json.Unmarshal(worksCall(t, s, "GET", history, owner, nil, 200).Body.Bytes(), &rows)
	if len(rows) != 4 || rows[0]["revision"] != float64(v+2) || rows[3]["revision"] != float64(v-2) {
		t.Fatal("restoration lost history", rows)
	}
	child := worksObject(t, worksCall(t, s, "POST", route+"/rewrite", owner, map[string]any{"revision": v + 2, "instruction": "补充说明", "slide": 0, "idempotency_key": newID()}, 202))["id"].(string)
	worksCall(t, s, "POST", version+"/restore", owner, map[string]any{"revision": v + 2}, 409)
	worksCall(t, s, "POST", "/api/tasks/"+child+"/cancel", owner, nil, 200)
	worksCall(t, s, "DELETE", route, owner, nil, 200)
	for _, path := range []string{history, version, version + "/download/pptx"} {
		worksCall(t, s, "GET", path, owner, nil, 404)
	}
	worksCall(t, s, "POST", version+"/restore", owner, map[string]any{"revision": v + 2}, 404)
}
