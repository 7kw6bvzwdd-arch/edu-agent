package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type TaskInput struct {
	AnimationPicture *TeachingPicture  `json:"animation_picture,omitempty"`
	Chapter          string            `json:"chapter,omitempty"`
	MaterialID       string            `json:"material_id,omitempty"`
	MaterialStart    int               `json:"material_start,omitempty"`
	MaterialEnd      int               `json:"material_end,omitempty"`
	Curriculum       []Page            `json:"curriculum,omitempty"`
	ReferenceID      string            `json:"reference_id,omitempty"`
	Reference        *ReferenceProfile `json:"reference,omitempty"`
	Detail           string            `json:"detail,omitempty"`
	LessonMinutes    int               `json:"lesson_minutes,omitempty"`
	StudentLevel     string            `json:"student_level,omitempty"`
	ReferenceMode    string            `json:"reference_mode,omitempty"`
	Requirements     string            `json:"requirements,omitempty"`
	PreviewFirst     bool              `json:"preview_first,omitempty"`
	AnimationEngine  string            `json:"animation_engine,omitempty"`
	SourceTaskID     string            `json:"source_task_id,omitempty"`
	ParentID         string            `json:"parent_id,omitempty"`
	ParentRevision   int               `json:"parent_revision,omitempty"`
	SlideIndex       int               `json:"slide_index,omitempty"`
	Instruction      string            `json:"instruction,omitempty"`
	Type             string            `json:"type"`
	Topic            string            `json:"topic"`
	Subject          string            `json:"subject"`
	Grade            string            `json:"grade"`
	Textbook         string            `json:"textbook"`
	Objectives       string            `json:"objectives"`
	Pages            int               `json:"pages"`
	Theme            string            `json:"theme"`
	UseKB            bool              `json:"use_kb"`
	UseWeb           bool              `json:"use_web"`
	IdempotencyKey   string            `json:"idempotency_key"`
}
type Slide struct {
	CanvasObjects   []CanvasObject          `json:"canvas_objects,omitempty"`
	CanvasOrder     []string                `json:"canvas_order,omitempty"`
	CanvasPositions map[string]CanvasOffset `json:"canvas_positions,omitempty"`
	Answer          string                  `json:"answer,omitempty"`
	Scientific      *ScientificDiagram      `json:"scientific,omitempty"`
	Order           int                     `json:"order,omitempty"`
	Graph           *FunctionGraph          `json:"graph,omitempty"`
	ID              string                  `json:"id,omitempty"`
	Picture         *TeachingPicture        `json:"picture,omitempty"`
	Emphasis        []string                `json:"emphasis,omitempty"`
	Focus           string                  `json:"focus,omitempty"`
	Table           *TeachingTable          `json:"table,omitempty"`
	Figure          *TeachingFigure         `json:"figure,omitempty"`
	Scene           json.RawMessage         `json:"scene,omitempty"`
	Layout          string                  `json:"layout,omitempty"`
	Section         string                  `json:"section,omitempty"`
	Title           string                  `json:"title"`
	Bullets         []string                `json:"bullets"`
	Notes           string                  `json:"notes"`
	Formula         string                  `json:"formula,omitempty"`
	Diagram         string                  `json:"diagram,omitempty"`
	Chart           any                     `json:"chart,omitempty"`
}
type Deck struct {
	EditableFormulas bool                        `json:"editable_formulas,omitempty"`
	Plan             *TeachingPlan               `json:"plan,omitempty"`
	PageChecks       map[string]PageQualityCheck `json:"page_checks,omitempty"`
	Generation       *CoursewareCheckpoint       `json:"generation,omitempty"`
	AnimationQuality *AnimationQuality           `json:"animation_quality,omitempty"`
	Title            string                      `json:"title"`
	Theme            string                      `json:"theme"`
	Lesson           string                      `json:"lesson"`
	Slides           []Slide                     `json:"slides"`
	NeedsReview      bool                        `json:"needs_review"`
	Notice           string                      `json:"notice"`
	Evaluation       any                         `json:"evaluation,omitempty"`
	Script           string                      `json:"script,omitempty"`
}
type Source struct {
	Origin        string         `json:"origin,omitempty"`
	EvidenceGroup string         `json:"evidence_group,omitempty"`
	Reliability   string         `json:"reliability,omitempty"`
	Filter        *PassageFilter `json:"filter,omitempty"`
	Hash          string         `json:"hash"`
	RetrievedAt   string         `json:"retrieved_at"`
	ID            string         `json:"id"`
	DocumentID    string         `json:"document_id"`
	Title         string         `json:"title"`
	Content       string         `json:"content"`
	Location      string         `json:"location"`
	URL           string         `json:"url,omitempty"`
	Kind          string         `json:"kind"`
	Score         float64        `json:"score"`
}

