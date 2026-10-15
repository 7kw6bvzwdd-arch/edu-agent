package main

import (
	"crypto/sha256"
	"crypto/subtle"
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
	for _, value := range []string{"ip:" + ip, "account:" + strings.ToLower(username)} {
		key := recoveryDigest(scope + ":" + value)
		var n int
		err := s.db.QueryRow(r.Context(), `INSERT INTO auth_attempts(key,attempts,expires_at) VALUES($1,1,now()+interval '15 minutes')
   ON CONFLICT(key) DO UPDATE SET attempts=CASE WHEN auth_attempts.expires_at<=now() THEN 1 ELSE auth_attempts.attempts+1 END,
   expires_at=CASE WHEN auth_attempts.expires_at<=now() THEN now()+interval '15 minutes' ELSE auth_attempts.expires_at END RETURNING attempts`, key).Scan(&n)
		if err != nil {
			return err
		}
		if n > 20 {
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
	err = tx.QueryRow(r.Context(), "INSERT INTO users(username,password_hash,display_name,recovery_hash) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING RETURNING id", in.Username, string(hash), in.Name, recoveryDigest(code)).Scan(&uid)
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
	var uid int64
	var stored string
	err = tx.QueryRow(r.Context(), "SELECT id,recovery_hash FROM users WHERE username=$1 AND active FOR UPDATE", in.Username).Scan(&uid, &stored)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err != nil || stored == "" || subtle.ConstantTimeCompare([]byte(stored), []byte(recoveryDigest(in.Code))) != 1 {
		return fail(400, "账号或恢复码无效；无法使用恢复码时请联系管理员")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	code := newID() + newID()
	if _, err = tx.Exec(r.Context(), "UPDATE users SET password_hash=$1,recovery_hash=$2 WHERE id=$3", string(hash), recoveryDigest(code), uid); err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), "DELETE FROM sessions WHERE user_id=$1", uid); err != nil {
		return err
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
	var hash string
	if err = tx.QueryRow(r.Context(), "SELECT password_hash FROM users WHERE id=$1 AND active FOR UPDATE", a.UserID).Scan(&hash); err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		return fail(400, "当前密码不正确")
	}
	code := newID() + newID()
	if _, err = tx.Exec(r.Context(), "UPDATE users SET recovery_hash=$1 WHERE id=$2", recoveryDigest(code), a.UserID); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	respond(w, 200, map[string]string{"recovery_code": code})
	return nil
}
