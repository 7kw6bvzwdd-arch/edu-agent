package main

import (
	"net/http"
	"regexp"
	"strconv"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

var accountName = regexp.MustCompile(`^[a-zA-Z0-9_.-]{3,40}$`)

func (s *Server) createMember(w http.ResponseWriter, r *http.Request, a Actor) error {
	var in struct {
		Username string `json:"username"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if e := decode(r, &in); e != nil {
		return e
	}
	if !accountName.MatchString(in.Username) || len(in.Password) < 10 || len(in.Password) > 72 || len([]rune(in.Name)) < 1 || len([]rune(in.Name)) > 30 {
		return fail(400, "账号需3–40位字母数字，密码10–72字节，姓名1–30字")
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if e != nil {
		return e
	}
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var uid, mid int64
	e = tx.QueryRow(r.Context(), "INSERT INTO users(username,password_hash,display_name) VALUES($1,$2,$3) ON CONFLICT DO NOTHING RETURNING id", in.Username, string(hash), in.Name).Scan(&uid)
	if e == pgx.ErrNoRows {
		return fail(409, "账号已存在，请使用新的成员账号")
	}
	if e != nil {
		return e
	}
	e = tx.QueryRow(r.Context(), "INSERT INTO members(tenant_id,user_id,role,permissions) VALUES($1,$2,'teacher',$3) RETURNING id", a.TenantID, uid, asJSON([]string{"courseware.generate", "animation.render"})).Scan(&mid)
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), "INSERT INTO audit_logs(tenant_id,member_id,action,target) VALUES($1,$2,'member.create',$3)", a.TenantID, a.MemberID, strconv.FormatInt(mid, 10))
	if e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	respond(w, 201, map[string]int64{"id": mid})
	return nil
}
func (s *Server) assignPackage(w http.ResponseWriter, r *http.Request, a Actor) error {
	var in struct {
		SKU string `json:"sku"`
		Key string `json:"idempotency_key"`
	}
	if e := decode(r, &in); e != nil {
		return e
	}
	if len(in.Key) < 8 || len(in.Key) > 128 {
		return fail(400, "分配请求需要有效幂等键")
	}
	target, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if e != nil {
		return fail(400, "成员ID无效")
	}
	var p Product
	for _, product := range products {
		if product.ID == in.SKU {
			p = product
		}
	}
	if p.ID == "" {
		return fail(400, "无效套餐")
	}
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var balance int64
	var plan string
	e = tx.QueryRow(r.Context(), "SELECT balance,plan FROM members WHERE id=$1 AND active FOR UPDATE", target).Scan(&balance, &plan)
	if e != nil {
		return e
	}
	var existing bool
	reference := "admin:" + in.Key
	e = tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM ledger WHERE member_id=$1 AND kind='purchase' AND reference=$2)", target, reference).Scan(&existing)
	if e != nil {
		return e
	}
	if existing {
		respond(w, 200, map[string]bool{"ok": true})
		return nil
	}
	if p.ID == "topup" && plan == "" {
		return fail(400, "成员需先开通基础套餐")
	}
	e = tx.QueryRow(r.Context(), "UPDATE members SET balance=balance+$1,plan=CASE WHEN $2='basic' THEN 'basic' ELSE plan END WHERE id=$3 RETURNING balance", p.Points, p.ID, target).Scan(&balance)
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), "INSERT INTO ledger(tenant_id,member_id,kind,delta,balance_after,reference) VALUES($1,$2,'purchase',$3,$4,$5)", a.TenantID, target, p.Points, balance, reference)
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), "INSERT INTO orders(id,tenant_id,member_id,sku,points,amount_cents,status,paid_at) VALUES($1,$2,$3,$4,$5,0,'paid',now())", newID(), a.TenantID, target, p.ID, p.Points)
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), "INSERT INTO audit_logs(tenant_id,member_id,action,target,detail) VALUES($1,$2,'package.assign',$3,$4)", a.TenantID, a.MemberID, strconv.FormatInt(target, 10), asJSON(map[string]any{"sku": p.ID, "points": p.Points, "reference": reference, "simulated": true}))
	if e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	respond(w, 200, map[string]bool{"ok": true})
	return nil
}
