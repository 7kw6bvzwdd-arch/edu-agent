package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestPublicPageBoundaries(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.1.2.3", "192.168.1.2", "169.254.169.254", "100.64.0.1", "::1", "::ffff:127.0.0.1", "fc00::1", "2001:db8::1"} {
		if publicIP(net.ParseIP(ip)) {
			t.Errorf("accepted non-public IP %s", ip)
		}
	}
	if !publicIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("public address rejected")
	}
	for _, u := range []string{"file:///etc/passwd", "http://user:password@example.com", "https://example.com:8080"} {
		if _, e := validateURL(u); e == nil {
			t.Errorf("unsafe URL accepted: %s", u)
		}
	}
	if _, _, e := fetchPage(context.Background(), "http://127.0.0.1"); e == nil {
		t.Fatal("loopback page accepted")
	}
}

func TestCoursewareHTTPWorkflow(t *testing.T) {
	db := os.Getenv("TEST_DATABASE_URL")
	if db == "" {
		t.Skip("set isolated TEST_DATABASE_URL")
	}
	renderer := os.Getenv("TEST_RENDER_URL")
	if renderer == "" {
		t.Skip("set TEST_RENDER_URL for real PPTX integration")
	}
	config := Config{DatabaseURL: db, Secret: "test-secret-at-least-thirty-two-bytes", Mode: "sample", RuntimeURL: os.Getenv("TEST_RUNTIME_DATABASE_URL"), SeedPassword: "test-pass-123", Storage: t.TempDir(), RenderURL: renderer}
	if os.Getenv("TEST_CLOUD") == "1" {
		config = cloudTestConfig(t)
	}
	s, e := NewServer(context.Background(), config)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	call := func(method, path, token string, body any) (int, map[string]any, []byte) {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewReader(asJSON(body)))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, req)
		b, _ := io.ReadAll(w.Result().Body)
		var v map[string]any
		json.Unmarshal(b, &v)
		return w.Code, v, b
	}
	_, v, _ := call("POST", "/api/auth/login", "", map[string]string{"username": "teacher", "password": "test-pass-123"})
	token := v["token"].(string)
	_, bal, _ := call("GET", "/api/billing", token, nil)
	before := bal["available"].(float64)
	code, v, _ := call("POST", "/api/tasks", token, TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, UseKB: true, Theme: "forest", IdempotencyKey: newID()})
	if code != 202 {
		t.Fatal(code, v)
	}
	id := v["id"].(string)
	if e = s.workOne(context.Background()); e != nil {
		t.Fatal(e)
	}
	_, v, _ = call("GET", "/api/tasks/"+id, token, nil)
	if v["status"] != "awaiting_confirmation" {
		t.Fatalf("research result: %v", v)
	}
	result := v["result"].(map[string]any)
	revision := v["revision"]
	code, _, _ = call("GET", "/api/tasks/"+id+"/download/pptx", token, nil)
	if code != 404 {
		t.Fatal("PPTX available before teacher confirmation")
	}
	code, _, _ = call("POST", "/api/tasks/"+id+"/confirm", token, map[string]any{"result": result, "revision": revision, "accept_review": false})
	if code != 400 {
		t.Fatal("unverified content accepted without consent")
	}
	code, v, _ = call("POST", "/api/tasks/"+id+"/confirm", token, map[string]any{"result": result, "revision": revision, "accept_review": true})
	if code != 202 {
		t.Fatal(code, v)
	}
	if e = s.workOne(context.Background()); e != nil {
		t.Fatal(e)
	}
	_, v, _ = call("GET", "/api/tasks/"+id, token, nil)
	if v["status"] != "succeeded" {
		t.Fatal(v)
	}
	revision = v["revision"]
	result = v["result"].(map[string]any)
	if s.cloud.objects != nil {
		if err := os.Remove(storagePath(s, 1, id, ".pptx")); err != nil {
			t.Fatal(err)
		}
	}
	code, _, file := call("GET", "/api/tasks/"+id+"/download/pptx", token, nil)
	if code != 200 || len(file) < 1000 || string(file[:2]) != "PK" {
		t.Fatal("not a real PPTX")
	}
	_, bal, _ = call("GET", "/api/billing", token, nil)
	if bal["available"].(float64) != before-20 || bal["reserved"].(float64) != 0 {
		t.Fatal("settlement incorrect", bal)
	}
	result["theme"] = "ocean"
	result["lesson"] = "教师手工修改教案"
	code, v, _ = call("POST", "/api/tasks/"+id+"/edit", token, map[string]any{"result": result, "revision": revision})
	if code != 200 {
		t.Fatal("free edit", code, v)
	}
	_, bal, _ = call("GET", "/api/billing", token, nil)
	if bal["available"].(float64) != before-20 {
		t.Fatal("manual edit charged")
	}
	_, v, _ = call("GET", "/api/tasks/"+id, token, nil)
	revision = v["revision"]
	code, v, _ = call("POST", "/api/tasks/"+id+"/rewrite", token, map[string]any{"instruction": "增加生活情境", "slide": 0, "revision": revision, "idempotency_key": newID()})
	if code != 202 {
		t.Fatal(code, v)
	}
	rewriteID := v["id"].(string)
	if e = s.workOne(context.Background()); e != nil {
		t.Fatal(e)
	}
	_, v, _ = call("GET", "/api/tasks/"+rewriteID, token, nil)
	if v["status"] != "succeeded" {
		t.Fatal("rewrite", v)
	}
	_, bal, _ = call("GET", "/api/billing", token, nil)
	if bal["available"].(float64) != before-25 {
		t.Fatal("rewrite charge", bal)
	}
	_, admin, _ := call("POST", "/api/auth/login", "", map[string]string{"username": "admin", "password": "test-pass-123"})
	code, _, _ = call("GET", "/api/tasks/"+id, admin["token"].(string), nil)
	if code != 404 {
		t.Fatal("admin could read private member output")
	}
	// A cancelled task's late worker result must not resurrect it or settle the reservation.
	code, v, _ = call("POST", "/api/tasks", token, TaskInput{Type: "courseware", Topic: "勾股定理", UseKB: true, IdempotencyKey: newID()})
	if code != 202 {
		t.Fatal(code, v)
	}
	raceID := v["id"].(string)
	_, actor, _ := call("GET", "/api/auth/me", token, nil)
	tid := int64(actor["tenant_id"].(float64))
	var wg sync.WaitGroup
	for _, status := range []string{"cancelled", "succeeded"} {
		wg.Add(1)
		go func(st string) {
			defer wg.Done()
			if e := s.finish(context.Background(), tid, raceID, st, "race", "", nil); e != nil {
				t.Error(e)
			}
		}(status)
	}
	wg.Wait()
	_, v, _ = call("GET", "/api/tasks/"+raceID, token, nil)
	charge := v["charge_state"]
	if charge != "settled" && charge != "released" {
		t.Fatal("missing terminal state")
	}
	oldRevision := v["revision"]
	if e = s.finish(context.Background(), tid, raceID, "succeeded", "late", "", nil); e != nil {
		t.Fatal(e)
	}
	_, v, _ = call("GET", "/api/tasks/"+raceID, token, nil)
	if v["charge_state"] != charge || v["revision"] != oldRevision {
		t.Fatal("late finish changed terminal task")
	}
	_, low, _ := call("POST", "/api/auth/login", "", map[string]string{"username": "low", "password": "test-pass-123"})
	code, _, _ = call("POST", "/api/tasks", low["token"].(string), TaskInput{Type: "courseware", Topic: "浮力", UseKB: true, IdempotencyKey: newID()})
	if code != 402 {
		t.Fatal("insufficient quota accepted")
	}
	for _, topic := range []string{"浮力", "勾股定理", "一次函数", "欧姆定律", "光的反射"} {
		d := sampleDeck(TaskInput{Topic: topic, Pages: 8, Theme: "forest"})
		if len(d.Slides) != 8 || !strings.Contains(d.Lesson, topic) {
			t.Fatal("invalid sample", topic)
		}
	}
}
