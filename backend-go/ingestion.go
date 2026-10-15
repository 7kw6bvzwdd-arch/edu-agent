package main

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

func (s *Server) retryDocument(w http.ResponseWriter, r *http.Request, a Actor) error {
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	res, e := tx.Exec(r.Context(), "UPDATE documents SET status='pending',error='' WHERE id=$1 AND status='failed' AND upload_path<>''", r.PathValue("id"))
	if e != nil {
		return e
	}
	if res.RowsAffected() != 1 {
		return fail(409, "只有解析失败的上传文档可以重试")
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	respond(w, 202, map[string]bool{"ok": true})
	return nil
}

// Reparse in place: retain previous pages/chunks until saveDocument commits the
// replacement. Existing tasks keep their frozen input and source snapshots.
func (s *Server) reparseDocument(w http.ResponseWriter, r *http.Request, a Actor) error {
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	res, e := tx.Exec(r.Context(), "UPDATE documents SET status='pending',error='',lease_until=NULL WHERE id=$1 AND status='ready' AND upload_path<>''", r.PathValue("id"))
	if e != nil {
		return e
	}
	if res.RowsAffected() != 1 {
		return fail(409, "只有已就绪且保留原文件的资料可以重新解析；正在解析的资料请等待")
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	respond(w, 202, map[string]bool{"ok": true})
	return nil
}
func (s *Server) documentWorker(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if e := s.ingestOne(ctx); e != nil {
			fmt.Println("document worker:", e)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Server) ingestOne(ctx context.Context) error {
	rows, e := s.db.Query(ctx, "SELECT id FROM tenants ORDER BY id")
	if e != nil {
		return e
	}
	tenants := []int64{}
	for rows.Next() {
		var tid int64
		if e = rows.Scan(&tid); e != nil {
			rows.Close()
			return e
		}
		tenants = append(tenants, tid)
	}
	rows.Close()
	for _, tid := range tenants {
		tx, e := s.tx(ctx, tid)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "UPDATE documents SET status='failed',error='解析等待超时，可重试',lease_until=NULL WHERE status='indexing' AND lease_until<now()")
		if e != nil {
			tx.Rollback(ctx)
			return e
		}
		var id, path, name string
		var member int64
		e = tx.QueryRow(ctx, "SELECT id,upload_path,name,member_id FROM documents WHERE status='pending' ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1").Scan(&id, &path, &name, &member)
		if e == pgx.ErrNoRows {
			tx.Commit(ctx)
			continue
		}
		if e != nil {
			tx.Rollback(ctx)
			return e
		}
		_, e = tx.Exec(ctx, "UPDATE documents SET status='indexing',lease_until=now()+interval '10 minutes' WHERE id=$1", id)
		if e != nil {
			tx.Rollback(ctx)
			return e
		}
		if e = tx.Commit(ctx); e != nil {
			return e
		}
		work, cancel := context.WithTimeout(ctx, 9*time.Minute)
		pages, e := s.extractFile(work, path, name)
		if e == nil {
			_, e = s.saveDocument(work, Actor{TenantID: tid, MemberID: member}, name, "", pages, id)
		}
		cancel()
		if e != nil {
			cleanup, c := context.WithTimeout(context.Background(), 10*time.Second)
			defer c()
			tx, err := s.tx(cleanup, tid)
			if err != nil {
				return err
			}
			defer tx.Rollback(cleanup)
			_, err = tx.Exec(cleanup, "UPDATE documents SET status='failed',error=$1,lease_until=NULL WHERE id=$2 AND status='indexing'", e.Error(), id)
			if err != nil {
				return err
			}
			return tx.Commit(cleanup)
		}
		return nil
	}
	return nil
}
