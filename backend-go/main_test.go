package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
)

func TestHTTPMemberQuotaAndIsolation(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("Set TEST_DATABASE_URL to an isolated PostgreSQL database")
	}
	s, err := NewServer(context.Background(), Config{DatabaseURL: isolatedTestDatabase(t), Secret: "test-secret-at-least-thirty-two-bytes", Mode: "sample", RuntimeURL: os.Getenv("TEST_RUNTIME_DATABASE_URL"), SeedPassword: "test-pass-123", Storage: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	call := func(method, path, token string, body any) (int, map[string]any) {
		t.Helper()
		b, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		var v map[string]any
		json.Unmarshal(w.Body.Bytes(), &v)
		return w.Code, v
	}
	code, v := call("POST", "/api/auth/login", "", map[string]any{"username": "teacher", "password": "test-pass-123"})
	if code != 200 {
		t.Fatalf("login: %d %v", code, v)
	}
	token := v["token"].(string)
	code, v = call("GET", "/api/billing", "BearerBad", nil)
	if code != 401 {
		t.Fatal("invalid token accepted")
	}
	code, v = call("GET", "/api/billing", token, nil)
	if code != 200 {
		t.Fatal(v)
	}
	before := v["available"].(float64)
	key := newID()
	req := map[string]any{"type": "courseware", "topic": "浮力", "subject": "物理", "grade": "八年级", "pages": 8, "use_kb": true, "use_web": false, "idempotency_key": key}
	code, v = call("POST", "/api/tasks", token, req)
	if code != 202 {
		t.Fatalf("create %d %v", code, v)
	}
	id := v["id"].(string)
	_, dup := call("POST", "/api/tasks", token, req)
	if dup["id"] != id {
		t.Fatal("duplicate request created another task")
	}
	_, bal := call("GET", "/api/billing", token, nil)
	if bal["available"].(float64) != before-20 {
		t.Fatalf("reservation wrong: %v", bal)
	}
	_, other := call("POST", "/api/auth/login", "", map[string]any{"username": "other", "password": "test-pass-123"})
	otherToken := other["token"].(string)
	code, _ = call("GET", "/api/tasks/"+id, otherToken, nil)
	if code != 404 {
		t.Fatal("cross tenant task visible", code)
	}
	for range 2 {
		code, _ = call("POST", "/api/tasks/"+id+"/cancel", token, map[string]any{})
		if code != 200 {
			t.Fatal("cancel failed")
		}
	}
	_, bal = call("GET", "/api/billing", token, nil)
	if bal["available"].(float64) != before {
		t.Fatal("release was not idempotent", bal)
	}
	_, order := call("POST", "/api/billing/orders", token, map[string]any{"sku": "topup"})
	oid := order["id"].(string)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, _ := call("POST", "/api/billing/orders/"+oid+"/pay", token, map[string]any{})
			if c != 200 {
				t.Error("payment", c)
			}
		}()
	}
	wg.Wait()
	_, bal = call("GET", "/api/billing", token, nil)
	if bal["available"].(float64) != before+100 {
		t.Fatal("duplicate payment granted points repeatedly", bal)
	}
	code, _ = call(http.MethodGet, "/api/admin/members", token, nil)
	if code != 403 {
		t.Fatal("teacher got admin access")
	}
}
