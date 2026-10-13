package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Server) editTask(w http.ResponseWriter, r *http.Request, a Actor) error {
	defer s.queueDeletedCleanup(a.TenantID, r.PathValue("id"), false)
	var in struct {
		Result   Deck `json:"result"`
		Revision int  `json:"revision"`
	}
	if e := decode(r, &in); e != nil {
		return e
	}
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	var revision int
	var old Deck
	var status, kind string
	e = tx.QueryRow(r.Context(), "SELECT revision,result,status,type FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL", r.PathValue("id"), a.MemberID).Scan(&revision, &old, &status, &kind)
	var sourceRevision int
	if e == nil && r.PathValue("version") != "" {
		sourceRevision, e = parseRevision(r)
		if e == nil && kind != "courseware" {
			e = fail(409, "仅已完成课件支持版本恢复")
		}
		if e == nil {
			e = tx.QueryRow(r.Context(), "SELECT result FROM artifact_revisions WHERE task_id=$1 AND revision=$2 AND pptx_path<>''", r.PathValue("id"), sourceRevision).Scan(&in.Result)
		}
	}
	tx.Rollback(r.Context())
	if e != nil {
		return e
	}
	if kind == "animation" || !(status == "succeeded" || status == "degraded") {
		return fail(409, "只能重新导出已完成的课件")
	}
	if revision != in.Revision {
		return fail(409, "内容已更新，请刷新")
	}
	in.Result.Notice = old.Notice
	preserveCoursewareChecks(&in.Result, old)
	in.Result = arrangeCourseware(in.Result)
	if e := validateDeck(in.Result); e != nil {
		return e
	}
	in.Result.NeedsReview = old.NeedsReview || in.Result.NeedsReview
	in.Result.Evaluation = nil
	in.Result.Script = ""
	in.Result.Generation = nil
	path, e := s.savePPTX(r.Context(), a.TenantID, r.PathValue("id")+"-"+newID(), &in.Result)
	if e != nil {
		return e
	}
	committed := false
	defer func() {
		if !committed {
			os.Remove(path)
		}
	}()
	tx, e = s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	// Lock the parent before checking active rewrites: rewrite creation takes a shared parent lock.
	if e = tx.QueryRow(r.Context(), "SELECT revision FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL FOR UPDATE", r.PathValue("id"), a.MemberID).Scan(&revision); e != nil {
		if e == pgx.ErrNoRows {
			return fail(409, "课件已删除，请刷新")
		}
		return e
	}
	if revision != in.Revision {
		return fail(409, "课件已被其他操作更新，请刷新后再试")
	}
	var rewriting bool
	if e = tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM tasks WHERE member_id=$1 AND input->>'parent_id'=$2 AND type='revision' AND charge_state='reserved' AND deleted_at IS NULL)", a.MemberID, r.PathValue("id")).Scan(&rewriting); e != nil {
		return e
	}
	if rewriting {
		return fail(409, "有单页AI修改正在进行，请等待完成或取消后再保存/恢复")
	}
	if err := coursewareChatIdle(r.Context(), tx, r.PathValue("id"), ""); err != nil {
		return err
	}
	if e = archiveRevision(r.Context(), tx, a.TenantID, r.PathValue("id")); e != nil {
		return e
	}
	if e = invalidateChangedChecks(r.Context(), tx, r.PathValue("id"), in.Result, old); e != nil {
		return e
	}
	res, e := tx.Exec(r.Context(), "UPDATE tasks SET result=$1,pptx_path=$2,revision=revision+1,updated_at=now() WHERE id=$3 AND member_id=$4 AND revision=$5 AND deleted_at IS NULL", asJSON(in.Result), path, r.PathValue("id"), a.MemberID, in.Revision)
	if e != nil {
		return e
	}
	if res.RowsAffected() != 1 {
		os.Remove(path)
		return fail(409, "课件已被其他操作更新，请刷新后再试")
	}
	if e = archiveRevision(r.Context(), tx, a.TenantID, r.PathValue("id")); e != nil {
		return e
	}
	stage, message := "edited", "教师手工编辑并重新导出（免费）"
	if sourceRevision > 0 {
		stage, message = "restored", fmt.Sprintf("已恢复版本%d，保存为新版本%d并重新导出（免费）", sourceRevision, revision+1)
	}
	if e = event(r.Context(), tx, a.TenantID, r.PathValue("id"), stage, message); e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	committed = true
	respond(w, 200, map[string]bool{"ok": true})
	return nil
}

