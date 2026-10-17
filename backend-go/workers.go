package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Round robin is shared by this process; row locks and attempts fence other processes.
func (s *Server) tenantIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.db.Query(ctx, "SELECT id FROM tenants ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}
func (s *Server) workerTenants(ctx context.Context) ([]int64, error) {
	return s.scheduledTenants(ctx, false)
}
func (s *Server) scheduledTenants(ctx context.Context, video bool) ([]int64, error) {
	ids, err := s.tenantIDs(ctx)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return ids, nil
	}
	s.scheduleMu.Lock()
	defer s.scheduleMu.Unlock()
	cursor := s.nextTenant
	if video {
		cursor = s.nextExportTenant
	}
	first := 0
	for i, id := range ids {
		if id > cursor {
			first = i
			break
		}
	}
	if video {
		s.nextExportTenant = ids[first]
	} else {
		s.nextTenant = ids[first]
	}
	return append(ids[first:], ids[:first]...), nil
}
func workerLoop(ctx context.Context, action func(context.Context) error) {
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for ctx.Err() == nil {
		if err := action(ctx); err != nil && ctx.Err() == nil {
			slog.Error("worker operation failed", "error_type", fmt.Sprintf("%T", err))
		}
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}
func (s *Server) worker(ctx context.Context) {
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); workerLoop(ctx, s.workOne) }()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		workerLoop(ctx, func(ctx context.Context) error {
			ids, err := s.scheduledTenants(ctx, true)
			if err != nil {
				return err
			}
			for _, id := range ids {
				worked, err := s.workAnimationExport(ctx, id)
				if worked || err != nil {
					return err
				}
			}
			return nil
		})
	}()
	wg.Add(1)
	go func() { defer wg.Done(); workerLoop(ctx, s.recoverExpiredTasks) }()
	wg.Wait()
}
func (s *Server) runLeasedTask(ctx context.Context, cancel context.CancelFunc, a Actor, id string, attempt int, action func() error) error {
	stopped := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(time.Minute)
		defer tick.Stop()
		for {
			select {
			case <-stopped:
				return
			case <-ctx.Done():
				return
			case <-tick.C:
				tx, err := s.tx(ctx, a.TenantID)
				if err != nil {
					cancel()
					return
				}
				res, err := tx.Exec(ctx, `UPDATE tasks SET lease_until=now()+interval '3 minutes' WHERE id=$1 AND member_id=$2 AND attempt=$3 AND deleted_at IS NULL AND (status IN ('researching','rendering','expanding') OR status LIKE 'anim_%_running')`, id, a.MemberID, attempt)
				if err == nil {
					err = tx.Commit(ctx)
				} else {
					tx.Rollback(ctx)
				}
				tx.Rollback(ctx) // Also release a transaction whose commit failed.
				if err != nil || res.RowsAffected() == 0 {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { close(stopped); <-done }()
	return action()
}

// Retain reservations and retry budgets on infrastructure interruptions.
// Cancelled tasks cannot match this update. Attempt increments fence late workers.
func (s *Server) requeueInterrupted(tid int64, id string, attempt int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := s.tx(ctx, tid)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	res, err := tx.Exec(ctx, `UPDATE tasks SET status=CASE status WHEN 'researching' THEN 'pending' WHEN 'rendering' THEN 'confirmed' WHEN 'expanding' THEN 'expand_pending' ELSE replace(status,'_running','_pending') END,stage='auto_recovering',attempt=attempt+1,lease_until=NULL,next_attempt_at=NULL,updated_at=now() WHERE id=$1 AND attempt=$2 AND deleted_at IS NULL AND (status IN ('researching','rendering','expanding') OR status LIKE 'anim_%_running')`, id, attempt)
	if err != nil {
		return err
	}
	if res.RowsAffected() > 0 {
		if err = event(ctx, tx, tid, id, "auto_recovering", "服务重启或执行连接中断，已保存检查点，正在排队恢复；不重复扣费"); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (s *Server) recoverExpiredTasks(ctx context.Context) error {
	ids, err := s.tenantIDs(ctx)
	if err != nil {
		return err
	}
	for _, tid := range ids {
		tx, err := s.tx(ctx, tid)
		if err != nil {
			return err
		}
		// A process interruption does not consume an export failure attempt. A new
		// token fences a late exporter even when its attempt number is reused.
		_, err = tx.Exec(ctx, `UPDATE tasks SET animation_export=animation_export || jsonb_build_object('status','pending','token',md5(random()::text || clock_timestamp()::text || id),'attempt',GREATEST(0,COALESCE((animation_export->>'attempt')::int,1)-1),'error','服务中断，视频导出已排队恢复'),animation_export_lease_until=NULL,updated_at=now() WHERE deleted_at IS NULL AND animation_export->>'status'='running' AND animation_export_lease_until<now()`)
		if err != nil {
			tx.Rollback(ctx)
			return err
		}
		rows, err := queryMaps(ctx, tx, `SELECT id,attempt FROM tasks WHERE deleted_at IS NULL AND lease_until<now() AND (status IN ('researching','rendering','expanding') OR status LIKE 'anim_%_running')`)
		if err == nil {
			err = tx.Commit(ctx)
		} else {
			tx.Rollback(ctx)
		}
		if err != nil {
			return err
		}
		for _, row := range rows {
			if err = s.requeueExpired(ctx, tid, row["id"].(string), int(row["attempt"].(int32))); err != nil {
				return err
			}
		}
	}
	return nil
}

// Recheck expiration under the row lock so a concurrent renewal always wins.
func (s *Server) requeueExpired(ctx context.Context, tid int64, id string, attempt int) error {
	tx, err := s.tx(ctx, tid)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	res, err := tx.Exec(ctx, `UPDATE tasks SET status=CASE status WHEN 'researching' THEN 'pending' WHEN 'rendering' THEN 'confirmed' WHEN 'expanding' THEN 'expand_pending' ELSE replace(status,'_running','_pending') END,stage='auto_recovering',attempt=attempt+1,lease_until=NULL,next_attempt_at=now()+interval '2 seconds',updated_at=now() WHERE id=$1 AND attempt=$2 AND lease_until<now() AND deleted_at IS NULL AND (status IN ('researching','rendering','expanding') OR status LIKE 'anim_%_running')`, id, attempt)
	if err != nil {
		return err
	}
	if res.RowsAffected() > 0 {
		if err = event(ctx, tx, tid, id, "auto_recovering", "执行租约中断，正在从已保存检查点恢复；不重复扣费"); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
