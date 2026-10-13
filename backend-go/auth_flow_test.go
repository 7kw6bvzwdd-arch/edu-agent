package main

import (
	"context"
	"strings"
	"testing"
)

func TestRegisterCreatesIsolatedSpaceAndRecovery(t *testing.T) {
	s := worksServer(t)
	name := "signup-" + newID()[:12]
	body := map[string]string{"username": name, "name": "新教师", "workspace": "独立教研空间", "password": "secure-pass-123"}
	v := worksObject(t, worksCall(t, s, "POST", "/api/auth/register", "", body, 201))
	code, ok := v["recovery_code"].(string)
	if !ok || len(code) < 32 {
		t.Fatal("registration must issue a recovery code once")
	}
	worksCall(t, s, "POST", "/api/auth/register", "", body, 409)
	login := worksObject(t, worksCall(t, s, "POST", "/api/auth/login", "", map[string]string{"username": name, "password": body["password"]}, 200))
	token := login["token"].(string)
	worksCall(t, s, "POST", "/api/auth/switch-tenant", token, map[string]int64{"tenant_id": 1}, 403)
	worksCall(t, s, "POST", "/api/account/recovery-code", "", map[string]string{"current_password": body["password"]}, 401)
	worksCall(t, s, "POST", "/api/account/recovery-code", token, map[string]string{"current_password": "wrong-password"}, 400)
	issued := worksObject(t, worksCall(t, s, "POST", "/api/account/recovery-code", token, map[string]string{"current_password": body["password"]}, 200))
	worksCall(t, s, "POST", "/api/auth/reset-password", "", map[string]string{"username": name, "recovery_code": code, "password": "new-secure-123"}, 400)
	code = issued["recovery_code"].(string)
	billing := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	if w := worksCall(t, s, "GET", "/api/tasks", token, nil, 200); w.Body.String() != "[]\n" {
		t.Fatal("new member inherited tasks", w.Body.String())
	}
	if billing["available"] != float64(0) || billing["plan"] != "" {
		t.Fatal("registration must not grant unpurchased points", billing)
	}
	teacher := worksLogin(t, s, "teacher")
	foreignTask := worksFixture(t, s, teacher, "courseware", "succeeded", "private-existing-work", TaskInput{})
	for _, suffix := range []string{"", "/download/md", "/download/pptx", "/events", "/revisions", "/conversation"} {
		worksCall(t, s, "GET", "/api/tasks/"+foreignTask+suffix, token, nil, 404)
	}
	ctx := context.Background()
	tx, err := s.tx(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	doc := newID()
	_, err = tx.Exec(ctx, `INSERT INTO documents(id,tenant_id,member_id,name,status) SELECT $1,1,id,'private-registration-fixture','ready' FROM members WHERE tenant_id=1 AND user_id=(SELECT id FROM users WHERE username='teacher')`, doc)
	if err == nil {
		_, err = tx.Exec(ctx, "INSERT INTO chunks(id,tenant_id,document_id,seq,content,location) VALUES($1,1,$2,1,$3,'1')", newID(), doc, "浮力私有教研注册隔离特征词")
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(worksCall(t, s, "GET", "/api/documents", teacher, nil, 200).Body.String(), doc) {
		t.Fatal("foreign document fixture missing")
	}
	if body := worksCall(t, s, "GET", "/api/documents", token, nil, 200).Body.String(); body != "[]\n" {
		t.Fatal("registration inherited enterprise documents")
	}
	if body := worksCall(t, s, "POST", "/api/retrieval/test", token, map[string]string{"query": "浮力私有教研注册隔离特征词"}, 200).Body.String(); strings.Contains(body, doc) || strings.Contains(body, "注册隔离特征词") {
		t.Fatal("registration can retrieve another enterprise's knowledge")
	}
	worksCall(t, s, "DELETE", "/api/documents/"+doc, token, nil, 404)
	worksCall(t, s, "POST", "/api/auth/reset-password", "", map[string]string{"username": name, "recovery_code": "invalid", "password": "new-secure-123"}, 400)
	fresh := worksObject(t, worksCall(t, s, "POST", "/api/auth/reset-password", "", map[string]string{"username": name, "recovery_code": code, "password": "new-secure-123"}, 200))
	if fresh["recovery_code"] == code {
		t.Fatal("recovery code must rotate")
	}
	worksCall(t, s, "GET", "/api/auth/me", token, nil, 401)
	worksCall(t, s, "POST", "/api/auth/login", "", map[string]string{"username": name, "password": body["password"]}, 401)
	worksCall(t, s, "POST", "/api/auth/login", "", map[string]string{"username": name, "password": "new-secure-123"}, 200)
	worksCall(t, s, "POST", "/api/auth/reset-password", "", map[string]string{"username": name, "recovery_code": code, "password": "another-pass-123"}, 400)
}
