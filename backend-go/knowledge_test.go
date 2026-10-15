package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestKnowledgeLargeDocumentForwarding(t *testing.T) {
	const size = 100 << 20
	path := filepath.Join(t.TempDir(), "large.pdf")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(size); err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteAt([]byte("END"), size-3); err != nil {
		t.Fatal(err)
	}
	f.Close()
	renderer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader, err := r.MultipartReader()
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		part, err := reader.NextPart()
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		n, err := io.CopyN(io.Discard, part, size-3)
		tail, tailErr := io.ReadAll(part)
		if err != nil || tailErr != nil || n != size-3 || string(tail) != "END" {
			t.Errorf("document truncated: bytes=%d tail=%q error=%v", n, tail, err)
		}
		respond(w, 200, map[string]any{"pages": []Page{{Page: 1, Text: "large document"}}})
	}))
	defer renderer.Close()
	s := &Server{cfg: Config{RenderURL: renderer.URL}, http: renderer.Client()}
	if _, err = s.extractFile(context.Background(), path, "large.pdf"); err != nil {
		t.Fatal(err)
	}
}

func TestKnowledgeOversizedUpload(t *testing.T) {
	// A large sparse file can remain delete-pending briefly on Windows. Keep
	// it outside a TempDir so cleanup need not immediately remove its parent.
	f, err := os.CreateTemp("", "edu-oversized-upload-*.pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		f.Close()
		if err := os.Remove(f.Name()); err != nil {
			t.Error(err)
		}
	}()
	if err = f.Truncate((100 << 20) + 1); err != nil {
		t.Fatal(err)
	}
	reader, writer := io.Pipe()
	m := multipart.NewWriter(writer)
	done := make(chan struct{})
	go func() {
		defer close(done)
		part, err := m.CreateFormFile("file", "large.pdf")
		if err == nil {
			_, err = io.Copy(part, f)
		}
		if err == nil {
			err = m.Close()
		}
		writer.CloseWithError(err)
	}()
	defer func() { reader.Close(); <-done }()
	r := httptest.NewRequest("POST", "/api/documents", reader)
	r.Header.Set("Content-Type", m.FormDataContentType())
	s := &Server{cfg: Config{Storage: t.TempDir()}}
	err = s.uploadDocument(httptest.NewRecorder(), r, Actor{TenantID: 1})
	if err == nil || err.Error() != "文件超过100MB" {
		t.Fatalf("expected 100MB upload rejection, got %v", err)
	}
}

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
