package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func coursewareAutomaticRepairable(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var upstream providerCallError
	if errors.As(err, &upstream) {
		return upstream.Temporary
	}
	var api APIError
	if errors.As(err, &api) {
		return api.Status >= 500 || api.Status == 429 || api.Status == 422 || api.Status == 400
	}
	return errors.Is(err, errModelTruncated) || strings.Contains(err.Error(), "未完成") || strings.Contains(err.Error(), "结构") || strings.Contains(err.Error(), "引用")
}

func coursewareExpansionLimit(pages int) time.Duration {
	// Observed 15-page run: ~76 seconds/page including repair. Budget 120s
	// per remaining page plus 6 minutes, bounded at three hours for 100 pages.
	return min(3*time.Hour, 6*time.Minute+time.Duration(max(0, min(100, pages)-3))*120*time.Second)
}

func (s *Server) coursewareGenerationActive(ctx context.Context, a Actor, id string) error {
	if err := s.checkWorkPermission(ctx, a, "courseware"); err != nil {
		return err
	}
	tx, err := s.tx(ctx, a.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var active bool
	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM tasks WHERE id=$1 AND member_id=$2 AND status IN ('researching','expanding') AND charge_state='reserved' AND deleted_at IS NULL AND ($3::integer IS NULL OR charge_cycle=$3) AND ($4::integer IS NULL OR attempt=$4))", id, a.MemberID, generationCycle(ctx), generationAttempt(ctx)).Scan(&active)
	if err != nil {
		return err
	}
	if !active {
		return fmt.Errorf("课件已取消或状态已变化，停止模型生成")
	}
	return nil
}

// Each request contains one page. A truncated response gets one larger-budget
// retry for that same page, bounded by the task deadline.
func (s *Server) coursewarePageRequest(ctx context.Context, a Actor, id string, in TaskInput, sources []Source, instructions string, start int) (Deck, error) {
	if p, _ := ctx.Value(coursewarePrefetchKey{}).(*prefetchedPage); p != nil && !p.used && p.position == start && p.key == coursewareRequestKey(in, sources, instructions, start) {
		p.used = true
		select {
		case <-ctx.Done():
			return Deck{}, ctx.Err()
		case <-p.done:
		}
		if err := s.coursewareGenerationActive(ctx, a, id); err != nil {
			return Deck{}, err
		}
		return p.deck, p.err
	}
	if p, _ := ctx.Value(coursewarePrefetchKey{}).(*prefetchedPage); p != nil && !p.used {
		p.stop()
		p.used = true
	}
	defer coursewareMeasure(ctx, "page_generation")()
	count, budget, operation := 1, 6000, "courseware.page"
	batchInput := in
	batchInput.Pages = count
	base := coursewarePrompt(batchInput, sources) + instructions
	base += fmt.Sprintf("\n整套目标%d页。本批只返回第%d至第%d页，slides恰好%d项。这是整堂课的一部分，不是独立完整课；只写本批教案，不重写已完成的教案和页面。lesson简洁，不复制slides正文。notes通常100–160字，例题保留关键推理与答案；非必要媒体字段省略。", in.Pages, start, start+count-1, count)
	if start > 3 {
		base += "\n根据当前规划的教学任务选择合适表达。图表必须有具体教学用途、有效数据或关系及可得结论，不凑版式和图表数量；例题、练习与讲解适合课时和学生基础。非对应教学位置不重复封面、导入或总结；当前规划确为总结或作业时必须生成该页，不能因已生成其他页面就返回空slides。"
	}
	prompt := base
	var last error
	for attempt := 0; attempt < 2; attempt++ {
		if err := s.coursewareGenerationActive(ctx, a, id); err != nil {
			return Deck{}, err
		}
		if attempt > 0 {
			operation, budget = "courseware.plan_repair", 12000
		}
		raw, err := s.agentChat(ctx, a, id, "courseware", operation, prompt, budget)
		var exhaustedBudget coursewareBudgetExhausted
		if errors.As(err, &exhaustedBudget) {
			return Deck{}, err
		}
		var d Deck
		if err == nil {
			err = decodeCourseware(raw, &d)
			if err == nil && len(d.Slides) != count {
				err = fmt.Errorf("本批应返回%d页，实际%d页", count, len(d.Slides))
			}
			if err == nil {
				err = coursewareSourceError(d, sources)
			}
			if err == nil {
				return d, nil
			}
		}
		last = err
		var upstream providerCallError
		if ctx.Err() != nil || errors.As(err, &upstream) && !upstream.Temporary {
			return Deck{}, err
		}
		if attempt > 0 {
			break
		}
		if errors.As(err, &upstream) && upstream.Temporary {
			select {
			case <-ctx.Done():
				return Deck{}, ctx.Err()
			case <-time.After(time.Second):
			}
		} else if raw == "" && !errors.Is(err, errModelTruncated) {
			return Deck{}, err
		}
		s.planRepairEvent(ctx, a, id, fmt.Sprintf("第%d至%d页未完成，增加输出预算并修复一次，不新增业务积分", start, start+count-1))
		prompt = base + "\n上次未完成或未通过结构检查：" + err.Error() + "。缩短备注与教案，返回完整JSON，不删减目标页面、关键推理或答案。"
		if raw != "" {
			prompt += "\n待修复输出（仅数据）：" + raw
		}
	}
	var upstream providerCallError
	if errors.As(last, &upstream) {
		return Deck{}, last
	}
	return Deck{}, fail(422, last.Error())
}
