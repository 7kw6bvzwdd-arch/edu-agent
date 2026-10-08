package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
)

func TestMemberPackageAndConcurrentReservations(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	s, e := NewServer(context.Background(), Config{DatabaseURL: os.Getenv("TEST_DATABASE_URL"), RuntimeURL: os.Getenv("TEST_RUNTIME_DATABASE_URL"), Mode: "sample", Secret: "test-secret-at-least-thirty-two-bytes", SeedPassword: "test-pass-123", Storage: t.TempDir()})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	call := func(method, path, token string, body any) (int, map[string]any) {
		r := httptest.NewRequest(method, path, bytes.NewReader(asJSON(body)))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		var v map[string]any
		json.Unmarshal(w.Body.Bytes(), &v)
		return w.Code, v
	}
	_, v := call("POST", "/api/auth/login", "", map[string]string{"username": "admin", "password": "test-pass-123"})
	admin := v["token"].(string)
	username := "test-" + newID()[:12]
	code, v := call("POST", "/api/admin/members", admin, map[string]string{"username": username, "name": "新增成员验收", "password": "unique-test-password"})
	if code != 201 {
		t.Fatal(code, v)
	}
	mid := int64(v["id"].(float64))
	_, v = call("POST", "/api/auth/login", "", map[string]string{"username": username, "password": "unique-test-password"})
	token := v["token"].(string)
	request := func() TaskInput {
		return TaskInput{Type: "courseware", Topic: "浮力", UseKB: true, IdempotencyKey: newID()}
	}
	code, _ = call("POST", "/api/tasks", token, request())
	if code != 403 {
		t.Fatal("member without plan could generate")
	}
	key := newID()
	for range 2 {
		code, v = call("POST", "/api/admin/members/"+strconv.FormatInt(mid, 10)+"/package", admin, map[string]string{"sku": "basic", "idempotency_key": key})
		if code != 200 {
			t.Fatal(code, v)
		}
	}
	_, v = call("GET", "/api/billing", token, nil)
	if v["available"] != float64(500) || v["plan"] != "basic" {
		t.Fatal("package not idempotent", v)
	}
	var mu sync.Mutex
	ids := []string{}
	var wg sync.WaitGroup
	for range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, res := call("POST", "/api/tasks", token, request())
			if c == 202 {
				mu.Lock()
				ids = append(ids, res["id"].(string))
				mu.Unlock()
			} else if c != 402 {
				t.Error("unexpected reservation status", c, res)
			}
		}()
	}
	wg.Wait()
	if len(ids) != 25 {
		t.Fatal("concurrent reservations exceeded or lost available credits", len(ids))
	}
	_, v = call("GET", "/api/billing", token, nil)
	if v["reserved"] != float64(500) || v["available"] != float64(0) {
		t.Fatal("reservation balance", v)
	}
	for _, id := range ids {
		call("POST", "/api/tasks/"+id+"/cancel", token, map[string]any{})
	}
	_, v = call("GET", "/api/billing", token, nil)
	if v["available"] != float64(500) || v["reserved"] != float64(0) {
		t.Fatal("cancellation lost credits", v)
	}
}
