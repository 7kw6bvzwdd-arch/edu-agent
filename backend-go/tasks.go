package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type TaskInput struct {
	PreviewFirst    bool   `json:"preview_first,omitempty"`
	AnimationEngine string `json:"animation_engine,omitempty"`
	SourceTaskID    string `json:"source_task_id,omitempty"`
	ParentID        string `json:"parent_id,omitempty"`
	ParentRevision  int    `json:"parent_revision,omitempty"`
	SlideIndex      int    `json:"slide_index,omitempty"`
	Instruction     string `json:"instruction,omitempty"`
	Type            string `json:"type"`
	Topic           string `json:"topic"`
	Subject         string `json:"subject"`
	Grade           string `json:"grade"`
	Textbook        string `json:"textbook"`
	Objectives      string `json:"objectives"`
	Pages           int    `json:"pages"`
	Theme           string `json:"theme"`
	UseKB           bool   `json:"use_kb"`
	UseWeb          bool   `json:"use_web"`
	IdempotencyKey  string `json:"idempotency_key"`
}
type Slide struct {
	ID       string           `json:"id,omitempty"`
	Picture  *TeachingPicture `json:"picture,omitempty"`
	Emphasis []string         `json:"emphasis,omitempty"`
	Focus    string           `json:"focus,omitempty"`
	Table    *TeachingTable   `json:"table,omitempty"`
	Figure   *TeachingFigure  `json:"figure,omitempty"`
	Scene    *AnimationScene  `json:"scene,omitempty"`
	Layout   string           `json:"layout,omitempty"`
	Section  string           `json:"section,omitempty"`
	Title    string           `json:"title"`
	Bullets  []string         `json:"bullets"`
	Notes    string           `json:"notes"`
	Formula  string           `json:"formula,omitempty"`
	Diagram  string           `json:"diagram,omitempty"`
	Chart    any              `json:"chart,omitempty"`
}
type Deck struct {
	AnimationQuality *AnimationQuality `json:"animation_quality,omitempty"`
	Title            string            `json:"title"`
	Theme            string            `json:"theme"`
	Lesson           string            `json:"lesson"`
	Slides           []Slide           `json:"slides"`
	NeedsReview      bool              `json:"needs_review"`
	Notice           string            `json:"notice"`
	Evaluation       any               `json:"evaluation,omitempty"`
	Script           string            `json:"script,omitempty"`
}
type Source struct {
	Filter      *PassageFilter `json:"filter,omitempty"`
	Hash        string         `json:"hash"`
	RetrievedAt string         `json:"retrieved_at"`
	ID          string         `json:"id"`
	DocumentID  string         `json:"document_id"`
	Title       string         `json:"title"`
	Content     string         `json:"content"`
	Location    string         `json:"location"`
	URL         string         `json:"url,omitempty"`
	Kind        string         `json:"kind"`
	Score       float64        `json:"score"`
}

