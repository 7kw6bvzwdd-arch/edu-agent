package main

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
)

type coursewareBudgetExhausted struct{}
type coursewareManualReviewKey struct{}

func (coursewareBudgetExhausted) Error() string {
	return "制作服务持续异常，已保存当前课件并停止重复尝试；请管理员检查服务后再继续，未完成部分不会结算"
}

// Safety envelope, not a page or business-points cap. The baseline 14-page run
// used 72 calls; allow eight calls per planned page plus planning/recovery room.
func coursewareProviderLimit(pages int) int { return 20 + 8*min(100, max(3, pages)) }

// Reserve before contacting the supplier. A persisted counter also covers
// failed/cancelled calls and survives worker restart and usage-detail deletion.
func (s *Server) reserveCoursewareProviderCall(ctx context.Context, a Actor, id string) error {
	if s.cfg.Mode != "live" || id == "" {
		return nil
	}
	tx, err := s.tx(ctx, a.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	manual := ctx.Value(coursewareManualReviewKey{}) == true && generationCycle(ctx) == nil && generationAttempt(ctx) == nil
	changed, err := tx.Exec(ctx, `UPDATE tasks SET courseware_provider_calls=courseware_provider_calls+1
	 WHERE id=$1 AND member_id=$2 AND type='courseware' AND mode='live'
	 AND deleted_at IS NULL AND ((charge_state='reserved' AND status IN ('researching','expanding','rendering'))
	 OR ($5 AND ((charge_state IN ('reserved','settled') AND status IN ('awaiting_preview','awaiting_confirmation','succeeded'))
     OR (status='failed' AND charge_state='released' AND input->>'preview_first'='true'
       AND jsonb_typeof(result->'generation')='object'
       AND CASE WHEN jsonb_typeof(result->'slides')='array' THEN jsonb_array_length(result->'slides')>0 ELSE false END
       AND (lease_until IS NULL OR lease_until<=now())))))
	 AND ($3::integer IS NULL OR charge_cycle=$3) AND ($4::integer IS NULL OR attempt=$4)
	 AND courseware_provider_calls < 20+8*LEAST(100,GREATEST(3,
	 CASE WHEN jsonb_typeof(result->'plan'->'pages')='array' THEN jsonb_array_length(result->'plan'->'pages')
	 ELSE COALESCE(NULLIF((input->>'pages')::integer,0),15) END))`, id, a.MemberID, generationCycle(ctx), generationAttempt(ctx), manual)
	if err != nil {
		return err
	}
	if changed.RowsAffected() == 1 {
		return tx.Commit(ctx)
	}
	// Only exceptional paths need a second query; routine calls neither download
	// the full deck nor make a separate read/update round-trip over the tunnel.
	var active bool
	err = tx.QueryRow(ctx, `SELECT deleted_at IS NULL AND ((charge_state='reserved' AND status IN ('researching','expanding','rendering'))
	 OR ($5 AND ((charge_state IN ('reserved','settled') AND status IN ('awaiting_preview','awaiting_confirmation','succeeded'))
     OR (status='failed' AND charge_state='released' AND input->>'preview_first'='true'
       AND jsonb_typeof(result->'generation')='object'
       AND CASE WHEN jsonb_typeof(result->'slides')='array' THEN jsonb_array_length(result->'slides')>0 ELSE false END
       AND (lease_until IS NULL OR lease_until<=now())))))
	 AND ($3::integer IS NULL OR charge_cycle=$3) AND ($4::integer IS NULL OR attempt=$4)
	 FROM tasks WHERE id=$1 AND member_id=$2 AND type='courseware' AND mode='live'`, id, a.MemberID, generationCycle(ctx), generationAttempt(ctx), manual).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	} // Standalone reviewer/sample fixture, not live generation.
	if err != nil {
		return err
	}
	if !active {
		return context.Canceled
	}
	return coursewareBudgetExhausted{}
}
