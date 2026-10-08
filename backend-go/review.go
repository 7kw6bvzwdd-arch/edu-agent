package main

import (
	"context"
	"net/http"
	"time"
)

func (s *Server) reviewCourseware(ctx context.Context, a Actor, id string) error {
	tx, e := s.tx(ctx, a.TenantID)
	if e != nil {
		return e
	}
	var d Deck
	var sources []Source
	var revision int
	e = tx.QueryRow(ctx, "SELECT result,sources,revision FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL AND status='succeeded'", id, a.MemberID).Scan(&d, &sources, &revision)
	tx.Rollback(ctx)
	if e != nil {
		return e
	}
	report := s.assessTeaching(ctx, a, id, "courseware", d, sources)
	report["reviewed_revision"] = revision
	d.Evaluation = report
	tx, e = s.tx(ctx, a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	changed, e := tx.Exec(ctx, "UPDATE tasks SET result=$1 WHERE id=$2 AND revision=$3 AND member_id=$4 AND deleted_at IS NULL", asJSON(d), id, revision, a.MemberID)
	if e != nil {
		return e
	}
	if changed.RowsAffected() == 0 {
		return nil
	}
	if e = event(ctx, tx, a.TenantID, id, "reviewed", "辅助评审状态已更新；不影响成品及积分结算"); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s *Server) reviewAgain(w http.ResponseWriter, r *http.Request, a Actor) error {
	ctx, cancel := context.WithTimeout(r.Context(), 100*time.Second)
	defer cancel()
	if e := s.reviewCourseware(ctx, a, r.PathValue("id")); e != nil {
		return e
	}
	respond(w, 200, map[string]bool{"ok": true})
	return nil
}