func (s *Server) createTask(w http.ResponseWriter, r *http.Request, a Actor) error {
	var in TaskInput
	if e := decode(r, &in); e != nil {
		return e
	}
	in.Topic = strings.TrimSpace(in.Topic)
	if in.Type != "courseware" && in.Type != "animation" {
		return fail(400, "仅支持课件和教学动画")
	}
	perm := "courseware.generate"
	cost := int64(20)
	if in.Type == "animation" {
		perm = "animation.render"
		cost = 40
	}
	if !allowed(a, perm) {
		return fail(403, "没有生成权限")
	}
	if in.Topic == "" || len([]rune(in.Topic)) > 100 || len(in.IdempotencyKey) < 8 || len(in.IdempotencyKey) > 128 {
		return fail(400, "请填写课题并提供有效幂等键")
	}
	if !in.UseKB && !in.UseWeb {
		return fail(400, "至少选择一种资料来源")
	}
	maxPages := 20
	if in.Type == "animation" {
		maxPages = 12
	}
	if in.Pages < 8 || in.Pages > maxPages {
		in.Pages = 8
	}
	if !validTheme(in.Theme) {
		in.Theme = "forest"
	}
	if in.Type == "animation" {
		if in.AnimationEngine == "" {
			in.AnimationEngine = "manim"
		}
		if in.AnimationEngine != "manim" && in.AnimationEngine != "ark" {
			return fail(400, "animation_engine仅支持manim或ark")
		}
		if in.AnimationEngine == "ark" && s.cfg.Mode != "live" {
			return fail(503, "云视频仅支持live模式；sample模式不会调用真实视频服务，请选择manim样例")
		}
		if in.AnimationEngine == "ark" && !s.videoConfigured() {
			return fail(503, videoConfigMessage)
		}
	}
	if s.cfg.Mode == "live" && !s.agentConfigured(in.Type) {
		return fail(503, "当前制作智能体模型未配置，请设置对应COURSEWARE_LLM或ANIMATION_LLM配置，或默认LLM配置")
	}
	if in.UseWeb && in.SourceTaskID == "" && s.cfg.Mode == "live" && s.cfg.SearchKey == "" {
		return fail(503, "联网搜索服务未配置；可明确关闭联网后仅使用知识库")
	}
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	if in.SourceTaskID != "" {
		if in.Type != "animation" {
			return fail(400, "动画只能复用本人已完成课件的资料")
		}
		var source string
		e = tx.QueryRow(r.Context(), "SELECT id FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL AND type='courseware' AND status='succeeded' FOR SHARE", in.SourceTaskID, a.MemberID).Scan(&source)
		if e != nil {
			return e
		}
	}
	var balance, res int64
	var plan string
	e = tx.QueryRow(r.Context(), "SELECT balance,reserved,plan FROM members WHERE id=$1 FOR UPDATE", a.MemberID).Scan(&balance, &res, &plan)
	if e != nil {
		return e
	}
	var existing string
	var deleted bool
	e = tx.QueryRow(r.Context(), "SELECT id,deleted_at IS NOT NULL FROM tasks WHERE member_id=$1 AND idempotency_key=$2", a.MemberID, in.IdempotencyKey).Scan(&existing, &deleted)
	if e == nil {
		if deleted {
			return fail(409, "原任务已删除，请使用新的幂等键")
		}
		respond(w, 202, map[string]string{"id": existing})
		return nil
	}
	if e != pgx.ErrNoRows {
		return e
	}
	if plan == "" {
		return fail(403, "请先开通基础套餐")
	}
	if balance-res < cost {
		return fail(402, "成员可用积分不足，禁止生成")
	}
	if in.Type == "animation" {
		checkCtx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		if e := exec.CommandContext(checkCtx, "docker", "image", "inspect", s.cfg.AnimationImage).Run(); e != nil {
			return fail(503, "动画镜像尚未准备好，请先构建edu-manim:local")
		}
	}
	id := newID()
	_, e = tx.Exec(r.Context(), "UPDATE members SET reserved=reserved+$1 WHERE id=$2", cost, a.MemberID)
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), "INSERT INTO tasks(id,tenant_id,member_id,type,input,mode,cost,idempotency_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8)", id, a.TenantID, a.MemberID, in.Type, asJSON(in), s.cfg.Mode, cost, in.IdempotencyKey)
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), "INSERT INTO ledger(tenant_id,member_id,kind,delta,balance_after,reference) VALUES($1,$2,'reserve',0,$3,$4)", a.TenantID, a.MemberID, balance, id)
	if e != nil {
		return e
	}
	if e = event(r.Context(), tx, a.TenantID, id, "queued", "已预留成员积分，等待研究资料"); e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	respond(w, 202, map[string]string{"id": id})
	return nil
}
func (s *Server) listTasks(w http.ResponseWriter, r *http.Request, a Actor) error {
	q := r.URL.Query()
	view := q.Get("view")
	if view != "" && view != "works" && view != "interrupted" && view != "history" {
		return fail(400, "作品视图应为当前作品、待处理或全部记录")
	}
	limit, offset := 100, 0
	for key, dest := range map[string]*int{"limit": &limit, "offset": &offset} {
		if q.Has(key) {
			n, err := strconv.Atoi(q.Get(key))
			if err != nil || n < 0 || (key == "limit" && (n < 1 || n > 200)) {
				return fail(400, "limit应为1–200，offset应为非负整数")
			}
			*dest = n
		}
	}
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	// Keep the unqualified task feed compatible. Collection views hide superseded
	// retries even if the child was deleted, so deleting it cannot resurrect a card.
	v, e := queryMaps(r.Context(), tx, `SELECT id,COALESCE(NULLIF(display_title,''),input->>'topic','') AS display_title,type,status,stage,input,error,cost,mode,revision,created_at,updated_at
		FROM tasks t WHERE member_id=$1 AND deleted_at IS NULL
		AND ($2='' OR type=$2) AND ($3='' OR status=$3)
		AND ($4='' OR strpos(lower(concat_ws(' ',display_title,input->>'topic',input->>'subject',input->>'grade',result->>'title')),lower($4))>0)
		AND ($7<>'works' OR status NOT IN ('failed','cancelled'))
		AND ($7<>'interrupted' OR status IN ('failed','cancelled'))
		AND ($7 NOT IN ('works','interrupted') OR NOT (t.type='courseware' AND t.status='failed' AND EXISTS (
			SELECT 1 FROM tasks child WHERE child.tenant_id=t.tenant_id AND child.member_id=t.member_id
			AND child.type='courseware' AND child.idempotency_key='retry-'||t.id
		)))
		ORDER BY created_at DESC,id DESC LIMIT $5 OFFSET $6`, a.MemberID, q.Get("type"), q.Get("status"), strings.TrimSpace(q.Get("q")), limit, offset, view)
	if e != nil {
		return e
	}
	respond(w, 200, v)
	return nil
}
func (s *Server) getTask(w http.ResponseWriter, r *http.Request, a Actor) error {
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	v, e := queryMaps(r.Context(), tx, "SELECT id,COALESCE(NULLIF(display_title,''),input->>'topic','') AS display_title,type,status,stage,input,result,sources,error,mode,cost,charge_state,revision,created_at,updated_at FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL", r.PathValue("id"), a.MemberID)
	if e != nil {
		return e
	}
	if len(v) == 0 {
		return pgx.ErrNoRows
	}
	ev, e := queryMaps(r.Context(), tx, "SELECT id,stage,message,created_at FROM task_events WHERE task_id=$1 ORDER BY id", r.PathValue("id"))
	if e != nil {
		return e
	}
	v[0]["events"] = ev
	if v[0]["type"] == "courseware" {
		var d Deck
		if err := json.Unmarshal(asJSON(v[0]["result"]), &d); err == nil {
			meta, err := slideReviews(r.Context(), tx, r.PathValue("id"), d)
			if err != nil {
				return err
			}
			v[0]["slide_reviews"] = meta
			// Query only availability; do not expose server filesystem paths.
			var pptxPath string
			if err := tx.QueryRow(r.Context(), "SELECT pptx_path FROM tasks WHERE id=$1", r.PathValue("id")).Scan(&pptxPath); err != nil {
				return err
			}
			_, err = os.Stat(filepath.Join(pptxPath+".slides", "slide-1.png"))
			v[0]["preview_images_available"] = pptxPath != "" && err == nil
		}
	}
	respond(w, 200, v[0])
	return nil
}
func (s *Server) renameTask(w http.ResponseWriter, r *http.Request, a Actor) error {
	var in struct {
		Title string `json:"title"`
	}
	if e := decode(r, &in); e != nil {
		return e
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || len([]rune(in.Title)) > 100 {
		return fail(400, "作品名称需为1–100字")
	}
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var id string
	e = tx.QueryRow(r.Context(), "UPDATE tasks SET display_title=$1,updated_at=now() WHERE id=$2 AND member_id=$3 AND deleted_at IS NULL RETURNING id", in.Title, r.PathValue("id"), a.MemberID).Scan(&id)
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), "INSERT INTO audit_logs(tenant_id,member_id,action,target,detail) VALUES($1,$2,'task.rename',$3,$4)", a.TenantID, a.MemberID, id, asJSON(in))
	if e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	respond(w, 200, map[string]string{"id": id, "display_title": in.Title})
	return nil
}

