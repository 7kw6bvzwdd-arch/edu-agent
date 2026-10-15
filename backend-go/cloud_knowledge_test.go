package main

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// Synthetic embeddings are test fixtures only; cloud services and renderer are real.
func TestCloudKnowledgeHTTPIsolationCacheAndMissingLocalFile(t *testing.T) {
	c := cloudTestConfig(t)
	c.Mode = "live"
	c.EmbeddingModel = "fixture-1536"
	c.EmbeddingKey = "fixture-key"
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		if r.URL.Path != "/embeddings" || in["dimensions"] != float64(1536) {
			t.Error("incorrect embedding contract")
		}
		vector := make([]float64, 1536)
		vector[0] = 1
		respond(w, 200, map[string]any{"data": []any{map[string]any{"embedding": vector}}})
	}))
	defer provider.Close()
	c.EmbeddingURL = provider.URL
	s, err := NewServer(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	call := func(method, path, token string, body any) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewReader(asJSON(body)))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	login := func(user string) string {
		w := call("POST", "/api/auth/login", "", map[string]string{"username": user, "password": "test-pass-123"})
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var v map[string]any
		json.Unmarshal(w.Body.Bytes(), &v)
		return v["token"].(string)
	}
	teacher, other := login("teacher"), login("other")
	content := "云端缓存隔离验收：排开液体的重力等于浮力。"
	upload := func(token string) string {
		var buf bytes.Buffer
		m := multipart.NewWriter(&buf)
		part, _ := m.CreateFormFile("file", "cloud.md")
		part.Write([]byte(content))
		m.Close()
		r := httptest.NewRequest("POST", "/api/documents", &buf)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", m.FormDataContentType())
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 202 {
			t.Fatal(w.Code, w.Body.String())
		}
		var v map[string]any
		json.Unmarshal(w.Body.Bytes(), &v)
		return v["id"].(string)
	}
	id := upload(teacher)
	// Removing the local upload forces the ingestion worker to read from MinIO.
	if err = os.Remove(filepath.Join(c.Storage, "1", id+".md")); err != nil {
		t.Fatal(err)
	}
	if err = s.ingestOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	w := call("POST", "/api/retrieval/test", teacher, map[string]any{"query": content})
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(id)) {
		t.Fatal("own-cloud retrieval missing", w.Code, w.Body.String())
	}
	if calls.Load() != 1 {
		t.Fatal("same-tenant repeated input was not cached", calls.Load())
	}
	// Remove only this test tenant's PG cache; the next HTTP query must hit Redis.
	tx, err := s.tx(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), "DELETE FROM embedding_cache WHERE provider=$1", provider.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	w = call("POST", "/api/retrieval/test", teacher, map[string]any{"query": content})
	if w.Code != 200 || calls.Load() != 1 {
		t.Fatal("Redis not used by retrieval", w.Code, calls.Load())
	}
	w = call("POST", "/api/retrieval/test", other, map[string]any{"query": content})
	if w.Code != 200 || bytes.Contains(w.Body.Bytes(), []byte(id)) || calls.Load() != 2 {
		t.Fatal("tenant cache/vector isolation failed", w.Code, w.Body.String(), calls.Load())
	}
	w = call("DELETE", "/api/documents/"+id, other, nil)
	if w.Code != 404 {
		t.Fatal("cross-tenant delete", w.Code)
	}
	w = call("DELETE", "/api/documents/"+id, teacher, nil)
	if w.Code != 200 {
		t.Fatal("delete", w.Code, w.Body.String())
	}
	w = call("POST", "/api/retrieval/test", teacher, map[string]any{"query": content})
	if w.Code != 200 || bytes.Contains(w.Body.Bytes(), []byte(id)) {
		t.Fatal("deleted cloud chunks searchable", w.Code, w.Body.String())
	}
	w = call("GET", "/api/health", teacher, nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, name := range []string{"minio", "redis", "milvus", "postgresql"} {
		if !bytes.Contains(w.Body.Bytes(), []byte(`"`+name+`":"ok"`)) {
			t.Fatal("cloud health not verified", w.Body.String())
		}
	}
}