func (s *Server) createTask(w http.ResponseWriter, r *http.Request, a Actor) error {
	var in TaskInput
	if e := decode(r, &in); e != nil {
		return e
	}
	in.Topic = strings.TrimSpace(in.Topic)
	// All newly created courseware uses the same staged quality pipeline.
	// Historical deliveries remain readable and downloadable.
	if in.Type == "courseware" {
		in.PreviewFirst = true
		if in.Detail == "" {
			in.Detail = "auto"
		}
	}

	if len([]rune(in.Requirements)) > 8000 {
		return fail(400, "需求摘要不能超过8000字")
	}
	if in.Type != "courseware" && in.Type != "animation" {
		return fail(400, "仅支持课件和教学动画")
	}
	perm := "courseware.generate"
	cost := int64(20)
	if in.Type == "animation" {
		perm = "animation.render"
	}
	if !allowed(a, perm) {
		return fail(403, "没有生成权限")
	}
	if in.Type == "animation" {
		if in.AnimationEngine == "" || in.AnimationEngine == "html" {
			in.AnimationEngine = "html"
			return s.createHTMLAnimation(w, r, a, in)
		}
		return animationUnavailable()
	}
	if in.Topic == "" || len([]rune(in.Topic)) > 100 || len(in.IdempotencyKey) < 8 || len(in.IdempotencyKey) > 128 {
		return fail(400, "请填写课题并提供有效幂等键")
	}
	if !in.UseKB && !in.UseWeb {
		return fail(400, "至少选择一种资料来源")
	}
	if err := s.resolveTeachingMaterials(r.Context(), a, &in); err != nil {
		return err
	}
	maxPages := 20
	if in.Type == "courseware" && in.Detail == "" && in.Pages == 0 {
		in.Detail = "auto"
	}
	if len([]rune(in.Subject)) > 40 || len([]rune(in.Grade)) > 40 {
		return fail(400, "学科与学习阶段各不超过40字")
	}
	if in.Type == "courseware" && in.Detail != "" {
		if in.Detail != "auto" && in.Detail != "concise" && in.Detail != "default" && in.Detail != "detailed" {
			return fail(400, "请选择按需安排、精简、适中或详细")
		}
		in.Pages = map[string]int{"concise": 10, "default": 15, "detailed": 22}[in.Detail]
		maxPages = 100
		if in.LessonMinutes != 0 && (in.LessonMinutes < 10 || in.LessonMinutes > 180) {
			return fail(400, "课时请填写10至180分钟")
		}
		if len([]rune(in.StudentLevel)) > 400 {
			return fail(400, "学生基础描述过长")
		}
		if in.ReferenceMode == "" {
			in.ReferenceMode = "content"
		}
		if in.ReferenceMode != "content" && in.ReferenceMode != "style" && in.ReferenceMode != "sequence" {
			return fail(400, "参考方式无效")
		}
	}
	if in.Detail != "auto" && (in.Pages < 8 || in.Pages > maxPages) {
		in.Pages = 8
	}
	if !validTheme(in.Theme) {
		in.Theme = "forest"
	}
	if s.cfg.Mode == "live" && !s.agentConfigured(in.Type) {
		return fail(503, "当前制作智能体模型未配置，请设置对应COURSEWARE_LLM或ANIMATION_LLM配置，或默认LLM配置")
	}
	if in.Type == "courseware" && in.Detail != "" && s.cfg.Mode == "live" && !s.coursewareQualityConfigured() {
		return fail(503, "内容与画面审核尚未准备好，请联系管理员完成配置。需求保留，本次未创建作品或预留积分。")
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
		return fail(400, "只有动画可复用课件资料")
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
	if e = s.limitInterrupted(r.Context(), tx, a.MemberID); e != nil {
		return e
	}
	v, e := queryMaps(r.Context(), tx, `SELECT id,COALESCE(NULLIF(display_title,''),input->>'topic','') AS display_title,type,status,stage,input,error,cost,mode,revision,created_at,updated_at,restored_at,capacity_hold,(type='animation' AND html_path<>'') AS has_delivery
		FROM tasks t WHERE member_id=$1 AND deleted_at IS NULL
		AND ($2='' OR type=$2) AND ($3='' OR status=$3)
		AND ($4='' OR strpos(lower(concat_ws(' ',display_title,input->>'topic',input->>'subject',input->>'grade',result->>'title')),lower($4))>0)
		AND ($7<>'works' OR status NOT IN ('failed','cancelled') OR restored_at IS NOT NULL OR capacity_hold OR (type='animation' AND html_path<>''))
		AND ($7<>'interrupted' OR (status IN ('failed','cancelled') AND restored_at IS NULL AND NOT capacity_hold AND NOT (type='animation' AND html_path<>'')))
		AND ($7 NOT IN ('works','interrupted') OR NOT (t.type='courseware' AND t.status IN ('failed','cancelled') AND EXISTS (
			SELECT 1 FROM tasks child WHERE child.tenant_id=t.tenant_id AND child.member_id=t.member_id
			AND child.type='courseware' AND child.idempotency_key='retry-'||t.id
		)))
		ORDER BY created_at DESC,id DESC LIMIT $5 OFFSET $6`, a.MemberID, q.Get("type"), q.Get("status"), strings.TrimSpace(q.Get("q")), limit, offset, view)
	if e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
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
	v, e := queryMaps(r.Context(), tx, "SELECT id,COALESCE(NULLIF(display_title,''),input->>'topic','') AS display_title,type,status,stage,input,result,sources,error,mode,cost,charge_state,revision,created_at,updated_at,restored_at,courseware_provider_calls,automatic_failures FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL", r.PathValue("id"), a.MemberID)
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
	if v[0]["status"] == "failed" && v[0]["type"] != "animation" {
		v[0]["error"] = "制作暂未完成，已有页面已保存，预留已释放。可以稍后继续制作，完成页不会重新生成。"
		for _, row := range ev {
			if row["stage"] == "failed" {
				row["message"] = v[0]["error"]
			}
		}
	}
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
			pages, err := s.coursewarePreviewPages(r.Context(), tx, r.PathValue("id"), d, pptxPath)
			if err != nil {
				return err
			}
			ids := []string{}
			for _, sl := range d.Slides {
				if pages[sl.ID] != "" {
					ids = append(ids, sl.ID)
				}
			}
			v[0]["preview_images_available"] = len(ids) > 0
			v[0]["preview_slide_ids"] = ids
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
	if e = coursewareChatIdle(r.Context(), tx, id, ""); e != nil {
		return e
	}
	// Revisions contain copies of the original deck; hide these too, retaining all accounting rows.
	deleted, e := tx.Exec(r.Context(), "UPDATE tasks SET deleted_at=now(),cleanup_pending=true,updated_at=now() WHERE member_id=$1 AND deleted_at IS NULL AND (id=$2 OR (type='revision' AND input->>'parent_id'=$2))", a.MemberID, id)
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
	cleanupCtx, stop := context.WithTimeout(context.Background(), 8*time.Second)
	defer stop()
	cleaned, cleanupErr := s.cleanupDeletedTask(cleanupCtx, a.TenantID, id)
	respond(w, 200, map[string]bool{"ok": true, "cleanup_pending": cleanupErr != nil || !cleaned || deleted.RowsAffected() > 1})
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
	if d.Plan != nil {
		if err := validateTeachingPlan(*d.Plan); err != nil {
			return err
		}
		return validateDeckMinimum(d, 3)
	}
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
		if err := validateCanvasPositions(sl); err != nil {
			return err
		}
		for _, obj := range sl.CanvasObjects {
			imageSize += len(obj.DataURL)
		}
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
	if kind == "animation" {
		return animationUnavailable()
	}
	if kind == "courseware" {
		in.Result.Notice = old.Notice
		preserveCoursewareChecks(&in.Result, old)
		in.Result = arrangeCourseware(in.Result)
	}
	if e := validateDeck(in.Result); e != nil {
		return e
	}
	if kind == "courseware" && original.PreviewFirst {
		if err := coursewareChatIdle(r.Context(), tx, r.PathValue("id"), ""); err != nil {
			return err
		}
		if err := s.allSlidesApproved(r.Context(), tx, r.PathValue("id"), in.Result); err != nil {
			return err
		}
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
	in.Result.Evaluation = nil
	in.Result.Script = ""
	in.Result.Generation = nil
	in.Result.AnimationQuality = nil
	if _, err := s.renderCourseware(r.Context(), &in.Result); err != nil {
		return err
	}
	if kind == "courseware" && original.PreviewFirst {
		if err := s.allSlidesApproved(r.Context(), tx, r.PathValue("id"), in.Result); err != nil {
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
	if handled, err := s.cancelHTMLIfApplicable(w, r, a); handled || err != nil {
		return err
	}
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
	if r.PathValue("format") == "mp4" {
		if handled, e := s.animationMP4File(w, r, a); handled {
			return e
		}
	}
	if r.PathValue("format") == "html" {
		return s.animationFile(w, r, a)
	}
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
	return s.serveStoredFile(w, r, path)
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
func (s *Server) workOne(ctx context.Context) error {
	tenants, e := s.workerTenants(ctx)
	if e != nil {
		return e
	}
	for _, tid := range tenants {
		if worked, err := s.workHTMLAnimation(ctx, tid); worked || err != nil {
			return err
		}
		tx, e := s.tx(ctx, tid)
		if e != nil {
			return e
		}
		var id, status, kind, mode string
		var member int64
		var in TaskInput
		var result Deck
		var cycle, attempt int
		e = tx.QueryRow(ctx, "SELECT id,status,type,member_id,input,result,mode,charge_cycle,attempt FROM tasks WHERE deleted_at IS NULL AND charge_state='reserved' AND status IN ('pending','confirmed','expand_pending') AND (next_attempt_at IS NULL OR next_attempt_at<=now()) ORDER BY COALESCE(next_attempt_at,created_at) FOR UPDATE SKIP LOCKED LIMIT 1").Scan(&id, &status, &kind, &member, &in, &result, &mode, &cycle, &attempt)
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
		if kind == "courseware" && in.PreviewFirst && stage == "researching" {
			workLimit = 15 * time.Minute
		}
		if kind == "courseware" && in.PreviewFirst && stage == "expanding" {
			workLimit = coursewareExpansionLimit(in.Pages)
		}
		_, e = tx.Exec(ctx, "UPDATE tasks SET status=$1,stage=$1,next_attempt_at=NULL,lease_until=now()+interval '3 minutes',attempt=attempt+1,updated_at=now() WHERE id=$2", stage, id)
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
		defer s.queueDeletedCleanup(tid, id, true, cycle)
		workCtx, cancel := context.WithTimeout(ctx, workLimit)
		workCtx = withCoursewareArtifacts(workCtx)
		workCtx = context.WithValue(workCtx, generationCycleKey{}, cycle)
		workCtx = context.WithValue(workCtx, generationAttemptKey{}, attempt+1)
		a := Actor{MemberID: member, TenantID: tid}
		if mode != s.cfg.Mode {
			e = fmt.Errorf("运行模式已切换，请在当前模式重新提交任务")
		} else {
			e = s.runLeasedTask(workCtx, cancel, a, id, attempt+1, func() error { return s.processTask(workCtx, a, id, kind, stage, in, result) })
		}
		if errors.Is(workCtx.Err(), context.DeadlineExceeded) {
			e = fmt.Errorf("制作达到本阶段等待时限，已保存进度：%w", context.DeadlineExceeded)
		}
		interrupted := errors.Is(workCtx.Err(), context.Canceled)
		cancel()
		if ctx.Err() != nil || interrupted {
			return s.requeueInterrupted(tid, id, attempt+1)
		}
		if e != nil {
			finishCtx, c := context.WithTimeout(context.Background(), 10*time.Second)
			finishCtx = context.WithValue(finishCtx, generationCycleKey{}, cycle)
			finishCtx = context.WithValue(finishCtx, generationAttemptKey{}, attempt+1)
			resumed, recoveryErr := false, error(nil)
			if kind == "courseware" && mode == s.cfg.Mode {
				resumed, recoveryErr = s.resumeCoursewareAutomatically(finishCtx, a, id, stage, in, e)
			}
			if recoveryErr == nil && !resumed {
				recoveryErr = s.finish(finishCtx, tid, id, "failed", e.Error(), "", nil)
			}
			c()
			if recoveryErr != nil {
				return recoveryErr
			}
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
		return animationUnavailable()
	}
	return s.runCoursewareAgent(ctx, a, id, stage, in, d)
}

func (s *Server) renderPPTX(ctx context.Context, d Deck) ([]byte, error) {
	for _, sl := range d.Slides {
		if len(sl.CanvasPositions) > 0 || len(sl.CanvasObjects) > 0 || len(sl.CanvasOrder) > 0 {
			req, err := http.NewRequestWithContext(ctx, "GET", s.cfg.RenderURL+"/health", nil)
			if err != nil {
				return nil, err
			}
			res, err := s.http.Do(req)
			if err != nil {
				return nil, fail(503, "对象排版服务暂不可用，当前修改保留")
			}
			var health map[string]any
			err = json.NewDecoder(res.Body).Decode(&health)
			res.Body.Close()
			if err != nil || health["courseware_canvas_version"] != "canvas-v3" {
				return nil, fail(503, "对象排版服务尚未更新，当前修改保留")
			}
			break
		}
	}
	needsScience := false
	for _, sl := range d.Slides {
		needsScience = needsScience || sl.Scientific != nil
		if sl.Graph != nil {
			for _, series := range sl.Graph.Series {
				needsScience = needsScience || series.Kind != "" && series.Kind != "linear"
			}
		}
	}
	if needsScience {
		c, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(c, "GET", s.cfg.RenderURL+"/health", nil)
		if err != nil {
			return nil, err
		}
		res, err := s.http.Do(req)
		if err != nil {
			return nil, fail(503, "专业图示排版服务暂不可用")
		}
		var health map[string]any
		err = json.NewDecoder(res.Body).Decode(&health)
		res.Body.Close()
		if err != nil || health["scientific_diagrams_version"] != "science-v1" {
			return nil, fail(503, "专业图示排版服务尚未更新，数据已保留")
		}
	}
	needsMedia, needsGraph, needsPlot := d.Theme == "classroom", false, false
	for _, sl := range d.Slides {
		needsGraph = needsGraph || sl.Graph != nil
		needsPlot = needsPlot || usesMatplotlib(sl)
		needsMedia = needsMedia || sl.Table != nil || sl.Figure != nil || sl.Picture != nil || sl.Focus != "" || len(sl.Emphasis) > 0
	}
	if needsMedia || needsGraph || needsPlot {
		if err := s.requireTeachingMediaRenderer(ctx, needsGraph, needsPlot); err != nil {
			return nil, err
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
