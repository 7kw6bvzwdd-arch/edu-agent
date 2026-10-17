package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type coursewareRepairExhausted struct {
	Page  int
	Issue string
}

func (e coursewareRepairExhausted) Error() string {
	return fmt.Sprintf("第%d页达到本轮自动修复上限，草稿已保存，未完成部分不结算；请检查问题后继续：%s", e.Page, e.Issue)
}

func coursewareCanResume(err error) bool {
	var exhaustedBudget coursewareBudgetExhausted
	if errors.As(err, &exhaustedBudget) {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	var upstream providerCallError
	if errors.As(err, &upstream) {
		return upstream.Temporary || upstream.Status >= 500
	}
	var api APIError
	if errors.As(err, &api) {
		return api.Status >= 500 || api.Status == 429 || api.Status == 422
	}
	var unavailable pageReviewUnavailable
	if errors.As(err, &unavailable) {
		// Missing configuration needs administrator attention. A malformed review
		// response can be rechecked without spending on page generation again.
		return unavailable.Cause != nil
	}
	var exhausted coursewareRepairExhausted
	return errors.As(err, &exhausted) || errors.Is(err, context.DeadlineExceeded) || coursewareAutomaticRepairable(err)
}

func coursewareResumeDelay(failures int) time.Duration {
	// Retry transient failures with backoff. The persisted page allowance and
	// consecutive-failure ceiling below stop repeated unsuccessful rounds.
	return min(time.Hour, 15*time.Second*time.Duration(1<<min(8, max(0, failures-1))))
}

// Reuse the queue and the same reservation. No retry HTTP request, new work,
// ledger entry, or charge cycle is created. The attempt fences late workers.
func (s *Server) resumeCoursewareAutomatically(ctx context.Context, a Actor, id, stage string, in TaskInput, err error) (bool, error) {
	if !in.PreviewFirst || !coursewareCanResume(err) {
		return false, nil
	}
	if stage != "researching" && stage != "expanding" && stage != "rendering" {
		return false, nil
	}
	if e := s.checkWorkPermission(ctx, a, "courseware"); e != nil {
		return false, nil
	}
	tx, e := s.tx(ctx, a.TenantID)
	if e != nil {
		return false, e
	}
	defer tx.Rollback(ctx)
	var failures int
	var d Deck
	e = tx.QueryRow(ctx, `SELECT automatic_failures,result FROM tasks WHERE id=$1 AND member_id=$2
		AND type='courseware' AND mode=$3 AND status=$4 AND charge_state='reserved' AND deleted_at IS NULL
		AND ($5::integer IS NULL OR charge_cycle=$5) AND ($6::integer IS NULL OR attempt=$6) FOR UPDATE`,
		id, a.MemberID, s.cfg.Mode, stage, generationCycle(ctx), generationAttempt(ctx)).Scan(&failures, &d)
	if errors.Is(e, pgx.ErrNoRows) {
		return true, nil
	} // Cancelled, deleted, or a newer worker owns it.
	if e != nil {
		return false, e
	}
	if d.Generation != nil && d.Generation.PageAttempts[d.Generation.RepairPage] >= 3 {
		return false, nil // Existing failure path preserves the draft and releases its reservation.
	}
	if failures >= 3 {
		return false, nil
	}
	queue := map[string]string{"researching": "pending", "expanding": "expand_pending", "rendering": "confirmed"}[stage]
	page := 0
	if d.Generation != nil {
		page = d.Generation.RepairPage
	}
	message := "制作服务暂时中断，已保存进度，将自动恢复，无需点击继续制作"
	if page > 0 {
		message = fmt.Sprintf("第%d页仍需修复，已保存其他页面，将自动继续，无需点击", page)
	}
	delay := coursewareResumeDelay(failures + 1)
	_, e = tx.Exec(ctx, `UPDATE tasks SET status=$1,stage='auto_recovering',error='',lease_until=NULL,attempt=attempt+1,
		next_attempt_at=now()+$2*interval '1 second',automatic_failures=automatic_failures+1,updated_at=now() WHERE id=$3`, queue, int(delay.Seconds()), id)
	if e != nil {
		return false, e
	}
	if e = event(ctx, tx, a.TenantID, id, "auto_recovering", message); e != nil {
		return false, e
	}
	return true, tx.Commit(ctx)
}
