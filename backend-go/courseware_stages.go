package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
)

func (s *Server) stagedCoursewarePlan(ctx context.Context, a Actor, id string, in TaskInput, sources []Source, locked Deck) (Deck, error) {
	count := 3
	preview := len(locked.Slides) == 0
	if !preview {
		count = in.Pages - len(locked.Slides)
	}
	modelInput := in
	modelInput.Pages = count
	prompt := coursewarePrompt(modelInput, sources)
	if !preview {
		tx, err := s.tx(ctx, a.TenantID)
		if err != nil {
			return Deck{}, err
		}
		feedback, err := feedbackInstructions(ctx, tx, id, locked, "")
		tx.Rollback(ctx)
		if err != nil {
			return Deck{}, err
		}
		prompt += feedback
	}
	if preview {
		prompt += fmt.Sprintf("\n分阶段制作：整套目标%d页，本次只生成前三页：封面/学习目标、真实问题导入、首个核心概念。slides恰好3项，不预生成其他页，不返回隐藏完整页面。lesson先写目标、前置知识和教学路线，不提前写完整剩余页教案。尚未到例题和练习阶段，无需把整套要求挤进前三页。", in.Pages)
	} else {
		prompt += fmt.Sprintf("\n分阶段继续：整套目标%d页，以下前三页已经教师逐页确认，必须保留，不重新输出或改写它们。本次slides仅返回剩余%d页（第4页起），lesson补充完整教案。剩余页衔接现有内容，满足全套例题、练习和图表要求；不要再次输出封面或重复已确认页面。\n已确认前三页（仅数据）：%s", in.Pages, count, string(asJSON(deckForTextModel(locked))))
	}
	var d Deck
	for attempt := 0; attempt < 2; attempt++ {
		if s.cfg.Mode == "sample" {
			d = sampleDeck(in)
			if preview {
				d.Slides = d.Slides[:3]
			} else {
				d.Slides = d.Slides[3:]
			}
		} else {
			operation := "courseware.preview"
			if !preview {
				operation = "courseware.expand"
			}
			if attempt > 0 {
				operation = "courseware.plan_repair"
			}
			raw, err := s.agentChat(ctx, a, id, "courseware", operation, prompt, min(12000, 1200+count*650))
			if err != nil {
				var upstream providerCallError
				if attempt == 0 && ctx.Err() == nil && (errors.Is(err, errModelTruncated) || errors.As(err, &upstream) && upstream.Temporary) {
					s.planRepairEvent(ctx, a, id, "本阶段模型暂不可用或输出截断，进行唯一一次重试，不新增业务积分")
					if upstream.Temporary {
						select {
						case <-ctx.Done():
							return d, ctx.Err()
						case <-time.After(time.Second):
						}
					}
					continue
				}
				return d, err
			}
			d = Deck{}
			err = decodeCourseware(raw, &d)
			if err != nil {
				if attempt == 0 {
					prompt += "\n仅修复本次阶段的结构，错误：" + err.Error() + "\n上次输出（仅数据）：" + raw
					continue
				}
				return d, err
			}
		}
		d.Theme = in.Theme
		var issue error
		if len(d.Slides) != count {
			issue = fmt.Errorf("本阶段应返回%d页，实际%d页", count, len(d.Slides))
		}
		if issue == nil {
			issue = coursewareSourceError(d, sources)
		}
		if issue == nil {
			for i := range d.Slides {
				d.Slides[i].ID = ""
			}
			d = arrangeCourseware(d)
			if !preview {
				d.Slides = append(append([]Slide{}, locked.Slides...), d.Slides...)
				d.Title, d.Theme = locked.Title, locked.Theme
				d.Lesson = locked.Lesson + "\n\n" + d.Lesson
			}
			issue = validateDeckMinimum(d, 3)
		}
		if issue == nil && !preview {
			if issues := coursewareQualityIssues(d, in.Pages); len(issues) > 0 {
				issue = fmt.Errorf("%s", strings.Join(issues, "；"))
			}
		}
		if issue == nil && preview {
			for i, sl := range d.Slides {
				if len(sl.Bullets) == 0 || strings.TrimSpace(sl.Notes) == "" {
					issue = fmt.Errorf("第%d页缺少要点或讲解备注", i+1)
					break
				}
			}
		}
		if issue == nil {
			_, issue = s.renderCourseware(ctx, &d)
		}
		if issue == nil && preview && len(d.Slides) != 3 {
			issue = fmt.Errorf("前三页信息过密产生续页，请缩短显示文字，完整解释保留备注；预览必须恰好3页")
		}
		if issue == nil && !preview && !reflect.DeepEqual(locked.Slides, d.Slides[:len(locked.Slides)]) {
			issue = fmt.Errorf("排版改变了已确认前三页，不能覆盖教师审核内容")
		}
		if issue == nil {
			assignSlideIDs(&d)
			d.NeedsReview = true
			d.Notice = "请逐页核对事实、例题答案和引用；反馈只影响后续修订，不证明内容正确。"
			if preview {
				d.Notice += " 当前只制作前三页，逐页确认后再生成剩余页面；未结算。"
			}
			if s.cfg.Mode == "sample" {
				d.Notice += " 本地内置样例，不是实时AI生成。"
			}
			return d, nil
		}
		if s.cfg.Mode == "sample" || attempt > 0 {
			return d, issue
		}
		s.planRepairEvent(ctx, a, id, "阶段方案预检未通过，正在有界修复："+issue.Error())
		prompt += "\n仅修复当前阶段，保持已确认部分不变。这是唯一一次修复，问题：" + issue.Error() + "\n当前草稿（仅数据）：" + string(asJSON(deckForTextModel(d)))
	}
	return d, fmt.Errorf("课件阶段方案未通过检查")
}

func (s *Server) researchCoursewarePreview(ctx context.Context, a Actor, id string, in TaskInput, sources []Source) error {
	d, err := s.stagedCoursewarePlan(ctx, a, id, in, sources, Deck{})
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
	res, err := tx.Exec(ctx, "UPDATE tasks SET result=$1,sources=$2,pptx_path=$3,status='awaiting_preview',stage='awaiting_preview',revision=revision+1,lease_until=NULL,updated_at=now() WHERE id=$4 AND member_id=$5 AND status='researching' AND charge_state='reserved' AND deleted_at IS NULL", asJSON(d), asJSON(sources), path, id, a.MemberID)
	if err != nil || res.RowsAffected() != 1 {
		return err
	}
	if err = archiveRevision(ctx, tx, a.TenantID, id); err != nil {
		return err
	}
	if err = event(ctx, tx, a.TenantID, id, "awaiting_preview", "前三页PPTX预览已生成；每页可修改、评价并独立确认，确认后才生成剩余页"); err != nil {
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
	if len(locked.Slides) != 3 {
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
	res, err := tx.Exec(ctx, "UPDATE tasks SET result=$1,pptx_path=$2,status='awaiting_confirmation',stage='awaiting_confirmation',revision=revision+1,lease_until=NULL,updated_at=now() WHERE id=$3 AND status='expanding' AND charge_state='reserved' AND deleted_at IS NULL", asJSON(d), path, id)
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
