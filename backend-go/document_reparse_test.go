package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocumentReparseKeepsIdentityAndSavedSourceHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "other")
	var a Actor
	jsonActor := worksCall(t, s, "GET", "/api/auth/me", token, nil, 200)
	if err := json.Unmarshal(jsonActor.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	old := []Page{{1, "旧原文：y=x1_2"}}
	doc, err := s.saveDocument(context.Background(), a, "教材.pdf", "", old)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.cfg.Storage, "original.pdf")
	if err = os.WriteFile(path, []byte("original fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), "UPDATE documents SET upload_path=$1 WHERE id=$2", path, doc)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	var documents []map[string]any
	if err = json.Unmarshal(worksCall(t, s, "GET", "/api/documents", token, nil, 200).Body.Bytes(), &documents); err != nil {
		t.Fatal(err)
	}
	available := false
	for _, item := range documents {
		if item["id"] == doc {
			available = item["original_available"] == true
		}
	}
	if !available {
		t.Fatal("original-file availability missing from document list")
	}
	in := TaskInput{MaterialID: doc, MaterialStart: 1, MaterialEnd: 1, Topic: "保存的旧来源", UseKB: true}
	if err = s.resolveTeachingMaterials(context.Background(), a, &in); err != nil {
		t.Fatal(err)
	}
	sources := appendTeachingSources(in, nil)
	before := string(asJSON(sources))
	id := worksFixture(t, s, token, "courseware", "failed", "旧作品来源", in)
	tx, err = s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), "UPDATE tasks SET sources=$1 WHERE id=$2", asJSON(sources), id)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	billing := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	route := "/api/documents/" + doc + "/reparse"
	worksCall(t, s, "POST", route, "", nil, 401)
	worksCall(t, s, "POST", route, other, nil, 409)
	worksCall(t, s, "POST", route, token, nil, 202)
	worksCall(t, s, "POST", route, token, nil, 409)
	tx, err = s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	var pages []Page
	var status string
	if err = tx.QueryRow(context.Background(), "SELECT status,pages FROM documents WHERE id=$1", doc).Scan(&status, &pages); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || string(asJSON(pages)) != string(asJSON(old)) {
		t.Fatal("queue destroyed previous extraction", status, pages)
	}
	_, err = tx.Exec(context.Background(), "UPDATE documents SET status='indexing' WHERE id=$1", doc)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Same transaction path used by the real document worker.
	got, err := s.saveDocument(context.Background(), a, "教材.pdf", "", []Page{{1, "新原文：y=1/(x^2)"}}, doc)
	if err != nil || got != doc {
		t.Fatal("reparse duplicated document", got, err)
	}
	v := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
	var savedSources []Source
	if err = json.Unmarshal(asJSON(v["sources"]), &savedSources); err != nil {
		t.Fatal(err)
	}
	if string(asJSON(savedSources)) != before {
		t.Fatal("old task source snapshot rewritten")
	}
	tx, err = s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var count int
	var text string
	if err = tx.QueryRow(context.Background(), "SELECT count(*) FROM documents WHERE id=$1", doc).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(context.Background(), "SELECT string_agg(content,' ') FROM chunks WHERE document_id=$1", doc).Scan(&text); err != nil {
		t.Fatal(err)
	}
	if count != 1 || !strings.Contains(text, "y=1/(x^2)") || strings.Contains(text, "x1_2") {
		t.Fatal("old chunks retained or missing new chunks", count, text)
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != billing {
		t.Fatal("reparse changed business points")
	}
}