func parseRevision(r *http.Request) (int, error) {
	n, err := strconv.Atoi(r.PathValue("version"))
	if err != nil || n < 1 {
		return 0, fail(400, "版本号必须是正整数")
	}
	return n, nil
}

func (s *Server) listRevisions(w http.ResponseWriter, r *http.Request, a Actor) error {
	offset := 0
	if r.URL.Query().Has("offset") {
		var err error
		offset, err = strconv.Atoi(r.URL.Query().Get("offset"))
		if err != nil || offset < 0 {
			return fail(400, "offset必须是非负整数")
		}
	}
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var kind string
	if e = tx.QueryRow(r.Context(), "SELECT type FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL", r.PathValue("id"), a.MemberID).Scan(&kind); e != nil {
		return e
	}
	if kind != "courseware" {
		return fail(409, "仅课件支持版本历史")
	}
	rows, e := queryMaps(r.Context(), tx, `SELECT ar.revision,ar.created_at,ar.result->>'title' AS title,ar.result->>'theme' AS theme,jsonb_array_length(ar.result->'slides') AS pages
		FROM artifact_revisions ar JOIN tasks t ON t.tenant_id=ar.tenant_id AND t.id=ar.task_id
		WHERE t.id=$1 AND t.member_id=$2 AND t.deleted_at IS NULL AND ar.pptx_path<>'' ORDER BY ar.revision DESC LIMIT 51 OFFSET $3`, r.PathValue("id"), a.MemberID, offset)
	if e != nil {
		return e
	}
	respond(w, 200, rows)
	return nil
}

func (s *Server) getRevision(w http.ResponseWriter, r *http.Request, a Actor) error {
	version, e := parseRevision(r)
	if e != nil {
		return e
	}
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	rows, e := queryMaps(r.Context(), tx, `SELECT ar.revision,ar.created_at,ar.result FROM artifact_revisions ar JOIN tasks t ON t.tenant_id=ar.tenant_id AND t.id=ar.task_id
		WHERE t.id=$1 AND t.member_id=$2 AND t.type='courseware' AND t.deleted_at IS NULL AND ar.revision=$3 AND ar.pptx_path<>''`, r.PathValue("id"), a.MemberID, version)
	if e != nil {
		return e
	}
	if len(rows) == 0 {
		return pgx.ErrNoRows
	}
	respond(w, 200, rows[0])
	return nil
}
func archiveRevision(ctx context.Context, tx pgx.Tx, tid int64, id string) error {
	_, e := tx.Exec(ctx, "INSERT INTO artifact_revisions(tenant_id,task_id,revision,result,pptx_path,mp4_path) SELECT tenant_id,id,revision,result,pptx_path,mp4_path FROM tasks WHERE id=$1 AND deleted_at IS NULL ON CONFLICT DO NOTHING", id)
	if e != nil {
		return e
	}
	pruned, e := tx.Exec(ctx, `DELETE FROM artifact_revisions WHERE task_id=$1
 AND revision<>(SELECT min(revision) FROM artifact_revisions WHERE task_id=$1)
 AND revision NOT IN (SELECT revision FROM artifact_revisions WHERE task_id=$1 ORDER BY revision DESC LIMIT 20)
 AND revision<>(SELECT revision FROM tasks WHERE id=$1)`, id)
	if e == nil && pruned.RowsAffected() > 0 {
		_, e = tx.Exec(ctx, "UPDATE tasks SET cleanup_pending=true WHERE id=$1", id)
	}
	return e
}
func (s *Server) savePPTX(ctx context.Context, tid int64, id string, d *Deck) (string, error) {
	if strings.Contains(d.Lesson, coursewareLessonPages) {
		d.Lesson = checkpointLesson(*d)
	}
	b, e := s.renderCourseware(ctx, d)
	if e != nil {
		return "", e
	}
	path := storagePath(s, tid, id, ".pptx")
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return "", e
	}
	if err := s.writeStoredFile(ctx, path, b); err != nil {
		return "", err
	}
	if len(d.Slides) > 0 && d.Slides[0].ID != "" {
		assignSlideIDs(d)
		s.snapshotCourseware(ctx, path)
		if _, err := os.Stat(path + ".slides"); err == nil {
			if err = s.syncDirectory(ctx, path+".slides"); err != nil {
				return "", err
			}
		}
	}
	return path, nil
}

