package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// Shared PostgreSQL counters also cover multiple backend processes. Trust only
// RemoteAddr, never an arbitrary forwarded header. No passwords enter this key.
func (s *Server) authAttempt(r *http.Request, scope, username string) error {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	values := []string{"ip:" + ip, "account:" + strings.ToLower(username)}
	limits := []int{20, 20}
	if scope == "login" || scope == "password" {
		values = []string{"ip:" + ip, "account:" + strings.ToLower(username) + ":" + ip}
		limits = []int{120, 12}
	}
	for i, value := range values {
		key := recoveryDigest(scope + ":" + value)
		var n int
		err := s.db.QueryRow(r.Context(), `INSERT INTO auth_attempts(key,attempts,expires_at) VALUES($1,1,now()+interval '15 minutes')
   ON CONFLICT(key) DO UPDATE SET attempts=CASE WHEN auth_attempts.expires_at<=now() THEN 1 ELSE auth_attempts.attempts+1 END,
   expires_at=CASE WHEN auth_attempts.expires_at<=now() THEN now()+interval '15 minutes' ELSE auth_attempts.expires_at END RETURNING attempts`, key).Scan(&n)
		if err != nil {
			return err
		}
		if n > limits[i] {
			return fail(429, "尝试次数过多，请15分钟后重试")
		}
	}
	// Retain only active windows; the table is not an authentication history.
	_, err = s.db.Exec(r.Context(), "DELETE FROM auth_attempts WHERE expires_at<now()")
	return err
}
func recoveryDigest(code string) string {
	h := sha256.Sum256([]byte(code))
	return hex.EncodeToString(h[:])
}
func validNewPassword(p string) bool {
	return len(p) >= 10 && len(p) <= 72 && strings.TrimSpace(p) != ""
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "no-store")
	var in struct{ Username, Name, Workspace, Password string }
	if err := decode(r, &in); err != nil {
		return err
	}
	in.Username = strings.TrimSpace(in.Username)
	in.Name = strings.TrimSpace(in.Name)
	in.Workspace = strings.TrimSpace(in.Workspace)
	if !accountName.MatchString(in.Username) || !validNewPassword(in.Password) || len([]rune(in.Name)) < 1 || len([]rune(in.Name)) > 30 || len([]rune(in.Workspace)) < 1 || len([]rune(in.Workspace)) > 60 {
		return fail(400, "账号3–40位字母数字，密码10–72字节，名称1–30字，空间名称1–60字")
	}
	if err := s.authAttempt(r, "register", in.Username); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	code := newID() + newID()
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	var uid, tid int64
	err = tx.QueryRow(r.Context(), "SELECT id FROM (SELECT auth_create_user($1,$2,$3,$4) AS id) created WHERE id IS NOT NULL", in.Username, string(hash), in.Name, recoveryDigest(code)).Scan(&uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return fail(409, "账号已存在，请登录或使用其他账号")
	}
	if err != nil {
		return err
	}
	if err = tx.QueryRow(r.Context(), "INSERT INTO tenants(name) VALUES($1) RETURNING id", in.Workspace).Scan(&tid); err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), "SELECT set_config('app.tenant_id',$1,true)", strconv.FormatInt(tid, 10)); err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), "INSERT INTO members(tenant_id,user_id,role,permissions) VALUES($1,$2,'admin',$3)", tid, uid, asJSON([]string{"courseware.generate", "animation.render", "kb.manage"})); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	respond(w, 201, map[string]string{"recovery_code": code, "message": "注册成功，请保存一次性恢复码，再登录并开通基础套餐"})
	return nil
}

func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "no-store")
	var in struct {
		Username string `json:"username"`
		Code     string `json:"recovery_code"`
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	in.Username = strings.TrimSpace(in.Username)
	in.Code = strings.TrimSpace(in.Code)
	if !accountName.MatchString(in.Username) || !validNewPassword(in.Password) || len(in.Code) > 128 {
		return fail(400, "请检查账号、恢复码及新密码（10–72字节）")
	}
	if err := s.authAttempt(r, "reset", in.Username); err != nil {
		return err
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	code := newID() + newID()
	var changed bool
	if err = tx.QueryRow(r.Context(), "SELECT auth_reset_password($1,$2,$3,$4)", in.Username, recoveryDigest(in.Code), string(hash), recoveryDigest(code)).Scan(&changed); err != nil {
		return err
	}

	if !changed {
		return fail(400, "账号或凭据无效")
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	respond(w, 200, map[string]string{"recovery_code": code, "message": "密码已更新，旧会话和旧恢复码已失效"})
	return nil
}

func (s *Server) issueRecoveryCode(w http.ResponseWriter, r *http.Request, a Actor) error {
	w.Header().Set("Cache-Control", "no-store")
	var in struct {
		Password string `json:"current_password"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if len(in.Password) > 72 {
		return fail(400, "当前密码不正确")
	}
	if err := s.authAttempt(r, "recovery", strconv.FormatInt(a.UserID, 10)); err != nil {
		return err
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	code := newID() + newID()
	var changed bool
	if err = tx.QueryRow(r.Context(), "SELECT auth_issue_recovery($1,$2,$3)", a.UserID, in.Password, recoveryDigest(code)).Scan(&changed); err != nil {
		return err
	}
	if !changed {
		return fail(400, "账号或凭据无效")
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	respond(w, 200, map[string]string{"recovery_code": code})
	return nil
}

// A fixed cost-10 hash keeps missing/inactive users on the same bcrypt path.
const dummyPasswordHash = "$2a$10$7EqJtq98hPqEX7fNZaFWoO5uHNwyP1g3qJWt/2y3N7sG6uEp9XKgG"

func (s *Server) clearLoginAttempts(r *http.Request, username string) error {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	_, err = s.db.Exec(r.Context(), "DELETE FROM auth_attempts WHERE key=$1", recoveryDigest("login:account:"+strings.ToLower(username)+":"+ip))
	return err
}
