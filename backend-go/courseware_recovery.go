package main

import (
	"fmt"
	"github.com/jackc/pgx/v5"
	"net/http"
	"os"
)

// Split incompatible visual objects without discarding content or asking the
// model to regenerate subject matter. Input still targets 8–20 teaching pages;
// continuation pages may extend the editable artifact up to 100 pages.
func arrangeCourseware(d Deck) Deck {
	d = arrangeCoursewareTables(d)
	d = arrangeCoursewareFigures(d)
	slides := make([]Slide, 0, len(d.Slides))
	for _, sl := range d.Slides {
		conflict := (sl.Table != nil || sl.Figure != nil) && (sl.Chart != nil || sl.Diagram != "") || sl.Picture != nil && (sl.Chart != nil || sl.Diagram != "" || sl.Table != nil && sl.Figure != nil) || sl.Chart != nil && sl.Diagram != ""
		if !conflict {
			slides = append(slides, sl)
			continue
		}
		count := 0
		for _, kind := range []string{"表格", "图解", "图表", "示意图", "配图"} {
			if kind == "表格" && sl.Table == nil || kind == "图解" && sl.Figure == nil || kind == "图表" && sl.Chart == nil || kind == "示意图" && sl.Diagram == "" || kind == "配图" && sl.Picture == nil {
				continue
			}
			part := sl
			part.Table = nil
			part.Figure = nil
			part.Chart = nil
			part.Diagram = ""
			part.Picture = nil
			switch kind {
			case "表格":
				part.Table = sl.Table
			case "图解":
				part.Figure = sl.Figure
			case "图表":
				part.Chart = sl.Chart
			case "示意图":
				part.Diagram = sl.Diagram
			case "配图":
				part.Picture = sl.Picture
			}
			if count > 0 {
				title := []rune(sl.Title)
				if len(title) > 54 {
					title = title[:54]
				}
				part.Title = fmt.Sprintf("%s · %s补充%d", string(title), kind, len(slides)+1)
				part.Bullets = []string{"结合本组页面的题设，观察“" + part.Title + "”并说明依据。"}
				part.Focus = ""
				part.Formula = ""
			}
			slides = append(slides, part)
			count++
		}
	}
	if len(slides) > len(d.Slides) {
		d.Notice += fmt.Sprintf(" 已自动将冲突图形分开展示，增加%d页；教学内容与讲解备注保留。", len(slides)-len(d.Slides))
	}
	d.Slides = slides
	return arrangeCoursewareText(d)
}

func reusableCourseware(d Deck, sources []Source) bool {
	if validateDeck(d) != nil {
		return false
	}
	known := map[string]bool{}
	for _, source := range sources {
		known["["+source.ID+"]"] = true
	}
	for _, ref := range citation.FindAllString(d.Lesson+string(asJSON(d.Slides)), -1) {
		if !known[ref] {
			return false
		}
	}
	return true
}

