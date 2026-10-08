package main

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"testing"
)

func TestKnowledgeHTTPIsolationAndRetry(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" || os.Getenv("TEST_RENDER_URL") == "" {
		t.Skip("requires test PostgreSQL and renderer")
	}
	s, e := NewServer(context.Background(), Config{DatabaseURL: os.Getenv("TEST_DATABASE_URL"), RuntimeURL: os.Getenv("TEST_RUNTIME_DATABASE_URL"), Secret: "test-secret-at-least-thirty-two-bytes", Mode: "sample", SeedPassword: "test-pass-123", RenderURL: os.Getenv("TEST_RENDER_URL"), Storage: t.TempDir()})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	call := func(method, path, token string, body any) (int, []byte) {
		r := httptest.NewRequest(method, path, bytes.NewReader(asJSON(body)))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w.Code, w.Body.Bytes()
	}
	login := func(user string) string {
		_, b := call("POST", "/api/auth/login", "", map[string]string{"username": user, "password": "test-pass-123"})
		var v map[string]any
		json.Unmarshal(b, &v)
		return v["token"].(string)
	}
	teacher, other := login("teacher"), login("other")
	upload := func(name, body string) string {
		var buf bytes.Buffer
		m := multipart.NewWriter(&buf)
		p, e := m.CreateFormFile("file", name)
		if e != nil {
			t.Fatal(e)
		}
		p.Write([]byte(body))
		m.Close()
		r := httptest.NewRequest("POST", "/api/documents", &buf)
		r.Header.Set("Content-Type", m.FormDataContentType())
		r.Header.Set("Authorization", "Bearer "+teacher)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 202 {
			t.Fatal(w.Code, w.Body.String())
		}
		var v map[string]string
		json.Unmarshal(w.Body.Bytes(), &v)
		return v["id"]
	}
	id := upload("隔离验收.md", "浮力测试特征词：量子教研唯一标记。排开液体的重力等于浮力。")
	if e = s.ingestOne(context.Background()); e != nil {
		t.Fatal(e)
	}
	code, b := call("POST", "/api/retrieval/test", teacher, map[string]any{"query": "量子教研唯一标记"})
	if code != 200 || !bytes.Contains(b, []byte(id)) {
		t.Fatal("own tenant document missing", string(b))
	}
	_, b = call("POST", "/api/retrieval/test", other, map[string]any{"query": "量子教研唯一标记"})
	if bytes.Contains(b, []byte(id)) {
		t.Fatal("cross tenant retrieval leaked document")
	}
	code, _ = call("DELETE", "/api/documents/"+id, other, nil)
	if code != 404 {
		t.Fatal("cross tenant delete accepted")
	}
	code, _ = call("DELETE", "/api/documents/"+id, teacher, nil)
	if code != 200 {
		t.Fatal("delete failed")
	}
	_, b = call("POST", "/api/retrieval/test", teacher, map[string]any{"query": "量子教研唯一标记"})
	if bytes.Contains(b, []byte(id)) {
		t.Fatal("deleted chunks remained searchable")
	}
	broken := upload("无法解析.pdf", "not a PDF")
	if e = s.ingestOne(context.Background()); e != nil {
		t.Fatal(e)
	}
	_, b = call("GET", "/api/documents", teacher, nil)
	if !bytes.Contains(b, []byte(`"status":"failed"`)) {
		t.Fatal("parser failure not persisted", string(b))
	}
	code, _ = call("POST", "/api/documents/"+broken+"/retry", teacher, map[string]any{})
	if code != 202 {
		t.Fatal("failed document cannot retry")
	}
	call("DELETE", "/api/documents/"+broken, teacher, nil)
	// Runtime role with no tenant context must not see even its own business rows.
	if s.cfg.RuntimeURL != "" {
		var count int
		if e = s.db.QueryRow(context.Background(), "SELECT count(*) FROM members").Scan(&count); e != nil {
			t.Fatal(e)
		}
		if count != 0 {
			t.Fatal("runtime role bypassed RLS")
		}
	}
}
