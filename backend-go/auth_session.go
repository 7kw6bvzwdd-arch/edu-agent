package main

import (
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Refresh only an existing, still-valid session. Never recreate revoked sessions.
func (s *Server) refreshSession(w http.ResponseWriter, r *http.Request, a Actor) error {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), claims, func(*jwt.Token) (any, error) { return []byte(s.cfg.Secret), nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil {
		return fail(401, "登录已失效，请重新登录")
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		return fail(503, "登录验证服务暂不可用，请稍后重试")
	}
	defer tx.Rollback(r.Context())
	var refreshed bool
	err = tx.QueryRow(r.Context(), "SELECT auth_session_refresh($2,$3,$1)", time.Now().Add(12*time.Hour), claims.Session, a.UserID).Scan(&refreshed)
	if err != nil {
		return fail(503, "登录验证服务暂不可用，请稍后重试")
	}
	if !refreshed {
		return fail(401, "登录已失效，请重新登录")
	}

	token, err := s.token(a.UserID, a.TenantID, claims.Session)
	if err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return fail(503, "登录验证服务暂不可用，请稍后重试")
	}
	respond(w, 200, map[string]string{"token": token})
	return nil
}