func (s *Server) deleteTask(w http.ResponseWriter, r *http.Request, a Actor) error {
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	id := r.PathValue("id")
	var status, charge string
	e = tx.QueryRow(r.Context(), "SELECT status,charge_state FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL FOR UPDATE", id, a.MemberID).Scan(&status, &charge)
	if e != nil {
		return e
	}
	if charge == "reserved" || (status != "succeeded" && status != "degraded" && status != "failed" && status != "cancelled") {
		return fail(409, "任务仍在进行，请先取消任务再删除")
	}
	// Source/rewrite creation holds a share lock on this task until its child is committed.
	var active bool
	e = tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM tasks WHERE member_id=$1 AND deleted_at IS NULL AND charge_state='reserved' AND (input->>'parent_id'=$2 OR input->>'source_task_id'=$2))", a.MemberID, id).Scan(&active)
	if e != nil {
		return e
	}
	if active {
		return fail(409, "关联的改写或动画任务仍在进行，请先取消后再删除")
	}
	// Revisions contain copies of the original deck; hide these too, retaining all accounting rows.
	_, e = tx.Exec(r.Context(), "UPDATE tasks SET deleted_at=now(),updated_at=now() WHERE member_id=$1 AND deleted_at IS NULL AND (id=$2 OR (type='revision' AND input->>'parent_id'=$2))", a.MemberID, id)
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), "INSERT INTO audit_logs(tenant_id,member_id,action,target) VALUES($1,$2,'task.delete',$3)", a.TenantID, a.MemberID, id)
	if e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	respond(w, 200, map[string]bool{"ok": true})
	return nil
}

