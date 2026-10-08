package main

import (
	"context"
	"strings"
	"testing"
)

func TestResetSeedPasswordHTTP(t *testing.T) {
	s := worksServer(t)
	oldToken := worksLogin(t, s, "teacher")
	adminToken := worksLogin(t, s, "admin")
	before := worksCall(t, s, "GET", "/api/billing", oldToken, nil, 200).Body.String()
	original := s.cfg.SeedPassword
	t.Cleanup(func() {
		if err := resetSeedPassword(context.Background(), s.cfg.DatabaseURL, "teacher", original); err != nil {
			t.Error(err)
		}
	})
	replacement := "new-local-test-password"
	if err := resetSeedPassword(context.Background(), s.cfg.DatabaseURL, "teacher", replacement); err != nil {
		t.Fatal(err)
	}
	worksCall(t, s, "POST", "/api/auth/login", "", map[string]string{"username": "teacher", "password": original}, 401)
	worksCall(t, s, "GET", "/api/auth/me", oldToken, nil, 401)
	worksCall(t, s, "GET", "/api/auth/me", adminToken, nil, 200)
	login := worksObject(t, worksCall(t, s, "POST", "/api/auth/login", "", map[string]string{"username": "teacher", "password": replacement}, 200))
	token := login["token"].(string)
	if after := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String(); after != before {
		t.Fatal("reset changed billing")
	}
	for _, c := range []struct{ name, password string }{{"teacher", ""}, {"teacher", strings.Repeat("a", 73)}, {"custom-user", replacement}} {
		if resetSeedPassword(context.Background(), s.cfg.DatabaseURL, c.name, c.password) == nil {
			t.Fatal("invalid local seed reset accepted")
		}
	}
	worksCall(t, s, "GET", "/api/auth/me", token, nil, 200)
	// Restart/bootstrap must preserve the explicitly reset password.
	restarted, err := NewServer(context.Background(), s.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	worksCall(t, restarted, "POST", "/api/auth/login", "", map[string]string{"username": "teacher", "password": replacement}, 200)
}
