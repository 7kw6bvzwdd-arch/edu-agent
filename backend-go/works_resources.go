package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/minio/minio-go/v7"
)

func (s *Server) limitInterrupted(ctx context.Context, tx pgx.Tx, member int64) error {
	var locked int64
	if err := tx.QueryRow(ctx, "SELECT id FROM members WHERE id=$1 FOR UPDATE", member).Scan(&locked); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT id,CASE WHEN jsonb_typeof(result->'slides')='array' THEN jsonb_array_length(result->'slides') ELSE 0 END>0 OR pptx_path<>'' OR mp4_path<>'' AS valuable
 FROM tasks t WHERE member_id=$1 AND deleted_at IS NULL AND status IN ('failed','cancelled') AND charge_state<>'reserved' AND restored_at IS NULL
 AND NOT EXISTS(SELECT 1 FROM tasks child WHERE child.member_id=t.member_id AND child.idempotency_key='retry-'||t.id)
 ORDER BY updated_at DESC,id DESC FOR UPDATE SKIP LOCKED`, member)
	if err != nil {
		return err
	}
	type item struct {
		id       string
		valuable bool
	}
	items := []item{}
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.id, &v.valuable); err != nil {
			rows.Close()
			return err
		}
		items = append(items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	excess := max(0, len(items)-12)
	removed := map[string]bool{}
	for i := len(items) - 1; i >= 0 && excess > 0; i-- {
		v := items[i]
		if v.valuable {
			continue
		}
		var referenced bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tasks WHERE deleted_at IS NULL AND (input->>'parent_id'=$1 OR input->>'source_task_id'=$1)) OR EXISTS(SELECT 1 FROM courseware_messages WHERE task_id=$1 AND status='pending')`, v.id).Scan(&referenced); err != nil {
			return err
		}
		if referenced {
			continue
		}
		if _, err = tx.Exec(ctx, "UPDATE tasks SET deleted_at=now(),cleanup_pending=true,capacity_hold=false WHERE id=$1", v.id); err != nil {
			return err
		}
		removed[v.id] = true
		excess--
	}
	kept := 0
	for _, v := range items {
		if removed[v.id] {
			continue
		}
		kept++
		if _, err = tx.Exec(ctx, "UPDATE tasks SET capacity_hold=$1 WHERE id=$2 AND capacity_hold IS DISTINCT FROM $1", kept > 12, v.id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) deleteWorks(w http.ResponseWriter, r *http.Request, a Actor) error {
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	if len(body.IDs) < 1 || len(body.IDs) > 100 {
		return fail(400, "请选择1至100个作品")
	}
	tx, err := s.tx(r.Context(), a.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	unique := map[string]bool{}
	sort.Strings(body.IDs)
	for _, id := range body.IDs {
		if unique[id] {
			continue
		}
		unique[id] = true
		var status, charge string
		if err = tx.QueryRow(r.Context(), "SELECT status,charge_state FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL FOR UPDATE", id, a.MemberID).Scan(&status, &charge); err != nil {
			return err
		}
		if charge == "reserved" || (status != "succeeded" && status != "degraded" && status != "failed" && status != "cancelled") {
			return fail(409, "所选作品仍在制作，请先取消后删除")
		}
		var active bool
		if err = tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM tasks WHERE member_id=$1 AND deleted_at IS NULL AND charge_state='reserved' AND (input->>'parent_id'=$2 OR input->>'source_task_id'=$2))", a.MemberID, id).Scan(&active); err != nil {
			return err
		}
		if active {
			return fail(409, "所选作品仍有关联制作，请稍后删除")
		}
		if err = coursewareChatIdle(r.Context(), tx, id, ""); err != nil {
			return err
		}
	}
	// Validation and hiding are atomic; file cleanup waits for active leases.
	if _, err = tx.Exec(r.Context(), "UPDATE tasks SET deleted_at=now(),cleanup_pending=true,updated_at=now() WHERE member_id=$1 AND deleted_at IS NULL AND (id=ANY($2::text[]) OR (type='revision' AND input->>'parent_id'=ANY($2::text[])))", a.MemberID, body.IDs); err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), "INSERT INTO audit_logs(tenant_id,member_id,action,target,detail) VALUES($1,$2,'task.delete_many','works',$3)", a.TenantID, a.MemberID, asJSON(body)); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	respond(w, 200, map[string]any{"deleted": len(unique), "cleanup_pending": true})
	return nil
}