// Each interrupted attempt has at most one follow-up, preserving released
// ledger entries. Concurrent clicks return that same child and reserve once.
func (s *Server) retryCourseware(w http.ResponseWriter, r *http.Request, a Actor) error {
	tx, err := s.tx(r.Context(), a.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	var kind, status, mode, charge string
	var input TaskInput
	var draft Deck
	var sources []Source
	err = tx.QueryRow(r.Context(), "SELECT type,status,mode,charge_state,input,result,sources FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL FOR UPDATE", r.PathValue("id"), a.MemberID).Scan(&kind, &status, &mode, &charge, &input, &draft, &sources)
	if err != nil {
		return err
	}
	if kind != "courseware" || status != "failed" || charge != "released" {
		return fail(409, "此入口仅用于继续已中断且释放额度的课件")
	}
	if mode != s.cfg.Mode {
		return fail(409, "运行模式已切换，请重新创建课件")
	}
	key := "retry-" + r.PathValue("id")
	var existing string
	var deleted bool
	err = tx.QueryRow(r.Context(), "SELECT id,deleted_at IS NOT NULL FROM tasks WHERE member_id=$1 AND idempotency_key=$2", a.MemberID, key).Scan(&existing, &deleted)
	if err == nil {
		if deleted {
			return fail(409, "续作已删除，请新建课件")
		}
		respond(w, 202, map[string]string{"id": existing})
		return nil
	}
	if err != pgx.ErrNoRows {
		return err
	}
	var balance, reserved int64
	var plan string
	if err = tx.QueryRow(r.Context(), "SELECT balance,reserved,plan FROM members WHERE id=$1 FOR UPDATE", a.MemberID).Scan(&balance, &reserved, &plan); err != nil {
		return err
	}
	if plan == "" {
		return fail(403, "请先开通基础套餐")
	}
	if balance-reserved < 20 {
		return fail(402, "成员可用积分不足，禁止生成")
	}
	stage := "pending"
	previewPath := ""
	revision := 0
	result := any(map[string]any{})
	draft = arrangeCourseware(draft)
	if input.PreviewFirst && validateDeckMinimum(draft, 3) == nil && coursewareSourceError(draft, sources) == nil {
		// A released task keeps its private history. Reuse only an artifact matching
		// the current visible pages; never serve a stale PPTX as the new preview.
		err = tx.QueryRow(r.Context(), "SELECT pptx_path FROM artifact_revisions WHERE task_id=$1 AND result->'slides'=$2::jsonb AND result->>'theme'=$3 AND result->>'title'=$4 AND pptx_path<>'' ORDER BY revision DESC LIMIT 1", r.PathValue("id"), asJSON(draft.Slides), draft.Theme, draft.Title).Scan(&previewPath)
		if err != nil && err != pgx.ErrNoRows {
			return err
		}
		if _, err := os.Stat(previewPath); err != nil {
			previewPath = ""
		}
		if previewPath != "" && (len(draft.Slides) == 3 || reusableCourseware(draft, sources)) {
			stage = "awaiting_confirmation"
			if len(draft.Slides) == 3 {
				stage = "awaiting_preview"
			}
			revision = 1
			draft.Evaluation = nil
			draft.NeedsReview = true
			draft.Notice += " 已恢复上次真实预览，请重新逐页审核；未自动生成剩余页。"
			result = draft
		}
	} else if !input.PreviewFirst && reusableCourseware(draft, sources) {
		stage = "awaiting_confirmation"
		draft.NeedsReview = true
		draft.Evaluation = nil
		draft.Notice += " 已恢复上次草稿，请确认内容后继续制作。"
		result = draft
	}
	id := newID()
	input.IdempotencyKey = key
	if _, err = tx.Exec(r.Context(), "UPDATE members SET reserved=reserved+20 WHERE id=$1", a.MemberID); err != nil {
		return err
	}
	if stage == "pending" {
		previewPath = ""
	}
	_, err = tx.Exec(r.Context(), "INSERT INTO tasks(id,tenant_id,member_id,type,status,stage,input,result,sources,mode,cost,idempotency_key,pptx_path,revision) VALUES($1,$2,$3,'courseware',$4,$4,$5,$6,$7,$8,20,$9,$10,$11)", id, a.TenantID, a.MemberID, stage, asJSON(input), asJSON(result), asJSON(sources), mode, key, previewPath, revision)
	if err != nil {
		return err
	}
	if previewPath != "" {
		if err = archiveRevision(r.Context(), tx, a.TenantID, id); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(r.Context(), "INSERT INTO ledger(tenant_id,member_id,kind,delta,balance_after,reference) VALUES($1,$2,'reserve',0,$3,$4)", a.TenantID, a.MemberID, balance, id); err != nil {
		return err
	}
	if err = event(r.Context(), tx, a.TenantID, id, stage, "继续制作：已重新预留20积分，上次未结算；有草稿时直接恢复供确认"); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	respond(w, 202, map[string]string{"id": id})
	return nil
}
