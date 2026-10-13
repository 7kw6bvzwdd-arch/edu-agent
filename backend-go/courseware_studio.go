package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

type coursewareState struct {
	Deck               Deck
	Input              TaskInput
	Sources            []Source
	Status, Mode, Path string
	Revision           int
}
type slideReview struct {
	ID       string `json:"id"`
	Hash     string `json:"hash"`
	Approved bool   `json:"approved"`
	Vote     int    `json:"vote"`
	Reason   string `json:"reason"`
}

func ownedCourseware(ctx context.Context, tx pgx.Tx, a Actor, id, lock string) (coursewareState, error) {
	var st coursewareState
	var kind string
	err := tx.QueryRow(ctx, "SELECT type,result,input,sources,status,mode,pptx_path,revision FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL "+lock, id, a.MemberID).Scan(&kind, &st.Deck, &st.Input, &st.Sources, &st.Status, &st.Mode, &st.Path, &st.Revision)
	if err == nil && kind != "courseware" {
		err = fail(409, "此功能仅支持课件")
	}
	return st, err
}
func slideContentHash(d Deck, sl Slide) string {
	return fmt.Sprintf("%x", sha256.Sum256(append([]byte(d.Title+"\n"+d.Theme+"\n"), asJSON(sl)...)))
}
func slideReviews(ctx context.Context, tx pgx.Tx, id string, d Deck) ([]slideReview, error) {
	reviews := []slideReview{}
	for _, sl := range d.Slides {
		meta := slideReview{ID: sl.ID, Hash: slideContentHash(d, sl)}
		err := tx.QueryRow(ctx, "SELECT approved,vote,reason FROM courseware_feedback WHERE task_id=$1 AND slide_id=$2 AND content_hash=$3", id, meta.ID, meta.Hash).Scan(&meta.Approved, &meta.Vote, &meta.Reason)
		if err != nil && err != pgx.ErrNoRows {
			return nil, err
		}
		reviews = append(reviews, meta)
	}
	return reviews, nil
}
func allSlidesApproved(ctx context.Context, tx pgx.Tx, id string, d Deck) error {
	if d.Plan != nil {
		var mode string
		if err := tx.QueryRow(ctx, "SELECT mode FROM tasks WHERE id=$1", id).Scan(&mode); err != nil {
			return err
		}
		for _, sl := range d.Slides {
			check := d.PageChecks[sl.ID]
			if check.Hash != slideContentHash(d, sl) || !pageCheckPassed(check, mode == "sample") {
				return fail(409, "页面检查尚未通过，请保存并完成内容与画面检查后继续")
			}
		}
	}
	meta, err := slideReviews(ctx, tx, id, d)
	if err != nil {
		return err
	}
	for i, m := range meta {
		if !m.Approved || m.Vote < 0 {
			return fail(409, fmt.Sprintf("第%d页尚未审核通过，或存在未处理的点踩；请修改并重新审核", i+1))
		}
	}
	return nil
}
func assignSlideIDs(d *Deck) {
	seen := map[string]bool{}
	for i := range d.Slides {
		id := d.Slides[i].ID
		if len(id) < 8 || len(id) > 64 || seen[id] {
			id = newID()
			d.Slides[i].ID = id
		}
		seen[id] = true
	}
}

// Actual Office snapshots are optional on hosts without Office. The PPTX still
// must render successfully; the UI explicitly labels any HTML fallback.
var officePreviewMu sync.Mutex

