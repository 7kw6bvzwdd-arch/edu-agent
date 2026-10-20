package main

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
)

type Product struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Points int64  `json:"points"`
	Amount int64  `json:"amount_cents"`
}

var products = []Product{{"basic", "基础套餐", 500, 4900}, {"topup", "积分加量包", 100, 900}}

func (s *Server) billing(w http.ResponseWriter, r *http.Request, a Actor) error {
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var b, res int64
	var plan string
	e = tx.QueryRow(r.Context(), "SELECT balance,reserved,plan FROM members WHERE id=$1", a.MemberID).Scan(&b, &res, &plan)
	if e != nil {
		return e
	}
	ledger, e := queryMaps(r.Context(), tx, "SELECT id,kind,delta,balance_after,reference,created_at FROM ledger WHERE member_id=$1 ORDER BY id DESC LIMIT 100", a.MemberID)
	if e != nil {
		return e
	}
	orders, e := queryMaps(r.Context(), tx, "SELECT * FROM orders WHERE member_id=$1 ORDER BY created_at DESC LIMIT 50", a.MemberID)
	if e != nil {
		return e
	}
	respond(w, 200, map[string]any{"plan": plan, "balance": b, "reserved": res, "available": b - res, "products": products, "simulated_payment_enabled": s.cfg.Mode != "live" || s.cfg.AllowSimulatedPayment, "ledger": ledger, "orders": orders})
	return nil
}
func queryMaps(ctx context.Context, tx pgx.Tx, sql string, args ...any) ([]map[string]any, error) {
	rows, e := tx.Query(ctx, sql, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []map[string]any{}
	fields := rows.FieldDescriptions()
	for rows.Next() {
		v, e := rows.Values()
		if e != nil {
			return nil, e
		}
		m := map[string]any{}
		for i, f := range fields {
			m[f.Name] = v[i]
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (s *Server) createOrder(w http.ResponseWriter, r *http.Request, a Actor) error {
	var in struct {
		SKU string `json:"sku"`
	}
	if e := decode(r, &in); e != nil {
		return e
	}
	var product Product
	for _, p := range products {
		if p.ID == in.SKU {
			product = p
		}
	}
	if product.ID == "" {
		return fail(400, "无效套餐")
	}
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	id := newID()
	_, e = tx.Exec(r.Context(), "INSERT INTO orders(id,tenant_id,member_id,sku,points,amount_cents) VALUES($1,$2,$3,$4,$5,$6)", id, a.TenantID, a.MemberID, product.ID, product.Points, product.Amount)
	if e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	respond(w, 201, map[string]any{"id": id, "product": product})
	return nil
}
func (s *Server) payOrder(w http.ResponseWriter, r *http.Request, a Actor) error {
	if s.cfg.Mode == "live" && !s.cfg.AllowSimulatedPayment {
		return fail(403, "真实模型环境已关闭自助模拟入账，请联系管理员分配演示额度")
	}
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var status, sku string
	var points int64
	e = tx.QueryRow(r.Context(), "SELECT status,points,sku FROM orders WHERE id=$1 AND member_id=$2 FOR UPDATE", r.PathValue("id"), a.MemberID).Scan(&status, &points, &sku)
	if e != nil {
		return e
	}
	if status == "pending" {
		var plan string
		if e = tx.QueryRow(r.Context(), "SELECT plan FROM members WHERE id=$1 FOR UPDATE", a.MemberID).Scan(&plan); e != nil {
			return e
		}
		if sku == "topup" && plan == "" {
			return fail(400, "请先开通基础套餐")
		}

		var balance int64
		e = tx.QueryRow(r.Context(), "UPDATE members SET balance=balance+$1,plan=CASE WHEN $3='basic' THEN 'basic' ELSE plan END WHERE id=$2 RETURNING balance", points, a.MemberID, sku).Scan(&balance)
		if e != nil {
			return e
		}
		_, e = tx.Exec(r.Context(), "UPDATE orders SET status='paid',paid_at=now() WHERE id=$1", r.PathValue("id"))
		if e != nil {
			return e
		}
		_, e = tx.Exec(r.Context(), "INSERT INTO ledger(tenant_id,member_id,kind,delta,balance_after,reference) VALUES($1,$2,'purchase',$3,$4,$5)", a.TenantID, a.MemberID, points, balance, r.PathValue("id"))
		if e != nil {
			return e
		}
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	respond(w, 200, map[string]any{"status": "paid", "simulated": true})
	return nil
}
func (s *Server) members(w http.ResponseWriter, r *http.Request, a Actor) error {
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	v, e := queryMaps(r.Context(), tx, "SELECT m.id,u.username,u.display_name,m.role,m.permissions,m.balance,m.reserved,m.active FROM members m JOIN users u ON u.id=m.user_id ORDER BY m.id")
	if e != nil {
		return e
	}
	respond(w, 200, v)
	return nil
}
func (s *Server) updateMember(w http.ResponseWriter, r *http.Request, a Actor) error {
	id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if e != nil {
		return fail(400, "成员ID错误")
	}
	if id == a.MemberID {
		return fail(400, "不能通过此操作修改自己的管理员权限")
	}
	var in struct {
		Active      bool     `json:"active"`
		Permissions []string `json:"permissions"`
	}
	if e = decode(r, &in); e != nil {
		return e
	}
	valid := map[string]bool{"courseware.generate": true, "animation.render": true, "kb.manage": true, "log.view": true, "user.manage": true, "billing.manage": true, "eval.run": true}
	for _, p := range in.Permissions {
		if !valid[p] {
			return fail(400, "未知权限")
		}
	}
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	res, e := tx.Exec(r.Context(), "UPDATE members SET active=$1,permissions=$2 WHERE id=$3 AND role<>'admin'", in.Active, asJSON(in.Permissions), id)
	if e != nil {
		return e
	}
	if res.RowsAffected() == 0 {
		return fail(404, "成员不存在或为管理员")
	}
	_, e = tx.Exec(r.Context(), "INSERT INTO audit_logs(tenant_id,member_id,action,target,detail) VALUES($1,$2,'member.update',$3,$4)", a.TenantID, a.MemberID, strconv.FormatInt(id, 10), asJSON(in))
	if e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	respond(w, 200, map[string]bool{"ok": true})
	return nil
}
func (s *Server) dashboard(w http.ResponseWriter, r *http.Request, a Actor) error {
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var member any = a.MemberID
	if allowed(a, "log.view") {
		member = nil
	}
	tasks, e := queryMaps(r.Context(), tx, "SELECT status,count(*)::integer AS count FROM tasks WHERE deleted_at IS NULL AND ($1::bigint IS NULL OR member_id=$1) GROUP BY status", member)
	if e != nil {
		return e
	}
	trend, e := queryMaps(r.Context(), tx, "SELECT substring(period FROM 3) AS day,sum(calls)::bigint AS count,sum(input_tokens+output_tokens)::bigint AS tokens FROM usage_totals WHERE ($1::bigint IS NULL OR member_id=$1) AND period LIKE 'd:%' AND period>='d:'||to_char(now()-interval '30 days','YYYY-MM-DD') GROUP BY period ORDER BY period", member)
	if e != nil {
		return e
	}
	logs, e := queryMaps(r.Context(), tx, "SELECT id::text AS id,operation,model,input_tokens,output_tokens,latency_ms,success,created_at FROM usage_logs WHERE ($1::bigint IS NULL OR member_id=$1) ORDER BY created_at DESC,id DESC LIMIT 50", member)
	if e != nil {
		return e
	}
	var p95 float64
	if e = tx.QueryRow(r.Context(), "SELECT coalesce(percentile_cont(.95) WITHIN GROUP(ORDER BY latency_ms),0) FROM usage_logs WHERE ($1::bigint IS NULL OR member_id=$1)", member).Scan(&p95); e != nil {
		return e
	}
	totals, e := queryMaps(r.Context(), tx, "SELECT coalesce(sum(calls),0)::bigint AS calls,coalesce(sum(input_tokens),0)::bigint AS input_tokens,coalesce(sum(output_tokens),0)::bigint AS output_tokens,coalesce(sum(failures),0)::bigint AS failures FROM usage_totals WHERE period='all' AND ($1::bigint IS NULL OR member_id=$1)", member)
	if e != nil {
		return e
	}
	respond(w, 200, map[string]any{"tasks": tasks, "trend": trend, "usage": logs, "totals": totals[0], "p95_ms": p95, "p95_scope": "当前保留的调用明细", "mode": s.cfg.Mode})
	return nil
}
func event(ctx context.Context, tx pgx.Tx, tid int64, id, stage, message string) error {
	_, e := tx.Exec(ctx, "INSERT INTO task_events(tenant_id,task_id,stage,message) VALUES($1,$2,$3,$4)", tid, id, stage, message)
	return e
}
func (s *Server) finish(ctx context.Context, tid int64, id, status, message, path string, result any) error {
	tx, e := s.tx(ctx, tid)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var member, cost int64
	var charge string
	var deleted bool
	var cycle, attempt int
	e = tx.QueryRow(ctx, "SELECT member_id,cost,charge_state,deleted_at IS NOT NULL,charge_cycle,attempt FROM tasks WHERE id=$1 FOR UPDATE", id).Scan(&member, &cost, &charge, &deleted, &cycle, &attempt)
	if e != nil {
		return e
	}
	if expected, ok := generationCycle(ctx).(int); ok && expected != cycle {
		return nil
	}
	if expected, ok := generationAttempt(ctx).(int); ok && expected != attempt {
		return nil
	}
	if deleted || charge != "reserved" {
		return nil
	}
	settled := status == "succeeded" || status == "degraded"
	if settled {
		var taskType string
		var input TaskInput
		if e = tx.QueryRow(ctx, "SELECT type,input FROM tasks WHERE id=$1", id).Scan(&taskType, &input); e != nil {
			return e
		}
		if taskType == "courseware" {
			var mode string
			var planned Deck
			if e = tx.QueryRow(ctx, "SELECT mode,result FROM tasks WHERE id=$1", id).Scan(&mode, &planned); e != nil {
				return e
			}
			final, ok := result.(Deck)
			if planned.Plan != nil && (!ok || final.Plan == nil) {
				return fail(409, "教学规划与交付内容不一致，请恢复后继续制作")
			}
			if ok && final.Plan != nil {
				if e := s.teachingDeckDeliveryError(final, mode == "sample"); e != nil {
					return e
				}
				if len(final.Slides) == 0 || len(final.Slides) != len(final.Plan.Pages) {
					return fail(409, "教学规划尚未全部完成，暂不能交付")
				}
				for _, sl := range final.Slides {
					check := final.PageChecks[sl.ID]
					if check.Hash != slideContentHash(final, sl) || !s.pageCheckPassed(check, mode == "sample", sl) {
						return fail(409, "课件仍有页面未完成内容与实际画面审核，暂不能交付")
					}
				}
			}
		}
		if taskType == "animation" {
			return fail(410, "旧版动画不再结算；HTML动画通过独立验收流程交付")
		}
		if taskType == "revision" {
			final, ok := result.(Deck)
			var parent Deck
			var mode string
			if e = tx.QueryRow(ctx, "SELECT result,mode FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL FOR UPDATE", input.ParentID, member).Scan(&parent, &mode); e != nil {
				return e
			}
			if parent.Plan != nil && len(parent.Plan.LearningGoals) > 0 && (!ok || final.Plan == nil || len(final.Plan.LearningGoals) == 0) {
				return fail(409, "单页改写不能移除原课件的目标映射和核验")
			}
			if ok {
				if err := s.teachingDeckDeliveryError(final, mode == "sample"); err != nil {
					return err
				}
			}
			changed, e := tx.Exec(ctx, "UPDATE tasks SET result=$1,pptx_path=$2,revision=revision+1,updated_at=now() WHERE id=$3 AND member_id=$4 AND revision=$5 AND status='succeeded' AND deleted_at IS NULL", asJSON(result), path, input.ParentID, member, input.ParentRevision)
			if e != nil {
				return e
			}
			if changed.RowsAffected() != 1 {
				return fmt.Errorf("原课件已变更，单页改写未覆盖原版本")
			}
			if e = archiveRevision(ctx, tx, tid, input.ParentID); e != nil {
				return e
			}
			if e = event(ctx, tx, tid, input.ParentID, "edited", "单页AI修改已写入新版本；结算记录在对应改写任务中"); e != nil {
				return e
			}
		}
	}
	kind := "release"
	delta := int64(0)
	newCharge := "released"
	var balance int64
	if settled {
		kind = "settle"
		delta = -cost
		newCharge = "settled"
		e = tx.QueryRow(ctx, "UPDATE members SET reserved=reserved-$1,balance=balance-$1 WHERE id=$2 AND reserved>=$1 RETURNING balance", cost, member).Scan(&balance)
	} else {
		e = tx.QueryRow(ctx, "UPDATE members SET reserved=reserved-$1 WHERE id=$2 AND reserved>=$1 RETURNING balance", cost, member).Scan(&balance)
	}
	if e != nil {
		return e
	}
	if result == nil {
		result = map[string]any{}
	}
	errorText := message
	if settled {
		errorText = ""
	}
	_, e = tx.Exec(ctx, "UPDATE tasks SET status=$1,stage=$1,error=$2,charge_state=$3,pptx_path=CASE WHEN type IN ('courseware','revision') THEN $4 ELSE pptx_path END,mp4_path=CASE WHEN type='animation' THEN $4 ELSE mp4_path END,result=CASE WHEN $5::jsonb='{}'::jsonb THEN result ELSE $5::jsonb END,revision=revision+1,restored_at=NULL,capacity_hold=false,lease_until=CASE WHEN status IN ('researching','rendering','expanding') AND $1 IN ('failed','cancelled') THEN lease_until ELSE NULL END,updated_at=now() WHERE id=$6", status, errorText, newCharge, path, asJSON(result), id)
	if e != nil {
		return e
	}
	reference := id
	if cycle > 0 {
		reference = fmt.Sprintf("%s:%d", id, cycle)
	}
	_, e = tx.Exec(ctx, "INSERT INTO ledger(tenant_id,member_id,kind,delta,balance_after,reference) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING", tid, member, kind, delta, balance, reference)
	if e != nil {
		return e
	}
	if settled {
		if e = archiveRevision(ctx, tx, tid, id); e != nil {
			return e
		}
	}
	if e = event(ctx, tx, tid, id, status, message); e != nil {
		return e
	}
	if status == "failed" || status == "cancelled" {
		if e = s.limitInterrupted(ctx, tx, member); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