func validTheme(theme string) bool {
	switch theme {
	case "forest", "violet", "ocean", "midnight", "paper", "coral", "classroom":
		return true
	}
	return false
}

func validateDeck(d Deck) error {
	return validateDeckMinimum(d, 8)
}
func validateDeckMinimum(d Deck, minimum int) error {
	if len(d.Slides) < minimum || len(d.Slides) > 100 || strings.TrimSpace(d.Title) == "" || strings.TrimSpace(d.Lesson) == "" {
		return fail(400, "无效课件内容")
	}
	if !validTheme(d.Theme) {
		return fail(400, "无效视觉主题")
	}
	imageSize := 0
	for _, sl := range d.Slides {
		if sl.Picture != nil {
			imageSize += len(sl.Picture.DataURL)
		}
		if err := validateTeachingMedia(sl); err != nil {
			return err
		}
		if len([]rune(sl.Section)) > 80 {
			return fail(400, "章节名称最多80字")
		}
		switch sl.Layout {
		case "", "cover", "concept", "steps", "comparison", "example", "exercise", "summary":
		default:
			return fail(400, "无效页面布局")
		}
		if len([]rune(sl.Title)) > 70 || sl.Title == "" || len(sl.Bullets) > 6 {
			return fail(400, "标题或内容超出排版限制")
		}
		for _, b := range sl.Bullets {
			if len([]rune(b)) > 160 {
				return fail(400, "每条内容最多160字")
			}
		}
	}
	if imageSize > 1200000 {
		return fail(400, "整套课件图片总量过大，请减少图片或压缩后再上传")
	}
	return nil
}
func (s *Server) confirmTask(w http.ResponseWriter, r *http.Request, a Actor) error {
	var in struct {
		Result       Deck `json:"result"`
		AcceptReview bool `json:"accept_review"`
		Revision     int  `json:"revision"`
	}
	if e := decode(r, &in); e != nil {
		return e
	}
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var status, kind string
	var revision int
	var old Deck
	var original TaskInput
	var sources []Source
	e = tx.QueryRow(r.Context(), "SELECT status,type,revision,result,input,sources FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL FOR UPDATE", r.PathValue("id"), a.MemberID).Scan(&status, &kind, &revision, &old, &original, &sources)
	if e != nil {
		return e
	}
	permission := "courseware.generate"
	if kind == "animation" {
		permission = "animation.render"
	}
	if !allowed(a, permission) {
		return fail(403, "没有当前制作权限")
	}
	if kind == "courseware" {
		in.Result.Notice = old.Notice
		in.Result = arrangeCourseware(in.Result)
	}
	if e := validateDeck(in.Result); e != nil {
		return e
	}
	if kind == "courseware" && original.PreviewFirst {
		if err := coursewareChatIdle(r.Context(), tx, r.PathValue("id"), ""); err != nil {
			return err
		}
		if err := allSlidesApproved(r.Context(), tx, r.PathValue("id"), in.Result); err != nil {
			return err
		}
	}
	if kind == "animation" && len(in.Result.Slides) > 12 {
		return fail(400, "动画分镜应为8–12页")
	}
	if status != "awaiting_confirmation" {
		return fail(409, "任务已确认或已经结束")
	}
	if in.Revision != revision {
		return fail(409, "内容已更新，请刷新")
	}
	if old.NeedsReview && !in.AcceptReview {
		return fail(400, "资料不足，请明确确认继续生成待核验课件")
	}
	in.Result.NeedsReview = old.NeedsReview || in.Result.NeedsReview
	if kind == "animation" {
		in.Result.Notice = old.Notice
	}
	in.Result.Evaluation = nil
	in.Result.Script = ""
	in.Result.AnimationQuality = nil
	if kind == "animation" {
		seconds, _ := strconv.ParseFloat(s.cfg.VideoDuration, 64)
		if err := validateAnimationPlan(in.Result, sources, seconds, original.AnimationEngine == "ark"); err != nil {
			return err
		}
		in.Result.AnimationQuality = &AnimationQuality{Version: "scene-v1", Status: "plan_checked", Checks: []string{"教师确认后的分镜已重新检查；旧评审不代表修改后的内容"}}
	} else if _, err := s.renderCourseware(r.Context(), &in.Result); err != nil {
		return err
	}
	if kind == "courseware" && original.PreviewFirst {
		if err := allSlidesApproved(r.Context(), tx, r.PathValue("id"), in.Result); err != nil {
			return err
		}
	}
	_, e = tx.Exec(r.Context(), "UPDATE tasks SET status='confirmed',stage='confirmed',result=$1,revision=revision+1,updated_at=now() WHERE id=$2", asJSON(in.Result), r.PathValue("id"))
	if e != nil {
		return e
	}
	if e = event(r.Context(), tx, a.TenantID, r.PathValue("id"), "confirmed", "教师已确认大纲与费用，开始制作文件"); e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	respond(w, 202, map[string]bool{"ok": true})
	return nil
}
func (s *Server) cancelTask(w http.ResponseWriter, r *http.Request, a Actor) error {
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	var id string
	e = tx.QueryRow(r.Context(), "SELECT id FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL", r.PathValue("id"), a.MemberID).Scan(&id)
	tx.Rollback(r.Context())
	if e != nil {
		return e
	}
	if e = s.finish(r.Context(), a.TenantID, id, "cancelled", "已取消并释放未结算积分", "", nil); e != nil {
		return e
	}
	respond(w, 200, map[string]bool{"ok": true})
	return nil
}
func (s *Server) download(w http.ResponseWriter, r *http.Request, a Actor) error {
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var pptx, mp4, status string
	var d Deck
	e = tx.QueryRow(r.Context(), "SELECT pptx_path,mp4_path,result,status FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL", r.PathValue("id"), a.MemberID).Scan(&pptx, &mp4, &d, &status)
	if e != nil {
		return e
	}
	if r.PathValue("version") != "" {
		version, err := parseRevision(r)
		if err != nil {
			return err
		}
		e = tx.QueryRow(r.Context(), `SELECT ar.pptx_path,ar.result FROM artifact_revisions ar JOIN tasks t ON t.id=ar.task_id AND t.tenant_id=ar.tenant_id
			WHERE t.id=$1 AND t.member_id=$2 AND t.deleted_at IS NULL AND t.type='courseware' AND ar.revision=$3 AND ar.pptx_path<>''`, r.PathValue("id"), a.MemberID, version).Scan(&pptx, &d)
		if e != nil {
			return e
		}
		mp4 = ""
	}
	format := r.PathValue("format")
	if format == "md" {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=lesson.md")
		fmt.Fprint(w, d.Lesson)
		return nil
	}
	path := pptx
	if format == "mp4" {
		path = mp4
	} else if format != "pptx" {
		return fail(400, "不支持的文件格式")
	}
	if path == "" || (r.PathValue("version") == "" && status != "succeeded" && status != "degraded" && !(format == "mp4" && status == "awaiting_acceptance")) {
		return fail(404, "文件尚未生成")
	}
	w.Header().Set("Content-Disposition", "attachment; filename=lesson."+format)
	http.ServeFile(w, r, path)
	return nil
}
func (s *Server) events(w http.ResponseWriter, r *http.Request, a Actor) error {
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	var owner int64
	e = tx.QueryRow(r.Context(), "SELECT member_id FROM tasks WHERE id=$1 AND deleted_at IS NULL", r.PathValue("id")).Scan(&owner)
	tx.Rollback(r.Context())
	if e != nil {
		return e
	}
	if owner != a.MemberID {
		return pgx.ErrNoRows
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fail(500, "不支持流式连接")
	}
	last := int64(0)
	fmt.Sscan(r.Header.Get("Last-Event-ID"), &last)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		fresh, e := s.actor(r)
		if e != nil || fresh.MemberID != a.MemberID {
			return nil
		}
		tx, e := s.tx(r.Context(), a.TenantID)
		if e != nil {
			return e
		}
		// Recheck on every poll: authorization at connection time does not survive deletion.
		var visible bool
		e = tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL)", r.PathValue("id"), a.MemberID).Scan(&visible)
		if e != nil || !visible {
			tx.Rollback(r.Context())
			return e
		}
		rows, e := queryMaps(r.Context(), tx, "SELECT id,stage,message,created_at FROM task_events WHERE task_id=$1 AND id>$2 AND EXISTS(SELECT 1 FROM tasks WHERE id=$1 AND member_id=$3 AND deleted_at IS NULL) ORDER BY id", r.PathValue("id"), last, a.MemberID)
		tx.Rollback(r.Context())
		if e != nil {
			return e
		}
		for _, v := range rows {
			last = v["id"].(int64)
			fmt.Fprintf(w, "id: %d\ndata: %s\n\n", last, asJSON(v))
		}
		fmt.Fprint(w, ": heartbeat\n\n")
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return nil
		case <-ticker.C:
		}
	}
}
func (s *Server) worker(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if e := s.workOne(ctx); e != nil {
			fmt.Fprintln(os.Stderr, "worker:", e)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Server) workOne(ctx context.Context) error {
	// The privileged connection only enumerates tenants; every business operation uses RLS.
	rows, e := s.db.Query(ctx, "SELECT id FROM tenants ORDER BY id")
	if e != nil {
		return e
	}
	tenants := []int64{}
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		tenants = append(tenants, id)
	}
	rows.Close()
	for _, tid := range tenants {
		tx, e := s.tx(ctx, tid)
		if e != nil {
			return e
		}
		expired, e := queryMaps(ctx, tx, "SELECT id FROM tasks WHERE deleted_at IS NULL AND charge_state='reserved' AND ((status IN ('researching','rendering','expanding') AND lease_until<now()) OR (status IN ('awaiting_preview','awaiting_confirmation','awaiting_acceptance') AND updated_at<now()-interval '1 day'))")
		if e != nil {
			tx.Rollback(ctx)
			return e
		}
		tx.Rollback(ctx)
		for _, v := range expired {
			s.finish(ctx, tid, v["id"].(string), "failed", "执行或确认等待超时，积分已释放", "", nil)
		}
		tx, e = s.tx(ctx, tid)
		if e != nil {
			return e
		}
		var id, status, kind, mode string
		var member int64
		var in TaskInput
		var result Deck
		e = tx.QueryRow(ctx, "SELECT id,status,type,member_id,input,result,mode FROM tasks WHERE deleted_at IS NULL AND status IN ('pending','confirmed','expand_pending') ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1").Scan(&id, &status, &kind, &member, &in, &result, &mode)
		if e == pgx.ErrNoRows {
			tx.Rollback(ctx)
			continue
		}
		if e != nil {
			tx.Rollback(ctx)
			return e
		}
		stage := "researching"
		if status == "confirmed" {
			stage = "rendering"
		}
		if status == "expand_pending" {
			stage = "expanding"
		}
		workLimit := 9 * time.Minute
		if stage == "rendering" && kind == "animation" && in.AnimationEngine == "ark" {
			workLimit = s.videoWaitLimit() + 2*time.Minute
		}
		_, e = tx.Exec(ctx, "UPDATE tasks SET status=$1,stage=$1,lease_until=now()+$3*interval '1 second',attempt=attempt+1,updated_at=now() WHERE id=$2", stage, id, int((workLimit + time.Minute).Seconds()))
		if e == nil {
			e = event(ctx, tx, tid, id, stage, map[string]string{"researching": "正在检索和组织教学方案", "rendering": "正在生成真实文件", "expanding": "前三页已确认，正在生成剩余课件，保留已审核内容"}[stage])
		}
		if e != nil {
			tx.Rollback(ctx)
			return e
		}
		if e = tx.Commit(ctx); e != nil {
			return e
		}
		workCtx, cancel := context.WithTimeout(ctx, workLimit)
		a := Actor{MemberID: member, TenantID: tid}
		if mode != s.cfg.Mode {
			e = fmt.Errorf("运行模式已切换，请在当前模式重新提交任务")
		} else {
			e = s.processTask(workCtx, a, id, kind, stage, in, result)
		}
		cancel()
		if e != nil {
			finishCtx, c := context.WithTimeout(context.Background(), 10*time.Second)
			s.finish(finishCtx, tid, id, "failed", e.Error(), "", nil)
			c()
		}
		return nil
	}
	return nil
}
func (s *Server) processTask(ctx context.Context, a Actor, id, kind, stage string, in TaskInput, d Deck) error {
	if e := s.checkWorkPermission(ctx, a, kind); e != nil {
		return e
	}
	if kind == "revision" {
		return s.processRevision(ctx, a, id, in, d)
	}
	if kind == "animation" {
		return s.runAnimationAgent(ctx, a, id, stage, in, d)
	}
	return s.runCoursewareAgent(ctx, a, id, stage, in, d)
}

func (s *Server) renderPPTX(ctx context.Context, d Deck) ([]byte, error) {
	for _, sl := range d.Slides {
		if d.Theme == "classroom" || sl.Table != nil || sl.Figure != nil || sl.Picture != nil || sl.Focus != "" || len(sl.Emphasis) > 0 {
			if err := s.requireTeachingMediaRenderer(ctx); err != nil {
				return nil, err
			}
			break
		}
	}
	return s.postBytes(ctx, s.cfg.RenderURL+"/render/pptx", d, "")
}

func (s *Server) checkWorkPermission(ctx context.Context, a Actor, kind string) error {
	tx, e := s.tx(ctx, a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var current Actor
	var active bool
	e = tx.QueryRow(ctx, "SELECT m.role,m.permissions,m.active AND u.active FROM members m JOIN users u ON u.id=m.user_id WHERE m.id=$1", a.MemberID).Scan(&current.Role, &current.Permissions, &active)
	if e != nil {
		return e
	}
	perm := "courseware.generate"
	if kind == "animation" {
		perm = "animation.render"
	}
	if !active || !allowed(current, perm) {
		return fmt.Errorf("成员已停用或制作权限已撤销，积分已释放")
	}
	return nil
}
