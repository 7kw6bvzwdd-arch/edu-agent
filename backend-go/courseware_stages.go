package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Server) stagedCoursewarePlan(ctx context.Context, a Actor, id string, in TaskInput, sources []Source, locked Deck) (Deck, error) {
	ctx = withCoursewareArtifacts(ctx)
	stageStarted := time.Now()
	var pending *prefetchedPage
	defer func() {
		pending.stop()
		s.coursewareEvent(ctx, a, id, "generation_timing", fmt.Sprintf("本阶段制作耗时%d毫秒（不含教师等待）；重叠环节不可相加。%s", time.Since(stageStarted).Milliseconds(), coursewareTimingReport(ctx)))
	}()
	d := locked
	if d.Title == "" {
		d.Title = in.Topic
	} // A representative preview is not the whole lesson title.
	d.NeedsReview = true
	d.Slides = append([]Slide{}, locked.Slides...)
	if in.Detail != "" && d.Plan == nil {
		plan, err := s.planCourseware(ctx, a, id, in, sources)
		if err != nil {
			return d, err
		}
		d.Plan = plan
	}
	if d.Plan != nil {
		in.Pages = len(d.Plan.Pages)
		d.EditableFormulas = true
	}
	phase, end := "expand", max(in.Pages, len(d.Slides))
	if len(d.Slides) < 3 || d.Generation != nil && d.Generation.Phase == "preview" {
		phase, end = "preview", 3
	}
	if s.cfg.Mode == "live" && s.cfg.CoursewareContentReviewDisabled && !strings.Contains(d.Notice, contentReviewDisabledNotice) {
		d.Notice += " " + contentReviewDisabledNotice
	}
	if d.Generation == nil {
		d.Generation = &CoursewareCheckpoint{}
	} else {
		cp := *d.Generation
		d.Generation = &cp
	}
	d.Generation.Version, d.Generation.Phase = coursewarePageRepairVersion, phase
	if d.Theme == "" {
		d.Theme = in.Theme
	}
	if in.Detail != "" && d.PageChecks == nil {
		d.PageChecks = map[string]PageQualityCheck{}
	}
	if len(d.Slides) > end {
		return d, fmt.Errorf("检查点页数超过阶段目标，请人工核对；未改动已有页面")
	}
	tx, err := s.tx(ctx, a.TenantID)
	if err != nil {
		return d, err
	}
	reviews, err := slideReviews(ctx, tx, id, d)
	tx.Rollback(ctx)
	if err != nil {
		return d, err
	}
	approved := map[string]bool{}
	for _, m := range reviews {
		if m.Approved && m.Vote >= 0 {
			approved[m.ID] = true
		}
	}
	attempts := map[int]int{}
	for {
		if err := s.coursewareGenerationActive(ctx, a, id); err != nil {
			return d, err
		}
		page, issue := d.Generation.RepairPage, d.Generation.Issue
		if page == 0 {
			// Locate a structural failure on exactly one saved page before adding more.
			for i := range d.Slides {
				if err := coursewarePageIssue(d, i+1, sources); err != nil {
					page, issue = i+1, err.Error()
					break
				}
			}
		}
		if page == 0 {
			for i, sl := range d.Slides {
				if d.Plan == nil {
					break
				}
				check := d.PageChecks[sl.ID]
				if check.Hash == slideContentHash(d, sl) && s.pageCheckPassed(check, s.cfg.Mode == "sample") {
					continue
				}
				one := d
				one.Slides = []Slide{sl}
				pptx, checkErr := s.renderCoursewarePage(ctx, &one, i+1)
				if checkErr == nil {
					check, checkErr = s.inspectGeneratedPage(ctx, a, id, in, one, pptx)
				}
				check.Hash = slideContentHash(d, sl)
				d.PageChecks[sl.ID] = check
				if saveErr := s.saveCoursewareCheckpoint(ctx, a, id, d, sources); saveErr != nil {
					return d, saveErr
				}
				if checkErr != nil {
					var unavailable pageReviewUnavailable
					var upstream providerCallError
					var api APIError
					if errors.As(checkErr, &unavailable) || errors.As(checkErr, &upstream) && upstream.Temporary || errors.As(checkErr, &api) && (api.Status >= 500 || api.Status == 429) {
						return d, checkErr
					}
					page, issue = i+1, checkErr.Error()
					break
				}
			}
		}
		if page == 0 && len(d.Slides) < end {
			page = len(d.Slides) + 1
		}
		if page == 0 && phase == "expand" {
			issues := coursewareQualityIssues(d, in.Pages)
			if s.cfg.Mode != "sample" {
				issues = append(issues, coursewareFunctionIssues(in, d)...)
			}
			if len(issues) > 0 {
				issue = issues[0]
				page = coursewareRepairTarget(d, issue, approved)
				if page == 0 {
					return d, fmt.Errorf("%s；没有可自动修改的未审核页面，已有页面已保留", issue)
				}
			}
		}
		if page == 0 {
			// Render without automatic splitting: page positions and approved hashes
			// are stable. A renderer page error returns to the same local repair loop.
			if _, err := s.renderCoursewareAvailable(ctx, d); err != nil {
				var problem coursewareRenderError
				if !errors.As(err, &problem) || problem.Page < 1 || problem.Page > len(d.Slides) {
					return d, err
				}
				page, issue = problem.Page, problem.Error()
			} else {
				break
			}
		}
		if page < 1 || page > len(d.Slides)+1 || page > end {
			return d, fmt.Errorf("检查点页码无效，已保留草稿")
		}
		if page <= len(d.Slides) && approved[d.Slides[page-1].ID] {
			return d, fmt.Errorf("第%d页已审核，自动修复未覆盖该页：%s", page, issue)
		}
		d.Generation.RepairPage, d.Generation.Issue = page, issue
		if err := s.saveCoursewareCheckpoint(ctx, a, id, d, sources); err != nil {
			return d, err
		}
		if attempts[page] >= 3 {
			return d, coursewareRepairExhausted{Page: page, Issue: issue}
		}
		attempts[page]++
		s.coursewareEvent(ctx, a, id, fmt.Sprintf("batch_start_%d_%d", page, page), fmt.Sprintf("正在制作或修复第%d页，只参考本页及相邻页，其他页面保持不变", page))
		if pending != nil && (pending.page != page || issue != "") {
			pending.stop()
			pending = nil
		}
		pageCtx := ctx
		if pending != nil {
			pageCtx = context.WithValue(pageCtx, coursewarePrefetchKey{}, pending)
		}
		pageCtx = context.WithValue(pageCtx, coursewareAbortNextKey{}, func() { pending.stop(); pending = nil })
		pageCtx = context.WithValue(pageCtx, coursewareNextPageKey{}, func(candidate Deck) {
			// Only append work can look ahead, after its content check. Repairs never
			// speculate; failure invalidates the candidate before anything is saved.
			if s.cfg.Mode != "live" || d.Plan == nil || issue != "" || page != len(d.Slides)+1 || page >= end {
				return
			}
			if pending != nil {
				pending.stop()
				pending = nil
			}
			nextDeck := d
			nextDeck.Slides = append(append([]Slide(nil), d.Slides...), candidate.Slides[0])
			if nextDeck.Title == "" {
				nextDeck.Title = candidate.Title
			}
			if nextDeck.Lesson == "" {
				nextDeck.Lesson = candidate.Lesson
			}
			nextIn, instructions, position, prepareErr := s.coursewarePageInstructions(ctx, a, id, in, nextDeck, page+1, "", phase)
			if prepareErr != nil {
				return
			}
			preCtx, stop := context.WithCancel(ctx)
			p := &prefetchedPage{page: page + 1, position: position, key: coursewareRequestKey(nextIn, sources, instructions, position), cancel: stop, done: make(chan struct{})}
			pending = p
			go func() {
				ticker := time.NewTicker(2 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-p.done:
						return
					case <-preCtx.Done():
						return
					case <-ticker.C:
						if s.coursewareGenerationActive(preCtx, a, id) != nil {
							stop()
							return
						}
					}
				}
			}()
			go func() {
				defer close(p.done)
				p.deck, p.err = s.coursewarePageRequest(preCtx, a, id, nextIn, sources, instructions, position)
			}()
		})
		sl, title, lesson, err := s.coursewareSinglePage(pageCtx, a, id, in, sources, d, page, issue, phase)
		if err != nil {
			pending.stop()
			pending = nil
		}
		if err == nil {
			if activeErr := s.coursewareGenerationActive(ctx, a, id); activeErr != nil {
				return d, activeErr
			}
		}
		if err != nil && sl.Title == "" {
			if coursewareAutomaticRepairable(err) && attempts[page] < 3 && ctx.Err() == nil {
				d.Generation.Issue = err.Error()
				if saveErr := s.saveCoursewareCheckpoint(ctx, a, id, d, sources); saveErr != nil {
					return d, saveErr
				}
				s.coursewareEvent(ctx, a, id, "repairing", fmt.Sprintf("第%d页检查未通过，自动修复后继续；无需手动确认，其他页面保持不变", page))
				select {
				case <-ctx.Done():
					return d, ctx.Err()
				case <-time.After(time.Second):
				}
				continue
			}
			return d, err
		}
		if d.Title == "" {
			d.Title = title
		}
		// One concise route plus authoritative per-slide notes; local repairs never
		// append a second full lesson or rewrite another page's teaching notes.
		if d.Lesson == "" {
			d.Lesson = lesson
		}
		if page <= len(d.Slides) {
			sl.ID = d.Slides[page-1].ID
			d.Slides[page-1] = sl
		} else {
			sl.ID = ""
			d.Slides = append(d.Slides, sl)
		}
		assignSlideIDs(&d)
		if in.Detail != "" {
			saved := d.Slides[page-1]
			key := fmt.Sprintf("position-%d", saved.Order)
			check := d.PageChecks[key]
			check.Hash = slideContentHash(d, saved)
			d.PageChecks[saved.ID] = check
			delete(d.PageChecks, key)
		}
		d.Generation.RepairPage, d.Generation.Issue = 0, ""
		if err := s.saveCoursewareCheckpoint(ctx, a, id, d, sources); err != nil {
			return d, err
		}
		if err != nil {
			return d, err
		}
		s.coursewareEvent(ctx, a, id, fmt.Sprintf("batch_complete_%d", page), fmt.Sprintf("第%d页已保存，继续制作不会重新生成其他完成页", page))
	}
	d.NeedsReview = true
	if phase == "expand" {
		restoreTeachingOrder(&d)
	}
	d.Lesson = checkpointLesson(d)
	d.Notice = "教学与视觉规范已应用；请逐页核对事实、例题答案和引用。已完成页按检查点保留，自动修复仅替换问题页。"
	if s.cfg.Mode == "live" && s.cfg.CoursewareContentReviewDisabled {
		d.Notice += " " + contentReviewDisabledNotice
	}
	if phase == "preview" {
		d.Notice += " 当前仅制作三页质量预览，逐页确认后再生成剩余页面；未结算。"
	}
	if s.cfg.Mode == "sample" {
		d.Notice += " 本地内置样例，不是实时AI生成。"
	}
	if err := s.saveCoursewareCheckpoint(ctx, a, id, d, sources); err != nil {
		return d, err
	}
	return d, nil
}