func (s *Server) snapshotCourseware(ctx context.Context, path string, samplePages ...int) bool {
	if ctx.Err() != nil || runtime.GOOS != "windows" {
		return false
	}
	script, err := filepath.Abs("../scripts/render-courseware-preview.ps1")
	if err != nil {
		return false
	}
	officePreviewMu.Lock()
	defer officePreviewMu.Unlock()
	if ctx.Err() != nil {
		return false
	}
	// Exact PPTX bytes include slide count/position and footer; never substitute
	// a one-page screenshot for a full-deck page with different numbering.
	pptx, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	sample := 0
	if len(samplePages) > 0 {
		sample = samplePages[0]
	}
	key := fmt.Sprintf("office:%x:%d", sha256.Sum256(pptx), sample)
	if data := artifactCacheGet(ctx, key); data != nil {
		var pages map[string][]byte
		if json.Unmarshal(data, &pages) == nil && os.MkdirAll(path+".slides", 0700) == nil {
			for name, png := range pages {
				if os.WriteFile(filepath.Join(path+".slides", name), png, 0600) != nil {
					return false
				}
			}
			return ctx.Err() == nil
		}
	}
	defer coursewareMeasure(ctx, "office_snapshot")()

	c, stop := context.WithTimeout(ctx, 60*time.Second)
	defer stop()
	cmd := exec.CommandContext(c, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script, "-Path", path, "-OutputDirectory", path+".slides")
	if len(samplePages) > 0 {
		cmd.Args = append(cmd.Args, "-SamplePages", fmt.Sprint(samplePages[0]))
	}

	if cmd.Run() != nil || ctx.Err() != nil {
		return false
	}
	pages := map[string][]byte{}
	entries, err := os.ReadDir(path + ".slides")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "slide-") && strings.HasSuffix(entry.Name(), ".png") {
			b, err := os.ReadFile(filepath.Join(path+".slides", entry.Name()))
			if err != nil {
				return false
			}
			pages[entry.Name()] = b
		}
	}
	if len(pages) == 0 {
		return false
	}
	artifactCachePut(ctx, key, asJSON(pages))
	return true
}

func (s *Server) previewArtifact(ctx context.Context, a Actor, id string, d *Deck) (string, error) {
	if err := validateDeckMinimum(*d, 3); err != nil {
		return "", err
	}
	threePages := len(d.Slides) == 3
	path, err := s.savePPTX(ctx, a.TenantID, id+"-preview-"+newID(), d)
	if err == nil && threePages && len(d.Slides) != 3 {
		return "", fail(422, "前三页内容过密产生续页，请缩短显示文字、保留详细讲解备注；没有截断或覆盖已保存预览")
	}
	return path, err
}

