package main

// MP4 is an attached export of an accepted revision, never another billed task.
import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

type animExport struct {
	Status   string `json:"status"`
	Revision int    `json:"revision"`
	Hash     string `json:"hash"`
	Attempt  int    `json:"attempt"`
	Token    string `json:"token"`
	Error    string `json:"error,omitempty"`
}

func animationVideoConfigured() bool {
	script := env("ANIMATION_EXPORT_SCRIPT", "")
	if script == "" {
		return false
	}
	info, err := os.Stat(script)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	_, err = exec.LookPath(env("ANIMATION_NODE_PATH", "node"))
	return err == nil
}

func (s *Server) animationExportAction(w http.ResponseWriter, r *http.Request, a Actor) error {
	var in struct {
		Revision int `json:"revision"`
	}
	if e := decode(r, &in); e != nil {
		return e
	}
	ctx := r.Context()
	tx, e := s.tx(ctx, a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	t, e := animRead(ctx, tx, r.PathValue("id"), a.MemberID, true)
	if e != nil {
		return e
	}
	if !allowed(a, "animation.render") {
		return fail(403, "没有动画制作权限")
	}
	var job animExport
	if e = tx.QueryRow(ctx, "SELECT animation_export FROM tasks WHERE id=$1", t.ID).Scan(&job); e != nil {
		return e
	}
	if r.PathValue("action") == "cancel-export" {
		if in.Revision != job.Revision {
			return fail(409, "导出版本已变化")
		}
		if job.Status == "pending" || job.Status == "running" {
			job.Status = "cancelled"
			// Keep the token until the cancelled process releases its lease.
		}
	} else {
		if t.HTML == "" || in.Revision != t.Flow.AcceptedRevision || t.Flow.AcceptedHash == "" {
			return fail(409, "请先验收交付当前版本再导出视频")
		}
		if job.Revision != in.Revision {
			job = animExport{Revision: in.Revision, Hash: t.Flow.AcceptedHash}
		}
		if job.Status != "pending" && job.Status != "running" && job.Status != "succeeded" {
			if !animationVideoConfigured() {
				return fail(503, "视频导出服务尚未配置，已验收 HTML 仍可下载")
			}
			if job.Attempt >= 3 {
				return fail(409, "本修订导出已达三次尝试上限，请检查渲染服务")
			}
			var leased bool
			if e = tx.QueryRow(ctx, "SELECT COALESCE(animation_export_lease_until>now(),false) FROM tasks WHERE id=$1", t.ID).Scan(&leased); e != nil {
				return e
			}
			if leased {
				return fail(409, "上次视频导出正在取消，请稍后重试")
			}
			job.Status = "pending"
			job.Token = newID()
			job.Error = ""
		}
	}
	if _, e = tx.Exec(ctx, "UPDATE tasks SET animation_export=$1,updated_at=now() WHERE id=$2", asJSON(job), t.ID); e != nil {
		return e
	}
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	respond(w, map[bool]int{true: 200, false: 202}[r.PathValue("action") == "cancel-export"], map[string]any{"export": animPublicExport(job)})
	return nil
}
func animPublicExport(job animExport) map[string]any {
	return map[string]any{"status": job.Status, "revision": job.Revision, "attempts": job.Attempt, "error": job.Error}
}

func (s *Server) workAnimationExport(ctx context.Context, tid int64) (bool, error) {
	tx, e := s.tx(ctx, tid)
	if e != nil {
		return false, e
	}
	defer tx.Rollback(ctx)
	var id string
	var member int64
	var job animExport
	e = tx.QueryRow(ctx, `SELECT id,member_id,animation_export FROM tasks WHERE deleted_at IS NULL AND input->>'animation_engine'='html' AND animation_export->>'status'='pending' AND (animation_export_lease_until IS NULL OR animation_export_lease_until<now()) ORDER BY updated_at FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&id, &member, &job)
	if e == pgx.ErrNoRows {
		return false, tx.Commit(ctx)
	}
	if e != nil {
		return false, e
	}
	job.Status = "running"
	job.Attempt++
	if _, e = tx.Exec(ctx, "UPDATE tasks SET animation_export=$1,animation_export_lease_until=now()+interval '3 minutes' WHERE id=$2", asJSON(job), id); e != nil {
		return false, e
	}
	if e = tx.Commit(ctx); e != nil {
		return false, e
	}
	a := Actor{TenantID: tid, MemberID: member}
	work, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	// Poll persisted cancellation, deletion and lease identity while rendering.
	stopped := make(chan struct{})
	monitorDone := make(chan struct{})
	defer func() { close(stopped); cancel(); <-monitorDone }()
	go func() {
		defer close(monitorDone)
		ticker := time.NewTicker(2 * time.Second)
		lastRenew := time.Now()
		defer ticker.Stop()
		for {
			select {
			case <-stopped:
				return
			case <-work.Done():
				return
			case <-ticker.C:
				check, err := s.tx(work, tid)
				if err != nil {
					cancel()
					return
				}
				var current bool
				err = check.QueryRow(work, "SELECT EXISTS(SELECT 1 FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL AND animation_export->>'token'=$3 AND (animation_export->>'attempt')::int=$4 AND animation_export->>'status'='running')", id, member, job.Token, job.Attempt).Scan(&current)
				if err == nil && current && time.Since(lastRenew) >= time.Minute {
					_, err = check.Exec(work, "UPDATE tasks SET animation_export_lease_until=now()+interval '3 minutes' WHERE id=$1 AND animation_export->>'token'=$2 AND (animation_export->>'attempt')::int=$3 AND animation_export->>'status'='running'", id, job.Token, job.Attempt)
					if err == nil {
						err = check.Commit(work)
						lastRenew = time.Now()
					}
				}
				check.Rollback(work)
				if err != nil || !current {
					cancel()
					return
				}
			}
		}
	}()
	path, renderErr := s.renderAnimationMP4(work, a, id, job)
	published := false
	defer func() {
		if path != "" && !published {
			cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_ = s.removeStoredFile(cleanup, path)
		}
	}()
	done, closeDone := context.WithTimeout(context.Background(), 15*time.Second)
	defer closeDone()
	tx, e = s.tx(done, tid)
	if e != nil {
		return true, e
	}
	defer tx.Rollback(done)
	var current animExport
	var deleted bool
	var accepted int
	if e = tx.QueryRow(done, "SELECT animation_export,deleted_at IS NOT NULL,COALESCE((animation->>'accepted_revision')::int,0) FROM tasks WHERE id=$1 AND member_id=$2 FOR UPDATE", id, member).Scan(&current, &deleted, &accepted); e != nil {
		return true, e
	}
	if deleted || current.Token != job.Token || current.Attempt != job.Attempt || current.Status != "running" {
		_, e = tx.Exec(done, "UPDATE tasks SET animation_export_lease_until=NULL WHERE id=$1 AND animation_export->>'token'=$2 AND (animation_export->>'attempt')::int=$3", id, job.Token, job.Attempt)
		if e != nil {
			return true, e
		}
		return true, tx.Commit(done)
	}
	if renderErr == nil {
		renderErr = s.checkWorkPermission(done, a, "animation")
	}
	if ctx.Err() != nil {
		job.Status = "pending"
		job.Attempt--
		job.Token = newID()
		job.Error = "服务重启，视频导出已排队恢复；已验收课件保留"
	} else if renderErr != nil {
		job.Status = "failed"
		job.Error = "视频导出未完成，可重试；已验收作品和积分不受影响"
	} else {
		job.Status = "succeeded"
		if _, e = tx.Exec(done, "UPDATE artifact_revisions SET mp4_path=$1 WHERE task_id=$2 AND revision=$3", path, id, job.Revision); e != nil {
			return true, e
		}
		if accepted == job.Revision {
			if _, e = tx.Exec(done, "UPDATE tasks SET mp4_path=$1 WHERE id=$2", path, id); e != nil {
				return true, e
			}
		}
	}
	if _, e = tx.Exec(done, "UPDATE tasks SET animation_export=$1,animation_export_lease_until=NULL,updated_at=now() WHERE id=$2", asJSON(job), id); e != nil {
		return true, e
	}
	if e = event(done, tx, tid, id, "animation_export_"+job.Status, map[bool]string{true: "已验收版本 MP4 导出完成", false: job.Error}[job.Status == "succeeded"]); e != nil {
		return true, e
	}
	if e = tx.Commit(done); e != nil {
		return true, e
	}
	published = job.Status == "succeeded"
	return true, nil
}

func (s *Server) renderAnimationMP4(ctx context.Context, a Actor, id string, job animExport) (string, error) {
	if e := s.checkWorkPermission(ctx, a, "animation"); e != nil {
		return "", e
	}
	tx, e := s.tx(ctx, a.TenantID)
	if e != nil {
		return "", e
	}
	var source string
	e = tx.QueryRow(ctx, "SELECT ar.html_path FROM artifact_revisions ar JOIN tasks t ON t.id=ar.task_id AND t.tenant_id=ar.tenant_id WHERE t.id=$1 AND t.member_id=$2 AND t.deleted_at IS NULL AND ar.revision=$3", id, a.MemberID, job.Revision).Scan(&source)
	tx.Rollback(ctx)
	if e != nil {
		return "", e
	}
	html, e := s.readStoredFile(ctx, source)
	if e != nil || animDigest(html) != job.Hash {
		return "", fmt.Errorf("已验收文件校验失败")
	}
	dir, e := os.MkdirTemp("", "edu-anim-export-")
	if e != nil {
		return "", e
	}
	defer os.RemoveAll(dir)
	input, output := filepath.Join(dir, "source.html"), filepath.Join(dir, "lesson.mp4")
	if e = os.WriteFile(input, html, 0600); e != nil {
		return "", e
	}
	script, e := filepath.Abs(env("ANIMATION_EXPORT_SCRIPT", "../animation-video/export.mjs"))
	if e != nil {
		return "", e
	}
	cmd := exec.CommandContext(ctx, env("ANIMATION_NODE_PATH", "node"), script, input, output)
	// Rendering receives only OS/browser settings, never model keys or database credentials.
	for _, key := range []string{"PATH", "PATHEXT", "SystemRoot", "WINDIR", "COMSPEC", "TEMP", "TMP", "TMPDIR", "HOME", "USERPROFILE", "LOCALAPPDATA", "APPDATA", "PROGRAMDATA", "PROGRAMFILES", "PROGRAMFILES(X86)", "ANIMATION_CHROME_PATH"} {
		if value, ok := os.LookupEnv(key); ok {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	// Windows needs to stop Chrome and FFmpeg children as well as the Node parent.
	if runtime.GOOS == "windows" {
		cmd.Cancel = func() error { return exec.Command("taskkill", "/PID", fmt.Sprint(cmd.Process.Pid), "/T", "/F").Run() }
	}
	releaseToken, restrictErr := restrictExportProcess(cmd)
	if restrictErr != nil {
		return "", fmt.Errorf("无法降低视频导出权限")
	}
	defer releaseToken()
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if e = cmd.Run(); e != nil {
		return "", fmt.Errorf("视频渲染失败")
	}
	b, e := os.ReadFile(output)
	if e != nil || len(b) < 24 || string(b[4:8]) != "ftyp" {
		return "", fmt.Errorf("视频产物无效")
	}
	path := storagePath(s, a.TenantID, fmt.Sprintf("%s-r%d-%s", id, job.Revision, job.Token), ".mp4")
	if e = s.writeStoredFile(ctx, path, b); e != nil {
		return "", e
	}
	return path, nil
}

func (s *Server) animationMP4File(w http.ResponseWriter, r *http.Request, a Actor) (bool, error) {
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return true, e
	}
	defer tx.Rollback(r.Context())
	var htmlEngine bool
	var revision int
	e = tx.QueryRow(r.Context(), "SELECT COALESCE(input->>'animation_engine'='html',false),COALESCE((animation->>'accepted_revision')::int,0) FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL", r.PathValue("id"), a.MemberID).Scan(&htmlEngine, &revision)
	if e != nil {
		return true, e
	}
	if !htmlEngine {
		return false, nil
	}
	if r.PathValue("version") != "" {
		revision, e = parseRevision(r)
		if e != nil {
			return true, e
		}
	}
	var path string
	e = tx.QueryRow(r.Context(), "SELECT mp4_path FROM artifact_revisions WHERE task_id=$1 AND revision=$2 AND mp4_path<>''", r.PathValue("id"), revision).Scan(&path)
	if e != nil {
		return true, fail(404, "该验收版本尚未导出 MP4")
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "attachment; filename=lesson.mp4")
	return true, s.serveStoredFile(w, r, path)
}
