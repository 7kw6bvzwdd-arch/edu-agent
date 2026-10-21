package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"
)

func TestCloudHealthHTTP(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("requires isolated test database")
	}
	s, err := NewServer(context.Background(), Config{DatabaseURL: isolatedTestDatabase(t), RuntimeURL: os.Getenv("TEST_RUNTIME_DATABASE_URL"), Secret: "test-secret-at-least-thirty-two-bytes", Mode: "sample", Storage: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/health", nil))
	var v map[string]any
	json.Unmarshal(w.Body.Bytes(), &v)
	deps, ok := v["dependencies"].(map[string]any)
	if w.Code != 200 || !ok || deps["postgresql"] != "ok" || deps["minio"] != "local" || deps["redis"] != "disabled" || deps["milvus"] != "disabled" {
		t.Fatalf("health must distinguish checked dependencies: %d %s", w.Code, w.Body.String())
	}
	if v["release_tag"] != releaseTag || v["release_commit"] != releaseCommit {
		t.Fatal("health must identify the running release")
	}
}

func cloudTestConfig(t *testing.T) Config {
	t.Helper()
	if os.Getenv("TEST_CLOUD") != "1" {
		t.Skip("requires isolated cloud test services")
	}
	return Config{DatabaseURL: isolatedTestDatabase(t), RuntimeURL: os.Getenv("TEST_RUNTIME_DATABASE_URL"), Secret: "test-secret-at-least-thirty-two-bytes", Mode: "sample", SeedPassword: "test-pass-123", Storage: t.TempDir(), RenderURL: os.Getenv("TEST_RENDER_URL"), MinIOEndpoint: os.Getenv("MINIO_ENDPOINT"), MinIOAccessKey: os.Getenv("MINIO_ACCESS_KEY"), MinIOSecretKey: os.Getenv("MINIO_SECRET_KEY"), MinIOBucket: "edu-agent-test", MinIOPrefix: "tests/" + newID() + "/", RedisAddr: os.Getenv("REDIS_ADDR"), RedisPassword: os.Getenv("REDIS_PASSWORD"), RedisPrefix: "edu-agent:test:" + newID() + ":", MilvusURL: os.Getenv("MILVUS_URL"), MilvusToken: os.Getenv("MILVUS_TOKEN"), MilvusDatabase: "edu_agent_test", MilvusCollection: "edu_agent_chunks"}
}
