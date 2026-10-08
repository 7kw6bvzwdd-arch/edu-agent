package main

import (
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"strings"
)

func (s *Server) account(w http.ResponseWriter, r *http.Request, a Actor) error {
	var username, name string
	if err := s.db.QueryRow(r.Context(), "SELECT username,display_name FROM users WHERE id=$1 AND active", a.UserID).Scan(&username, &name); err != nil {
		return err
	}
	respond(w, 200, map[string]string{"username": username, "name": name})
	return nil
}
func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request, a Actor) error {
	var in struct {
		Name string `json:"name"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	in.Name = strings.TrimSpace(in.Name)
	if len([]rune(in.Name)) < 1 || len([]rune(in.Name)) > 30 {
		return fail(400, "显示名称需为1–30个字")
	}
	_, err := s.db.Exec(r.Context(), "UPDATE users SET display_name=$1 WHERE id=$2 AND active", in.Name, a.UserID)
	if err != nil {
		return err
	}
	respond(w, 200, map[string]string{"name": in.Name})
	return nil
}
func (s *Server) changePassword(w http.ResponseWriter, r *http.Request, a Actor) error {
	var in struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if len(in.New) < 10 || len(in.New) > 72 || strings.TrimSpace(in.New) == "" {
		return fail(400, "新密码需为10–72字节，不能全为空白")
	}
	if in.New == in.Current {
		return fail(400, "新密码不能与当前密码相同")
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
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Current)) != nil {
		return fail(400, "当前密码不正确")
	}
	fresh, err := bcrypt.GenerateFromPassword([]byte(in.New), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), "UPDATE users SET password_hash=$1 WHERE id=$2", string(fresh), a.UserID); err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), "DELETE FROM sessions WHERE user_id=$1", a.UserID); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	respond(w, 200, map[string]bool{"reauthenticate": true})
	return nil
}