func (s *Server) restoreWork(w http.ResponseWriter, r *http.Request, a Actor) error {
	tx, err := s.tx(r.Context(), a.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	id := r.PathValue("id")
	var status string
	if err = tx.QueryRow(r.Context(), "SELECT status FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL FOR UPDATE", id, a.MemberID).Scan(&status); err != nil {
		return err
	}
	if status != "failed" && status != "cancelled" {
		return fail(409, "仅待处理或已取消的作品可以还原")
	}
	var superseded bool
	if err = tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM tasks WHERE member_id=$1 AND idempotency_key=$2)", a.MemberID, "retry-"+id).Scan(&superseded); err != nil {
		return err
	}
	if superseded {
		return fail(409, "已有后续制作记录，请从最新一次作品操作")
	}
	result, err := tx.Exec(r.Context(), "UPDATE tasks SET restored_at=now(),capacity_hold=false,updated_at=now() WHERE id=$1 AND restored_at IS NULL", id)
	if err != nil {
		return err
	}
	if result.RowsAffected() > 0 {
		if _, err = tx.Exec(r.Context(), "INSERT INTO audit_logs(tenant_id,member_id,action,target) VALUES($1,$2,'task.restore',$3)", a.TenantID, a.MemberID, id); err != nil {
			return err
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	respond(w, 200, map[string]bool{"ok": true})
	return nil
}

// Ids and locations come from owned database rows, never a browser-supplied path.
var artifactTaskID = regexp.MustCompile(`^[a-f0-9]{32}$`)

func (s *Server) cleanupDeletedTask(ctx context.Context, tid int64, id string) (bool, error) {
	tx, err := s.tx(ctx, tid)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var pending, leased, deleted bool
	err = tx.QueryRow(ctx, "SELECT cleanup_pending,COALESCE(lease_until>now(),false) OR charge_state='reserved',deleted_at IS NOT NULL FROM tasks WHERE id=$1 FOR UPDATE", id).Scan(&pending, &leased, &deleted)
	if err != nil {
		return false, err
	}
	if !pending {
		return true, nil
	}
	if leased {
		return false, nil
	}
	if !deleted {
		var busy bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM tasks WHERE deleted_at IS NULL AND charge_state='reserved' AND input->>'parent_id'=$1) OR EXISTS(SELECT 1 FROM courseware_messages WHERE task_id=$1 AND status='pending')", id).Scan(&busy); err != nil {
			return false, err
		}
		if busy {
			return false, nil
		}
	}
	if !artifactTaskID.MatchString(id) {
		return false, errors.New("invalid artifact task id")
	}
	root, err := filepath.Abs(s.cfg.Storage)
	if err != nil {
		return false, err
	}
	tenantDir := filepath.Join(root, fmt.Sprint(tid))
	if err = s.safeStoragePath(tenantDir); err != nil {
		return false, err
	}
	canonical := func(p string) string {
		abs, e := filepath.Abs(p)
		if e != nil {
			return ""
		}
		return filepath.Clean(abs)
	}
	// Protect every live reference, including reused previews owned by other attempts.
	protected := map[string]bool{}
	rows, err := tx.Query(ctx, `SELECT pptx_path,mp4_path FROM tasks WHERE deleted_at IS NULL
 UNION ALL SELECT ar.pptx_path,ar.mp4_path FROM artifact_revisions ar JOIN tasks t ON t.id=ar.task_id AND t.tenant_id=ar.tenant_id WHERE t.deleted_at IS NULL`)
	if err != nil {
		return false, err
	}
	for rows.Next() {
		var p, m string
		if err = rows.Scan(&p, &m); err != nil {
			rows.Close()
			return false, err
		}
		for _, v := range []string{p, m} {
			if v != "" {
				protected[canonical(v)] = true
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	paths := map[string]bool{}
	recent := map[string]bool{}
	rows, err = tx.Query(ctx, "SELECT pptx_path,mp4_path FROM tasks WHERE id=$1 UNION ALL SELECT pptx_path,mp4_path FROM artifact_revisions WHERE task_id=$1", id)
	if err != nil {
		return false, err
	}
	for rows.Next() {
		var p, m string
		if err = rows.Scan(&p, &m); err != nil {
			rows.Close()
			return false, err
		}
		for _, v := range []string{p, m} {
			if v != "" {
				paths[canonical(v)] = true
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	// Include incomplete render files that never made it into a revision row.
	belongs := func(name string) bool { return strings.HasPrefix(name, id+".") || strings.HasPrefix(name, id+"-") }
	entries, err := os.ReadDir(tenantDir)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	for _, entry := range entries {
		if belongs(entry.Name()) {
			path := filepath.Join(tenantDir, strings.TrimSuffix(entry.Name(), ".slides"))
			paths[path] = true
			if info, err := entry.Info(); err == nil && !deleted && info.ModTime().After(time.Now().Add(-time.Hour)) {
				recent[path] = true
			}
		}
	}
	if s.cloud.objects != nil {
		prefix, err := s.objectKey(filepath.Join(tenantDir, id))
		if err != nil {
			return false, err
		}
		for object := range s.cloud.objects.ListObjects(ctx, s.cfg.MinIOBucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
			if object.Err != nil {
				return false, errors.New("cloud artifact listing failed")
			}
			relative := strings.TrimPrefix(object.Key, strings.TrimSuffix(prefix, id))
			name := strings.SplitN(relative, "/", 2)[0]
			if belongs(name) {
				path := filepath.Join(tenantDir, strings.TrimSuffix(name, ".slides"))
				paths[path] = true
				if !deleted && object.LastModified.After(time.Now().Add(-time.Hour)) {
					recent[path] = true
				}
			}
		}
	}
	deferred := false
	for path := range paths {
		if filepath.Dir(path) != tenantDir {
			return false, errors.New("artifact outside tenant storage")
		}
		if protected[path] {
			continue
		}
		if recent[path] {
			deferred = true
			continue
		}
		if err = s.removeArtifactTree(ctx, path+".slides"); err != nil {
			return false, err
		}
		if err = s.safeStoragePath(path); err != nil {
			return false, err
		}
		if err = s.removeStoredFile(ctx, path); err != nil {
			return false, err
		}
	}
	if !deleted {
		_, err = tx.Exec(ctx, "UPDATE tasks SET cleanup_pending=$2 WHERE id=$1", id, deferred)
		if err != nil {
			return false, err
		}
		return true, tx.Commit(ctx)
	}
	for _, dir := range []string{"animation-jobs", "animation-quality"} {
		if err = s.removeArtifactTree(ctx, filepath.Join(root, dir, id)); err != nil {
			return false, err
		}
	}
	for _, table := range []string{"artifact_revisions", "courseware_feedback", "courseware_messages", "task_events"} {
		if _, err = tx.Exec(ctx, "DELETE FROM "+table+" WHERE task_id=$1", id); err != nil {
			return false, err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE tasks SET result='{}',sources='[]',pptx_path='',mp4_path='',error='',input=jsonb_strip_nulls(jsonb_build_object('topic',input->'topic','parent_id',input->'parent_id','source_task_id',input->'source_task_id')),cleanup_pending=false WHERE id=$1`, id)
	if err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

// Reject symlinked parents too, so a local link cannot redirect deletion outside storage.
func (s *Server) safeStoragePath(path string) error {
	if _, err := s.objectKey(path); err != nil {
		return err
	}
	root, err := filepath.Abs(s.cfg.Storage)
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	for p := abs; ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return errors.New("refusing symlink in artifact path")
		}
		if rel, err := filepath.Rel(root, p); err == nil && rel == "." {
			break
		}
		if filepath.Dir(p) == p {
			return errors.New("artifact path does not reach storage root")
		}
	}
	return nil
}
func (s *Server) removeArtifactTree(ctx context.Context, dir string) error {
	if err := s.safeStoragePath(dir); err != nil {
		return err
	}
	if s.cloud.objects != nil {
		key, err := s.objectKey(dir)
		if err != nil {
			return err
		}
		for object := range s.cloud.objects.ListObjects(ctx, s.cfg.MinIOBucket, minio.ListObjectsOptions{Prefix: strings.TrimSuffix(key, "/") + "/", Recursive: true}) {
			if object.Err != nil {
				return errors.New("cloud preview listing failed")
			}
			if err = s.cloud.objects.RemoveObject(ctx, s.cfg.MinIOBucket, object.Key, minio.RemoveObjectOptions{}); err != nil {
				return errors.New("cloud preview deletion failed")
			}
		}
	}
	return os.RemoveAll(dir)
}

func (s *Server) deletedWorkCleaner(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	lastRetention := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		rows, err := s.db.Query(ctx, "SELECT id FROM tenants ORDER BY id")
		if err != nil {
			continue
		}
		var tenants []int64
		for rows.Next() {
			var tid int64
			if rows.Scan(&tid) == nil {
				tenants = append(tenants, tid)
			}
		}
		rows.Close()
		runRetention := time.Since(lastRetention) >= time.Hour
		for _, tid := range tenants {
			tx, err := s.tx(ctx, tid)
			if err != nil {
				continue
			}
			if runRetention {
				if err = pruneUsageRetention(ctx, tx); err != nil {
					tx.Rollback(ctx)
					continue
				}
			}
			rows, err := tx.Query(ctx, "SELECT id FROM tasks WHERE cleanup_pending AND charge_state<>'reserved' AND (lease_until IS NULL OR lease_until<=now()) ORDER BY updated_at LIMIT 20")
			var ids []string
			if err == nil {
				for rows.Next() {
					var id string
					if rows.Scan(&id) == nil {
						ids = append(ids, id)
					}
				}
				rows.Close()
			}
			if err == nil {
				tx.Commit(ctx)
			} else {
				tx.Rollback(ctx)
			}
			for _, id := range ids {
				c, cancel := context.WithTimeout(ctx, 15*time.Second)
				s.cleanupDeletedTask(c, tid, id)
				cancel()
			}
		}
		if runRetention {
			lastRetention = time.Now()
		}
	}
}

// A cancelled worker or in-flight editor can finish after the first cleanup.
func (s *Server) queueDeletedCleanup(tid int64, id string, releaseLease bool, cycles ...int) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := s.tx(ctx, tid)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)
	var cycle any
	if len(cycles) > 0 {
		cycle = cycles[0]
	}
	if _, err = tx.Exec(ctx, "UPDATE tasks SET cleanup_pending=(deleted_at IS NOT NULL OR cleanup_pending),lease_until=CASE WHEN $2 THEN NULL ELSE lease_until END WHERE id=$1 AND (deleted_at IS NOT NULL OR status IN ('failed','cancelled')) AND ($3::integer IS NULL OR charge_cycle=$3)", id, releaseLease, cycle); err == nil {
		tx.Commit(ctx)
	}
}

func (s *Server) workCleanupStatus(w http.ResponseWriter, r *http.Request, a Actor) error {
	tx, err := s.tx(r.Context(), a.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	var pending int
	if err = tx.QueryRow(r.Context(), "SELECT count(*) FROM tasks WHERE member_id=$1 AND deleted_at IS NOT NULL AND cleanup_pending", a.MemberID).Scan(&pending); err != nil {
		return err
	}
	respond(w, 200, map[string]int{"pending": pending})
	return nil
}