func (s *Server) saveCoursewareDraft(w http.ResponseWriter, r *http.Request, a Actor) error {
	defer s.queueDeletedCleanup(a.TenantID, r.PathValue("id"), false)
	var body struct {
		Result   Deck `json:"result"`
		Revision int  `json:"revision"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	if len(body.Result.Slides) > 100 {
		return fail(400, "课件内容超过处理容量，请保留当前草稿并整理内容")
	}
	tx, err := s.tx(r.Context(), a.TenantID)
	if err != nil {
		return err
	}
	st, err := ownedCourseware(r.Context(), tx, a, r.PathValue("id"), "")
	tx.Rollback(r.Context())
	if err != nil {
		return err
	}
	if !st.Input.PreviewFirst || (st.Status != "awaiting_preview" && st.Status != "awaiting_confirmation") || st.Revision != body.Revision {
		return fail(409, "课件正在制作、内容已更新或不在草稿阶段，请刷新后重试")
	}
	body.Result.Notice, body.Result.NeedsReview, body.Result.Evaluation = st.Deck.Notice, true, nil
	body.Result.Script = ""
	body.Result.Generation = nil
	preserveCoursewareChecks(&body.Result, st.Deck)
	body.Result = arrangeCourseware(body.Result)
	if st.Status == "awaiting_preview" && len(body.Result.Slides) != 3 {
		return fail(422, "预览阶段仅保存前三页；复杂内容请分配到剩余页面，不丢弃数据")
	}
	if st.Status == "awaiting_confirmation" {
		if err := validateDeck(body.Result); err != nil {
			return err
		}
	}
	if err := coursewareSourceError(body.Result, st.Sources); err != nil {
		return err
	}
	assignSlideIDs(&body.Result)
	s.checkEditedPages(r.Context(), a, r.PathValue("id"), st.Input, &body.Result)
	path, err := s.previewArtifact(r.Context(), a, r.PathValue("id"), &body.Result)
	if err != nil {
		return err
	}
	if st.Status == "awaiting_preview" && len(body.Result.Slides) != 3 {
		return fail(422, "本页内容过密产生续页；请缩短显示文字、将完整讲解留在备注，前三页不能被截断")
	}
	tx, err = s.tx(r.Context(), a.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	current, err := ownedCourseware(r.Context(), tx, a, r.PathValue("id"), "FOR UPDATE")
	if err != nil {
		return err
	}
	if current.Revision != body.Revision || current.Status != st.Status {
		return fail(409, "草稿已更新或已进入生成阶段；当前修改没有覆盖旧版")
	}
	if err := coursewareChatIdle(r.Context(), tx, r.PathValue("id"), ""); err != nil {
		return err
	}
	if err := invalidateChangedChecks(r.Context(), tx, r.PathValue("id"), body.Result, st.Deck); err != nil {
		return err
	}
	if err = archiveRevision(r.Context(), tx, a.TenantID, r.PathValue("id")); err != nil {
		return err
	}
	_, err = tx.Exec(r.Context(), "UPDATE tasks SET result=$1,pptx_path=$2,revision=revision+1,updated_at=now() WHERE id=$3", asJSON(body.Result), path, r.PathValue("id"))
	if err != nil {
		return err
	}
	if err = archiveRevision(r.Context(), tx, a.TenantID, r.PathValue("id")); err != nil {
		return err
	}
	if err = event(r.Context(), tx, a.TenantID, r.PathValue("id"), "edited", "草稿已免费保存并更新预览；有变化的页面需要重新审核"); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	respond(w, 200, map[string]any{"revision": body.Revision + 1})
	return nil
}

func (s *Server) reviewCoursewareSlide(w http.ResponseWriter, r *http.Request, a Actor) error {
	return s.recordSlideFeedback(w, r, a, true)
}
func (s *Server) feedbackCoursewareSlide(w http.ResponseWriter, r *http.Request, a Actor) error {
	return s.recordSlideFeedback(w, r, a, false)
}
func (s *Server) recordSlideFeedback(w http.ResponseWriter, r *http.Request, a Actor, review bool) error {
	var body struct {
		Revision int    `json:"revision"`
		Hash     string `json:"hash"`
		Vote     int    `json:"vote"`
		Reason   string `json:"reason"`
		Approved bool   `json:"approved"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	body.Reason = strings.TrimSpace(body.Reason)
	if body.Vote < -1 || body.Vote > 1 || len([]rune(body.Reason)) > 400 || (!review && body.Vote < 0 && body.Reason == "") {
		return fail(400, "点踩请填写原因（最多400字），反馈只能为赞、踩或取消")
	}
	tx, err := s.tx(r.Context(), a.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	st, err := ownedCourseware(r.Context(), tx, a, r.PathValue("id"), "FOR UPDATE")
	if err != nil {
		return err
	}
	if st.Revision != body.Revision || (st.Status != "awaiting_preview" && st.Status != "awaiting_confirmation" && st.Status != "succeeded") {
		return fail(409, "当前版本已变化或正在生成，不能审核或评价")
	}
	found := false
	for _, sl := range st.Deck.Slides {
		if sl.ID == r.PathValue("slide") && body.Hash == slideContentHash(st.Deck, sl) {
			found = true
		}
	}
	if !found {
		return fail(409, "页面内容已改变，请先保存并评价最新版本")
	}
	if review && body.Approved && st.Deck.Plan != nil {
		check := st.Deck.PageChecks[r.PathValue("slide")]
		if check.Hash != body.Hash || !pageCheckPassed(check, s.cfg.Mode == "sample") {
			return fail(409, "本页检查尚未通过，请先保存修改并完成内容与画面检查")
		}
	}
	if err := coursewareChatIdle(r.Context(), tx, r.PathValue("id"), ""); err != nil {
		return err
	}
	_, err = tx.Exec(r.Context(), "INSERT INTO courseware_feedback(tenant_id,task_id,slide_id,content_hash) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING", a.TenantID, r.PathValue("id"), r.PathValue("slide"), body.Hash)
	if err != nil {
		return err
	}
	if review {
		var vote int
		if err = tx.QueryRow(r.Context(), "SELECT vote FROM courseware_feedback WHERE task_id=$1 AND slide_id=$2 AND content_hash=$3", r.PathValue("id"), r.PathValue("slide"), body.Hash).Scan(&vote); err != nil {
			return err
		}
		if body.Approved && vote < 0 {
			return fail(409, "这页仍有点踩，先修改并重新审核，或明确撤销反馈")
		}
		_, err = tx.Exec(r.Context(), "UPDATE courseware_feedback SET approved=$1,updated_at=now() WHERE task_id=$2 AND slide_id=$3 AND content_hash=$4", body.Approved, r.PathValue("id"), r.PathValue("slide"), body.Hash)
	} else {
		_, err = tx.Exec(r.Context(), "UPDATE courseware_feedback SET vote=$1,reason=$2,approved=CASE WHEN $1=-1 THEN false ELSE approved END,updated_at=now() WHERE task_id=$3 AND slide_id=$4 AND content_hash=$5", body.Vote, body.Reason, r.PathValue("id"), r.PathValue("slide"), body.Hash)
	}
	if err != nil {
		return err
	}
	label := "页面审核已记录；后续修改会使本次确认失效"
	if !review {
		label = "页面反馈已记录；点踩撤销本页审核，作为后续修订约束，不扣教师积分"
	}
	if err = event(r.Context(), tx, a.TenantID, r.PathValue("id"), "feedback", label); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	respond(w, 200, map[string]bool{"ok": true})
	return nil
}

func (s *Server) confirmCoursewarePreview(w http.ResponseWriter, r *http.Request, a Actor) error {
	var body struct {
		Revision int `json:"revision"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	tx, err := s.tx(r.Context(), a.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	st, err := ownedCourseware(r.Context(), tx, a, r.PathValue("id"), "FOR UPDATE")
	if err != nil {
		return err
	}
	if st.Input.PreviewFirst && (st.Status == "expand_pending" || st.Status == "expanding") && body.Revision == st.Revision {
		respond(w, 202, map[string]bool{"ok": true})
		return nil
	}
	if !st.Input.PreviewFirst || st.Status != "awaiting_preview" || st.Revision != body.Revision || len(st.Deck.Slides) != 3 {
		return fail(409, "仅当前前三页预览可继续生成，请刷新后重试")
	}
	if err := coursewareChatIdle(r.Context(), tx, r.PathValue("id"), ""); err != nil {
		return err
	}
	if err := allSlidesApproved(r.Context(), tx, r.PathValue("id"), st.Deck); err != nil {
		return err
	}
	_, err = tx.Exec(r.Context(), "UPDATE tasks SET status='expand_pending',stage='expand_pending',updated_at=now() WHERE id=$1", r.PathValue("id"))
	if err != nil {
		return err
	}
	if err = event(r.Context(), tx, a.TenantID, r.PathValue("id"), "expand_pending", "前三页已逐页确认，只生成剩余页面；保持原20积分预留，不新增业务收费"); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	respond(w, 202, map[string]bool{"ok": true})
	return nil
}

func (s *Server) coursewareSlidePreview(w http.ResponseWriter, r *http.Request, a Actor) error {
	tx, err := s.tx(r.Context(), a.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	st, err := ownedCourseware(r.Context(), tx, a, r.PathValue("id"), "")
	if err != nil {
		return err
	}
	if revision := r.URL.Query().Get("revision"); revision != strconv.Itoa(st.Revision) {
		return fail(409, "预览版本已变化，请重新加载")
	}
	pages, err := s.coursewarePreviewPages(r.Context(), tx, r.PathValue("id"), st.Deck, st.Path)
	if err != nil {
		return err
	}
	if path := pages[r.PathValue("slide")]; path != "" {
		w.Header().Set("Content-Type", "image/png")
		return s.serveStoredFile(w, r, path)
	}
	return pgx.ErrNoRows
}
func (s *Server) coursewarePreviewDownload(w http.ResponseWriter, r *http.Request, a Actor) error {
	tx, err := s.tx(r.Context(), a.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	st, err := ownedCourseware(r.Context(), tx, a, r.PathValue("id"), "")
	if err != nil {
		return err
	}
	if st.Path == "" || (st.Status != "awaiting_preview" && st.Status != "awaiting_confirmation" && st.Status != "succeeded") {
		return fail(404, "预览文件尚未就绪")
	}
	w.Header().Set("Content-Disposition", "attachment; filename=preview.pptx")
	return s.serveStoredFile(w, r, st.Path)
}
