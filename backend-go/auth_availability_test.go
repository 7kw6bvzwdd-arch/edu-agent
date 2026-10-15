package main

import (
	"context"
	"testing"
)

func TestAuthenticationOutageDoesNotInvalidateSession(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	s.db.Close()
	worksCall(t, s, "GET", "/api/auth/me", token, nil, 503)
	worksCall(t, s, "GET", "/api/billing", token, nil, 503)
	worksCall(t, s, "POST", "/api/auth/refresh", token, nil, 503)
	worksCall(t, s, "GET", "/api/auth/me", "invalid-token", nil, 401)
}

func TestAuthenticationRevokedSessionStillRequiresLogin(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	worksCall(t, s, "POST", "/api/auth/logout", token, nil, 200)
	worksCall(t, s, "GET", "/api/auth/me", token, nil, 401)
	if err := s.db.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}
