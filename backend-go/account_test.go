package main

import (
	"context"
	"strings"
	"testing"
)

func TestAccountSettingsHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "other")
	second := worksLogin(t, s, "teacher")
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	t.Cleanup(func() {
		_, err := s.db.Exec(context.Background(), "UPDATE users SET display_name='林老师' WHERE username='teacher'")
		if err != nil {
			t.Error(err)
		}
		if err := resetSeedPassword(context.Background(), s.cfg.DatabaseURL, "teacher", "test-pass-123"); err != nil {
			t.Error(err)
		}
	})
	worksCall(t, s, "PATCH", "/api/account", "", map[string]any{"name": "老师"}, 401)
	worksCall(t, s, "PATCH", "/api/account", token, map[string]any{"name": " "}, 400)
	worksCall(t, s, "PATCH", "/api/account", token, map[string]any{"name": strings.Repeat("字", 31)}, 400)
	worksCall(t, s, "PATCH", "/api/account", token, map[string]any{"name": "新老师"}, 200)
	me := worksObject(t, worksCall(t, s, "GET", "/api/auth/me", token, nil, 200))
	if me["name"] != "新老师" {
		t.Fatal("name not persisted")
	}
	account := worksObject(t, worksCall(t, s, "GET", "/api/account", token, nil, 200))
	if account["username"] != "teacher" || strings.Contains(string(asJSON(account)), "password") {
		t.Fatal("account profile invalid")
	}
	worksCall(t, s, "POST", "/api/account/password", token, map[string]string{"current_password": "wrong-password", "new_password": "new-test-password"}, 400)
	worksCall(t, s, "POST", "/api/account/password", token, map[string]string{"current_password": "test-pass-123", "new_password": "short"}, 400)
	worksCall(t, s, "POST", "/api/account/password", token, map[string]string{"current_password": "test-pass-123", "new_password": strings.Repeat("字", 25)}, 400)
	worksCall(t, s, "POST", "/api/account/password", token, map[string]string{"current_password": "test-pass-123", "new_password": "new-test-password"}, 200)
	for _, old := range []string{token, second} {
		worksCall(t, s, "GET", "/api/auth/me", old, nil, 401)
	}
	worksCall(t, s, "GET", "/api/auth/me", other, nil, 200)
	worksCall(t, s, "POST", "/api/auth/login", "", map[string]string{"username": "teacher", "password": "test-pass-123"}, 401)
	v := worksObject(t, worksCall(t, s, "POST", "/api/auth/login", "", map[string]string{"username": "teacher", "password": "new-test-password"}, 200))
	if after := worksCall(t, s, "GET", "/api/billing", v["token"].(string), nil, 200).Body.String(); after != before {
		t.Fatal("account change affected billing")
	}
}