func (s *Server) rewriteTask(w http.ResponseWriter, r *http.Request, a Actor) error {
	var in struct {
		Conversation bool   `json:"conversation"`
		Instruction  string `json:"instruction"`
		Slide        int    `json:"slide"`
		Revision     int    `json:"revision"`
		Key          string `json:"idempotency_key"`
	}
	if e := decode(r, &in); e != nil {
		return e
	}
	if len(in.Key) < 8 || len(in.Key) > 128 || len([]rune(in.Instruction)) > 600 || strings.TrimSpace(in.Instruction) == "" {
		return fail(400, "请填写有效修改要求和幂等键")
	}
	if s.cfg.Mode == "live" && !s.agentConfigured("courseware") {
		return fail(503, "真实模型未配置")
	}
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var balance, reserved int64
	if e = tx.QueryRow(r.Context(), "SELECT balance,reserved FROM members WHERE id=$1 FOR UPDATE", a.MemberID).Scan(&balance, &reserved); e != nil {
		return e
	}
	var d Deck
	var parent TaskInput
	var src []Source
	var revision int
	var kind, status, mode string
	e = tx.QueryRow(r.Context(), "SELECT result,sources,revision,type,status,mode,input FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL FOR SHARE", r.PathValue("id"), a.MemberID).Scan(&d, &src, &revision, &kind, &status, &mode, &parent)
	if e != nil {
		return e
	}
	var existing string
	var deleted bool
	e = tx.QueryRow(r.Context(), "SELECT id,deleted_at IS NOT NULL FROM tasks WHERE member_id=$1 AND idempotency_key=$2", a.MemberID, in.Key).Scan(&existing, &deleted)
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
	if kind != "courseware" || status != "succeeded" || revision != in.Revision || in.Slide < 0 || in.Slide >= len(d.Slides) {
		return fail(409, "当前课件状态、修订号或页码不允许修改")
	}
	if mode != s.cfg.Mode {
		return fail(409, "切换运行模式后请新建课件")
	}
	if err := coursewareChatIdle(r.Context(), tx, r.PathValue("id"), ""); err != nil {
		return err
	}
	if balance-reserved < 5 {
		return fail(402, "积分不足，禁止AI修改")
	}
	id := newID()
	input := TaskInput{Topic: d.Title, Grade: parent.Grade, Subject: parent.Subject, Textbook: parent.Textbook, Chapter: parent.Chapter, Curriculum: parent.Curriculum, Reference: parent.Reference, ReferenceMode: parent.ReferenceMode, ParentID: r.PathValue("id"), ParentRevision: revision, SlideIndex: in.Slide, Instruction: in.Instruction, IdempotencyKey: in.Key}
	_, e = tx.Exec(r.Context(), "UPDATE members SET reserved=reserved+5 WHERE id=$1", a.MemberID)
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), "INSERT INTO tasks(id,tenant_id,member_id,type,input,result,sources,mode,cost,idempotency_key) VALUES($1,$2,$3,'revision',$4,$5,$6,$7,5,$8)", id, a.TenantID, a.MemberID, asJSON(input), asJSON(d), asJSON(src), s.cfg.Mode, in.Key)
	if e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), "INSERT INTO ledger(tenant_id,member_id,kind,delta,balance_after,reference) VALUES($1,$2,'reserve',0,$3,$4)", a.TenantID, a.MemberID, balance, id)
	if e != nil {
		return e
	}
	if e = event(r.Context(), tx, a.TenantID, id, "queued", "单页AI修改已预留5积分；失败保留原课件"); e != nil {
		return e
	}
	if in.Conversation {
		_, e = tx.Exec(r.Context(), "INSERT INTO courseware_messages(id,tenant_id,task_id,request_key,revision,slide_id,mode,content,reply,child_id) VALUES($1,$2,$3,$4,$5,$6,'revise',$7,$8,$9)", newID(), a.TenantID, r.PathValue("id"), in.Key, in.Revision, d.Slides[in.Slide].ID, in.Instruction, "单页AI修改任务已提交，成功后更新原课件；预留5积分。", id)
		if e != nil {
			return e
		}
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	respond(w, 202, map[string]string{"id": id})
	return nil
}
func (s *Server) processRevision(ctx context.Context, a Actor, id string, in TaskInput, d Deck) error {
	original := d
	original.Slides = append([]Slide{}, d.Slides...)
	if in.SlideIndex < 0 || in.SlideIndex >= len(d.Slides) {
		return fmt.Errorf("修订页码无效")
	}
	check, e := s.tx(ctx, a.TenantID)
	if e != nil {
		return e
	}
	var visible bool
	e = check.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tasks child JOIN tasks parent ON parent.id=child.input->>'parent_id' AND parent.tenant_id=child.tenant_id AND parent.member_id=child.member_id
		WHERE child.id=$1 AND child.member_id=$2 AND child.deleted_at IS NULL AND parent.deleted_at IS NULL AND child.charge_state='reserved')`, id, a.MemberID).Scan(&visible)
	check.Rollback(ctx)
	if e != nil {
		return e
	}
	if !visible {
		return fmt.Errorf("改写任务或原课件已不可用")
	}
	feedbackTx, err := s.tx(ctx, a.TenantID)
	if err != nil {
		return err
	}
	feedback, err := feedbackInstructions(ctx, feedbackTx, in.ParentID, d, d.Slides[in.SlideIndex].ID)
	feedbackTx.Rollback(ctx)
	if err != nil {
		return err
	}
	in.Instruction += feedback
	if s.cfg.Mode == "sample" {
		d.Slides[in.SlideIndex].Notes += "\n示例修改要求：" + in.Instruction
		d.Slides[in.SlideIndex].Bullets = append(d.Slides[in.SlideIndex].Bullets[:min(5, len(d.Slides[in.SlideIndex].Bullets))], "示例改写：先独立思考，再与同学讨论解题依据。")
	} else {
		originalPicture := d.Slides[in.SlideIndex].Picture
		raw, e := s.agentChat(ctx, a, id, "courseware", "slide.rewrite", "根据教师要求修改课件的一页。返回JSON {title,layout,section,bullets,notes,formula,diagram,chart,table,figure,graph,scientific,focus,emphasis}；layout仅cover/concept/steps/comparison/example/exercise/summary，section最多80字。最多6条要点，每条最多80字。保留原有图表、表格、图解、公式及相关数据，除非教师明确要求修改。保留学科课题和视觉主题的区别；例题显示关键推理，完整讲解和练习答案留在notes。不要新增引用。以下全篇页数要求不适用于本次单页改写。图片由系统原样保留，不要输出picture，不能声称已看过图片；存在图片时不要新增chart/diagram，也不要同时添加table和figure。"+coursewareMediaContract+scientificContract+"\n"+coursewareTeachingSkill+coursewarePageContext(d, in.SlideIndex+1)+"\n修改要求："+in.Instruction+gradeGuidance(in))
		if e != nil {
			return e
		}
		var parsed Deck
		if e = decodeCourseware("{\"slides\":["+raw+"]}", &parsed); e != nil {
			return e
		}
		slide := parsed.Slides[0]
		slide.Notes += "\nAI修改内容须教师核验。"
		slide.Picture = originalPicture
		slide.ID = d.Slides[in.SlideIndex].ID
		d.Slides[in.SlideIndex] = slide
	}
	tx, e := s.tx(ctx, a.TenantID)
	if e != nil {
		return e
	}
	var snapshots []Source
	e = tx.QueryRow(ctx, "SELECT sources FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL AND charge_state='reserved'", id, a.MemberID).Scan(&snapshots)
	tx.Rollback(ctx)
	if e != nil {
		return e
	}
	known := map[string]bool{}
	for _, source := range snapshots {
		known["["+source.ID+"]"] = true
	}
	for _, ref := range citation.FindAllString(string(asJSON(d.Slides[in.SlideIndex])), -1) {
		if !known[ref] {
			return fmt.Errorf("单页修改包含不存在的引用，已保留旧版本")
		}
	}
	d.Evaluation = nil
	d = arrangeCourseware(d)
	if e := validateDeck(d); e != nil {
		return e
	}
	path, e := s.savePPTX(ctx, a.TenantID, id, &d)
	if e != nil {
		return e
	}
	if original.Slides[in.SlideIndex].ID != "" {
		if e := singlePageChange(original, d, in.SlideIndex); e != nil {
			return e
		}
	}
	return s.finish(ctx, a.TenantID, id, "succeeded", "单页修改已保存，新PPTX已生成，结算5积分", path, d)
}

func singlePageChange(before, after Deck, index int) error {
	if len(before.Slides) != len(after.Slides) || before.Slides[index].ID != after.Slides[index].ID {
		return fail(422, "单页修订产生续页或改变页面身份，请精简显示内容、保留讲解备注；原课件保留")
	}
	for i := range before.Slides {
		if i != index && !reflect.DeepEqual(before.Slides[i], after.Slides[i]) {
			return fail(422, "单页修订排版影响了其他页面，修改未覆盖原课件")
		}
	}
	return nil
}
