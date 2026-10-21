package main

import (
	"fmt"
	"github.com/jackc/pgx/v5"
	"net/http"
	"os"
	"strings"
)

const coursewareResumeNotice = "已恢复逐页检查点，继续只处理问题页和未完成页；未变更页面的审核保留。"

// Split incompatible visual objects without discarding content or asking the
// model to regenerate subject matter. Input still targets 8–20 teaching pages;
// continuation pages may extend the editable artifact up to 100 pages.
func arrangeCourseware(d Deck) Deck {
	if d.Plan != nil {
		return d
	}
	d = arrangeCoursewareTables(d)
	d = arrangeCoursewareFigures(d)
	slides := make([]Slide, 0, len(d.Slides))
	for _, sl := range d.Slides {
		if sl.HTML != "" {
			slides = append(slides, sl)
			continue
		}
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

// Legacy continuation links remain readable; new continuations reuse the work.
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
	current := r.PathValue("id")
	key := ""
	cycle := 0
	for depth := 0; depth < 100; depth++ {
		input, draft, sources = TaskInput{}, Deck{}, nil
		var restored bool
		var leased bool
		err = tx.QueryRow(r.Context(), "SELECT type,status,mode,charge_state,input,result,sources,restored_at IS NOT NULL,charge_cycle,COALESCE(lease_until>now(),false) FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL FOR UPDATE", current, a.MemberID).Scan(&kind, &status, &mode, &charge, &input, &draft, &sources, &restored, &cycle, &leased)
		if err != nil {
			return err
		}
		resumable := (status == "failed" || status == "cancelled" && restored) && charge == "released"
		if kind == "courseware" && charge == "reserved" {
			respond(w, 202, map[string]string{"id": current})
			return nil
		}
		if kind != "courseware" || (depth == 0 && !resumable) {
			return fail(409, "此入口仅用于继续已中断且释放额度的课件")
		}
		if mode != s.cfg.Mode {
			return fail(409, "运行模式已切换，请重新创建课件")
		}
		if leased {
			return fail(409, "上次制作正在结束，草稿已保存，请稍后继续")
		}
		if err = coursewareChatIdle(r.Context(), tx, current, ""); err != nil {
			return err
		}
		if depth > 0 && !resumable {
			respond(w, 202, map[string]string{"id": current})
			return nil
		}
		key = "retry-" + current
		var existing string
		var deleted bool
		err = tx.QueryRow(r.Context(), "SELECT id,deleted_at IS NOT NULL FROM tasks WHERE member_id=$1 AND idempotency_key=$2", a.MemberID, key).Scan(&existing, &deleted)
		if err == pgx.ErrNoRows {
			break
		}
		if err != nil {
			return err
		}
		if deleted {
			return fail(409, "续作已删除，请新建课件")
		}
		current = existing
	}
	if key == "" || key != "retry-"+current {
		return fail(409, "续作次数过多，请从最新作品继续")
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
	result := any(map[string]any{})
	if input.PreviewFirst && (len(draft.Slides) > 0 || draft.Generation != nil) {
		// Content checkpoints do not depend on successful whole-deck rendering.
		// Never arrange/split a saved page here: its identity and approval hash
		// must survive every continuation.
		phase := "preview"
		if draft.Generation != nil && draft.Generation.Phase == "expand" {
			phase = "expand"
		} else if len(draft.Slides) >= 3 {
			prefix := draft
			prefix.Slides = prefix.Slides[:3]
			if s.allSlidesApproved(r.Context(), tx, current, prefix) == nil || len(draft.Slides) > 3 {
				phase = "expand"
			}
		}
		if draft.Generation == nil {
			draft.Generation = &CoursewareCheckpoint{Version: coursewarePageRepairVersion, Phase: phase}
		}
		if phase == "expand" {
			stage = "expand_pending"
		}
		draft.Evaluation = nil
		draft.Generation.PageAttempts = nil // Explicit teacher continuation starts a new bounded repair allowance.
		draft.NeedsReview = true
		draft.Notice = strings.TrimSpace(strings.ReplaceAll(draft.Notice, coursewareResumeNotice, "")) + " " + coursewareResumeNotice
		if len(draft.Slides) == 3 && phase == "preview" && draft.Generation.RepairPage == 0 {
			err = tx.QueryRow(r.Context(), "SELECT pptx_path FROM artifact_revisions WHERE task_id=$1 AND result->'slides'=$2::jsonb AND result->>'theme'=$3 AND result->>'title'=$4 AND pptx_path<>'' ORDER BY revision DESC LIMIT 1", current, asJSON(draft.Slides), draft.Theme, draft.Title).Scan(&previewPath)
			if err != nil && err != pgx.ErrNoRows {
				return err
			}
			if _, statErr := os.Stat(previewPath); statErr != nil {
				previewPath = ""
			}
			if previewPath != "" {
				stage = "awaiting_preview"
				draft.Generation = nil
			}
		}
		result = draft
	} else if !input.PreviewFirst && reusableCourseware(draft, sources) {
		stage = "awaiting_confirmation"
		draft.NeedsReview = true
		draft.Evaluation = nil
		draft.Notice += " 已恢复上次草稿，请确认内容后继续制作。"
		result = draft
	}
	id := current
	cycle++
	if _, err = tx.Exec(r.Context(), "UPDATE members SET reserved=reserved+20 WHERE id=$1", a.MemberID); err != nil {
		return err
	}
	if stage == "pending" {
		previewPath = ""
	}
	_, err = tx.Exec(r.Context(), "UPDATE tasks SET status=$1,stage=$1,result=$2,sources=$3,pptx_path=$4,revision=revision+1,charge_state='reserved',charge_cycle=$5,error='',restored_at=NULL,capacity_hold=false,lease_until=NULL,next_attempt_at=NULL,automatic_failures=0,courseware_provider_calls=0,updated_at=now() WHERE id=$6", stage, asJSON(result), asJSON(sources), previewPath, cycle, id)
	if err != nil {
		return err
	}
	if previewPath != "" {
		if err = archiveRevision(r.Context(), tx, a.TenantID, id); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(r.Context(), "INSERT INTO ledger(tenant_id,member_id,kind,delta,balance_after,reference) VALUES($1,$2,'reserve',0,$3,$4)", a.TenantID, a.MemberID, balance, fmt.Sprintf("%s:%d", id, cycle)); err != nil {
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