func (s *Server) researchCoursewarePreview(ctx context.Context, a Actor, id string, in TaskInput, sources []Source) error {
	txRead, err := s.tx(ctx, a.TenantID)
	if err != nil {
		return err
	}
	st, err := ownedCourseware(ctx, txRead, a, id, "")
	txRead.Rollback(ctx)
	if err != nil {
		return err
	}
	if st.Deck.Generation != nil {
		sources = st.Sources
	}
	d, err := s.stagedCoursewarePlan(ctx, a, id, in, sources, st.Deck)
	if err != nil {
		return err
	}
	path, err := s.previewArtifact(ctx, a, id, &d)
	if err != nil {
		return err
	}
	tx, err := s.tx(ctx, a.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	d.Generation = nil
	if d.Plan != nil {
		in.Pages = len(d.Plan.Pages)
		if _, err = tx.Exec(ctx, "UPDATE tasks SET input=$1 WHERE id=$2", asJSON(in), id); err != nil {
			return err
		}
	}
	res, err := tx.Exec(ctx, "UPDATE tasks SET result=$1,sources=$2,pptx_path=$3,status='awaiting_preview',stage='awaiting_preview',revision=revision+1,lease_until=NULL,automatic_failures=0,updated_at=now() WHERE id=$4 AND member_id=$5 AND status='researching' AND charge_state='reserved' AND deleted_at IS NULL AND ($6::integer IS NULL OR charge_cycle=$6) AND ($7::integer IS NULL OR attempt=$7)", asJSON(d), asJSON(sources), path, id, a.MemberID, generationCycle(ctx), generationAttempt(ctx))
	if err != nil || res.RowsAffected() != 1 {
		return err
	}
	if err = archiveRevision(ctx, tx, a.TenantID, id); err != nil {
		return err
	}
	if err = event(ctx, tx, a.TenantID, id, "awaiting_preview", "三页PPTX质量预览已生成；逐页确认后制作其余页面，完整课件按教学顺序呈现"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Server) expandCourseware(ctx context.Context, a Actor, id string, in TaskInput, locked Deck) error {
	tx, err := s.tx(ctx, a.TenantID)
	if err != nil {
		return err
	}
	st, err := ownedCourseware(ctx, tx, a, id, "")
	tx.Rollback(ctx)
	if err != nil {
		return err
	}
	if len(locked.Slides) < 3 {
		return fmt.Errorf("扩展必须从已确认的前三页开始")
	}
	d, err := s.stagedCoursewarePlan(ctx, a, id, in, st.Sources, locked)
	if err != nil {
		return err
	}
	path, err := s.previewArtifact(ctx, a, id, &d)
	if err != nil {
		return err
	}
	tx, err = s.tx(ctx, a.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	d.Generation = nil
	res, err := tx.Exec(ctx, "UPDATE tasks SET result=$1,pptx_path=$2,status='awaiting_confirmation',stage='awaiting_confirmation',revision=revision+1,lease_until=NULL,automatic_failures=0,updated_at=now() WHERE id=$3 AND status='expanding' AND charge_state='reserved' AND deleted_at IS NULL AND ($4::integer IS NULL OR charge_cycle=$4) AND ($5::integer IS NULL OR attempt=$5)", asJSON(d), path, id, generationCycle(ctx), generationAttempt(ctx))
	if err != nil || res.RowsAffected() != 1 {
		return err
	}
	if err = archiveRevision(ctx, tx, a.TenantID, id); err != nil {
		return err
	}
	if err = event(ctx, tx, a.TenantID, id, "awaiting_confirmation", "剩余课件已生成；前三页保持不变，请逐页审核修改后完整导出，尚未结算"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
