package main

import (
	"context"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
	"time"
)

type usageSelection struct {
	IDs  []int64 `json:"ids"`
	All  bool    `json:"all"`
	From string  `json:"from"`
	To   string  `json:"to"`
}

func pruneUsageRetention(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `DELETE FROM usage_logs WHERE id IN (
 SELECT id FROM (SELECT id,created_at,row_number() OVER(PARTITION BY member_id ORDER BY created_at DESC,id DESC) AS n FROM usage_logs) retained
 WHERE n>5000 OR created_at<now()-interval '90 days');
 DELETE FROM usage_totals WHERE
 (period LIKE 'd:%' AND (period<'d:'||to_char((now() AT TIME ZONE 'Asia/Shanghai')::date-89,'YYYY-MM-DD') OR period>'d:'||to_char(now() AT TIME ZONE 'Asia/Shanghai','YYYY-MM-DD')))
 OR (period LIKE 'm:%' AND (period<'m:'||to_char(date_trunc('month',now() AT TIME ZONE 'Asia/Shanghai')-interval '23 months','YYYY-MM') OR period>'m:'||to_char(now() AT TIME ZONE 'Asia/Shanghai','YYYY-MM')));`)
	return err
}

func usageTimes(from, to string) (any, any, error) {
	var start, end *time.Time
	for _, field := range []struct {
		value  string
		target **time.Time
	}{{from, &start}, {to, &end}} {
		value, target := field.value, field.target
		if value == "" {
			continue
		}
		v, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return nil, nil, fail(400, "请选择有效的起止时间")
		}
		*target = &v
	}
	if start != nil && end != nil && !start.Before(*end) {
		return nil, nil, fail(400, "结束时间必须晚于开始时间")
	}
	return start, end, nil
}
func usageMember(a Actor) any {
	if allowed(a, "log.view") {
		return nil
	}
	return a.MemberID
}
func (s *Server) usageRecords(w http.ResponseWriter, r *http.Request, a Actor) error {
	q := r.URL.Query()
	limit, offset := 50, 0
	for key, dest := range map[string]*int{"limit": &limit, "offset": &offset} {
		if q.Has(key) {
			v, e := strconv.Atoi(q.Get(key))
			if e != nil || v < 0 || key == "limit" && (v < 1 || v > 100) {
				return fail(400, "分页参数无效")
			}
			*dest = v
		}
	}
	from, to, err := usageTimes(q.Get("from"), q.Get("to"))
	if err != nil {
		return err
	}
	tx, err := s.tx(r.Context(), a.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	where := " FROM usage_logs WHERE ($1::bigint IS NULL OR member_id=$1) AND ($2::timestamptz IS NULL OR created_at>=$2) AND ($3::timestamptz IS NULL OR created_at<$3)"
	var total int64
	if err = tx.QueryRow(r.Context(), "SELECT count(*)"+where, usageMember(a), from, to).Scan(&total); err != nil {
		return err
	}
	rows, err := queryMaps(r.Context(), tx, "SELECT id::text AS id,operation,model,input_tokens,output_tokens,latency_ms,success,created_at"+where+" ORDER BY created_at DESC,id DESC LIMIT $4 OFFSET $5", usageMember(a), from, to, limit, offset)
	if err != nil {
		return err
	}
	respond(w, 200, map[string]any{"items": rows, "total": total, "limit": limit, "offset": offset})
	return nil
}
func (s *Server) deleteUsageRecords(w http.ResponseWriter, r *http.Request, a Actor) error {
	var body usageSelection
	if err := decode(r, &body); err != nil {
		return err
	}
	if body.All && (body.From == "" || body.To == "" || len(body.IDs) > 0) || !body.All && (len(body.IDs) == 0 || len(body.IDs) > 100) {
		return fail(400, "请选择记录；跨页全选必须指定起止时间")
	}
	from, to, err := usageTimes(body.From, body.To)
	if err != nil {
		return err
	}
	tx, err := s.tx(r.Context(), a.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	if !body.All {
		unique := map[int64]bool{}
		for _, id := range body.IDs {
			if id < 1 {
				return fail(400, "记录编号无效")
			}
			unique[id] = true
		}
		rows, err := tx.Query(r.Context(), "SELECT id FROM usage_logs WHERE id=ANY($1::bigint[]) AND ($2::bigint IS NULL OR member_id=$2) FOR UPDATE", body.IDs, usageMember(a))
		if err != nil {
			return err
		}
		n := 0
		for rows.Next() {
			n++
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if n != len(unique) {
			return pgx.ErrNoRows
		}
	}
	result, err := tx.Exec(r.Context(), "DELETE FROM usage_logs WHERE ($1::bigint IS NULL OR member_id=$1) AND (($2 AND created_at>=$3::timestamptz AND created_at<$4::timestamptz) OR (NOT $2 AND id=ANY($5::bigint[])))", usageMember(a), body.All, from, to, body.IDs)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), "INSERT INTO audit_logs(tenant_id,member_id,action,target,detail) VALUES($1,$2,'usage.delete','usage',$3)", a.TenantID, a.MemberID, asJSON(map[string]any{"count": result.RowsAffected(), "all": body.All, "from": body.From, "to": body.To})); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	respond(w, 200, map[string]any{"deleted": result.RowsAffected()})
	return nil
}
