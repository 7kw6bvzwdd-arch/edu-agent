package main

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
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
	// Password changes lock this same user row before deleting sessions.
	var active bool
	err = tx.QueryRow(r.Context(), "SELECT active FROM users WHERE id=$1 FOR SHARE", a.UserID).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !active {
		return fail(401, "登录已失效，请重新登录")
	}
	if err != nil {
		return fail(503, "登录验证服务暂不可用，请稍后重试")
	}
	result, err := tx.Exec(r.Context(), "UPDATE sessions SET expires_at=GREATEST(expires_at,$1) WHERE id=$2 AND user_id=$3 AND expires_at>now()", time.Now().Add(12*time.Hour), claims.Session, a.UserID)
	if err != nil {
		return fail(503, "登录验证服务暂不可用，请稍后重试")
	}
	if result.RowsAffected() != 1 {
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
