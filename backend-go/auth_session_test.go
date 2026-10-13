package main

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestActiveSessionRefreshHTTP(t *testing.T) {
	s := worksServer(t)
	original := worksLogin(t, s, "teacher")
	claims := &Claims{}
	if _, err := jwt.ParseWithClaims(original, claims, func(*jwt.Token) (any, error) { return []byte(s.cfg.Secret), nil }); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	// Simulate a working browser approaching the fixed twelve-hour deadline.
	expires := time.Now().Add(5 * time.Minute)
	claims.ExpiresAt = jwt.NewNumericDate(expires)
	old, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.Secret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(ctx, "UPDATE sessions SET expires_at=$1 WHERE id=$2", expires, claims.Session); err != nil {
		t.Fatal(err)
	}
	before := worksCall(t, s, "GET", "/api/auth/me", old, nil, 200).Body.String()
	refreshed := worksObject(t, worksCall(t, s, "POST", "/api/auth/refresh", old, nil, 200))
	fresh, ok := refreshed["token"].(string)
	if !ok || fresh == "" || fresh == old {
		t.Fatal("active session did not receive a renewed token")
	}
	renewed := &Claims{}
	if _, err = jwt.ParseWithClaims(fresh, renewed, func(*jwt.Token) (any, error) { return []byte(s.cfg.Secret), nil }); err != nil {
		t.Fatal(err)
	}
	if renewed.Session != claims.Session || renewed.UserID != claims.UserID || renewed.TenantID != claims.TenantID || time.Until(renewed.ExpiresAt.Time) < 11*time.Hour {
		t.Fatal("renewal changed identity or failed to extend expiry")
	}
	var databaseExpiry time.Time
	if err = s.db.QueryRow(ctx, "SELECT expires_at FROM sessions WHERE id=$1", claims.Session).Scan(&databaseExpiry); err != nil {
		t.Fatal(err)
	}
	if time.Until(databaseExpiry) < 11*time.Hour {
		t.Fatal("JWT renewed without renewing database session")
	}
	if got := worksCall(t, s, "GET", "/api/auth/me", fresh, nil, 200).Body.String(); got != before {
		t.Fatal("renewal changed actor")
	}
	worksCall(t, s, "GET", "/api/auth/me", old, nil, 200) // In-flight requests remain valid.
	worksCall(t, s, "POST", "/api/auth/logout", fresh, nil, 200)
	worksCall(t, s, "POST", "/api/auth/refresh", old, nil, 401)
	worksCall(t, s, "GET", "/api/auth/me", fresh, nil, 401)

	original = worksLogin(t, s, "teacher")
	if _, err = jwt.ParseWithClaims(original, claims, func(*jwt.Token) (any, error) { return []byte(s.cfg.Secret), nil }); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(ctx, "UPDATE sessions SET expires_at=now()-interval '1 second' WHERE id=$1", claims.Session); err != nil {
		t.Fatal(err)
	}
	worksCall(t, s, "POST", "/api/auth/refresh", original, nil, 401)
	worksCall(t, s, "POST", "/api/auth/refresh", "invalid-token", nil, 401)

	// A valid database session must not rescue an expired or tampered JWT.
	original = worksLogin(t, s, "teacher")
	if _, err = jwt.ParseWithClaims(original, claims, func(*jwt.Token) (any, error) { return []byte(s.cfg.Secret), nil }); err != nil {
		t.Fatal(err)
	}
	claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Second))
	expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.Secret))
	if err != nil {
		t.Fatal(err)
	}
	worksCall(t, s, "POST", "/api/auth/refresh", expired, nil, 401)
	claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Hour))
	tampered, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("another-test-key"))
	if err != nil {
		t.Fatal(err)
	}
	worksCall(t, s, "POST", "/api/auth/refresh", tampered, nil, 401)
	// Enterprise switching and renewal retain the selected tenant, never grant membership.
	admin := worksLogin(t, s, "admin")
	if _, err = jwt.ParseWithClaims(admin, claims, func(*jwt.Token) (any, error) { return []byte(s.cfg.Secret), nil }); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(ctx, "UPDATE sessions SET expires_at=now()+interval '5 minutes' WHERE id=$1", claims.Session); err != nil {
		t.Fatal(err)
	}
	switched := worksObject(t, worksCall(t, s, "POST", "/api/auth/switch-tenant", admin, map[string]int64{"tenant_id": 2}, 200))["token"].(string)
	if err = s.db.QueryRow(ctx, "SELECT expires_at FROM sessions WHERE id=$1", claims.Session).Scan(&databaseExpiry); err != nil {
		t.Fatal(err)
	}
	if time.Until(databaseExpiry) < 11*time.Hour {
		t.Fatal("enterprise switch renewed JWT but left database session about to expire")
	}
	fresh = worksObject(t, worksCall(t, s, "POST", "/api/auth/refresh", switched, nil, 200))["token"].(string)
	actor := worksObject(t, worksCall(t, s, "GET", "/api/auth/me", fresh, nil, 200))
	if actor["tenant_id"] != float64(2) {
		t.Fatal("refresh changed the selected enterprise")
	}
	worksCall(t, s, "POST", "/api/auth/switch-tenant", original, map[string]int64{"tenant_id": 2}, 403)

	// A disabled membership still blocks renewal through the shared auth path.
	other := worksLogin(t, s, "other")
	tx, err := s.tx(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "UPDATE members SET active=false WHERE user_id=(SELECT id FROM users WHERE username='other')"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		tx, err := s.tx(ctx, 2)
		if err != nil {
			t.Error(err)
			return
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, "UPDATE members SET active=true WHERE user_id=(SELECT id FROM users WHERE username='other')"); err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			t.Error(err)
		}
	})
	worksCall(t, s, "POST", "/api/auth/refresh", other, nil, 403)
}
