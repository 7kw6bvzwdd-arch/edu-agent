package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Kept inside the task result so the existing ownership, retention and deletion
// rules also cover checkpoints. Only the server writes this field.
type CoursewareCheckpoint struct {
	Version      string      `json:"version"`
	Phase        string      `json:"phase"`
	RepairPage   int         `json:"repair_page,omitempty"`
	Issue        string      `json:"issue,omitempty"`
	PageAttempts map[int]int `json:"page_attempts,omitempty"`
}

const coursewareLessonPages = "\n\n## 逐页授课记录（随页面同步）\n"

type generationCycleKey struct{}
type generationAttemptKey struct{}

func generationCycle(ctx context.Context) any   { return ctx.Value(generationCycleKey{}) }
func generationAttempt(ctx context.Context) any { return ctx.Value(generationAttemptKey{}) }

func checkpointLesson(d Deck) string {
	intro, _, _ := strings.Cut(d.Lesson, coursewareLessonPages)
	var lesson strings.Builder
	lesson.WriteString(intro)
	lesson.WriteString(coursewareLessonPages)
	for i, sl := range d.Slides {
		fmt.Fprintf(&lesson, "\n### 第%d页：%s\n\n%s\n", i+1, sl.Title, sl.Notes)
	}
	return lesson.String()
}

// Snapshots may contain fewer pages than the active checkpoint. Only advertise
// or serve an image whose saved page content still matches the current slide.
func (s *Server) coursewarePreviewPages(ctx context.Context, tx pgx.Tx, id string, d Deck, path string) (map[string]string, error) {
	artifact := d
	if d.Generation != nil || path == "" {
		err := tx.QueryRow(ctx, "SELECT result,pptx_path FROM artifact_revisions WHERE task_id=$1 AND pptx_path<>'' ORDER BY revision DESC LIMIT 1", id).Scan(&artifact, &path)
		if err == pgx.ErrNoRows {
			return map[string]string{}, nil
		}
		if err != nil {
			return nil, err
		}
	}
	current := map[string]string{}
	for _, sl := range d.Slides {
		current[sl.ID] = slideContentHash(d, sl)
	}
	pages := map[string]string{}
	for i, sl := range artifact.Slides {
		if path == "" || current[sl.ID] != slideContentHash(artifact, sl) {
			continue
		}
		png := filepath.Join(path+".slides", fmt.Sprintf("slide-%d.png", i+1))
		if _, err := os.Stat(png); err == nil || s.cloud.objects != nil {
			pages[sl.ID] = png
		}
	}
	return pages, nil
}

func (s *Server) saveCoursewareCheckpoint(ctx context.Context, a Actor, id string, d Deck, sources []Source) error {
	progress := d.Generation != nil && d.Generation.RepairPage == 0 && len(d.Slides) > 0
	if progress && d.Plan != nil {
		for _, sl := range d.Slides {
			check := d.PageChecks[sl.ID]
			if check.Hash != slideContentHash(d, sl) || !s.pageCheckPassed(check, s.cfg.Mode == "sample") {
				progress = false
				break
			}
		}
	}
	tx, err := s.tx(ctx, a.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	res, err := tx.Exec(ctx, `UPDATE tasks SET result=$1,sources=$2,revision=revision+1,updated_at=now(),
		automatic_failures=CASE WHEN $7 THEN 0 ELSE automatic_failures END
		WHERE id=$3 AND member_id=$4 AND status IN ('researching','expanding') AND charge_state='reserved' AND deleted_at IS NULL AND ($5::integer IS NULL OR charge_cycle=$5) AND ($6::integer IS NULL OR attempt=$6)`, asJSON(d), asJSON(sources), id, a.MemberID, generationCycle(ctx), generationAttempt(ctx), progress)
	if err != nil {
		return err
	}
	if res.RowsAffected() != 1 {
		return fmt.Errorf("制作已停止，已保存页面保持不变")
	}
	return tx.Commit(ctx)
}

// Full slide bodies never escape this three-page window. The teaching route is
// a short task list, not a second copy of the deck or its lesson notes.
func coursewarePageContext(d Deck, page int) string {
	window := map[string]any{"page": page, "title": d.Title, "theme": d.Theme, "previous": nil, "current": nil, "next": nil}
	if d.Plan != nil {
		position := page
		if page <= len(d.Slides) && d.Slides[page-1].Order > 0 {
			position = d.Slides[page-1].Order
		} else {
			schedule := plannedSchedule(d.Plan)
			if page <= len(schedule) {
				position = schedule[page-1].Position
			}
		}
		window["page"] = position
		for _, sl := range d.Slides {
			for name, target := range map[string]int{"previous": position - 1, "current": position, "next": position + 1} {
				if sl.Order == target {
					copy := slideForTextModel(sl)
					copy.ID = ""
					window[name] = copy
				}
			}
		}
		return "\n相邻教学页面上下文（仅数据）：" + string(asJSON(window))
	}
	for name, index := range map[string]int{"previous": page - 2, "current": page - 1, "next": page} {
		if index >= 0 && index < len(d.Slides) {
			copy := slideForTextModel(d.Slides[index])
			copy.ID = ""
			window[name] = copy
		}
	}
	return "\n相邻页面上下文（仅数据，null表示不存在或尚未生成）：" + string(asJSON(window))
}

func coursewarePageIssue(d Deck, page int, sources []Source) error {
	one := d
	one.Slides = d.Slides[page-1 : page]
	// The combined lesson includes other pages. Validate only this page here;
	// each model response's lesson citations are checked before it is saved.
	one.Lesson = "本页讲解见notes。"
	if err := validateDeckMinimum(one, 1); err != nil {
		return err
	}
	if err := coursewareSourceError(one, sources); err != nil {
		return err
	}
	sl := one.Slides[0]
	if len(sl.Bullets) == 0 {
		return fmt.Errorf("正文为空")
	}
	for _, b := range sl.Bullets {
		if strings.TrimSpace(b) == "" {
			return fmt.Errorf("存在空白要点")
		}
	}
	if len([]rune(strings.TrimSpace(sl.Notes))) < 25 {
		return fmt.Errorf("讲解备注过少，请写出依据、提问或答案")
	}
	if issues := exerciseVisibilityIssues(one); len(issues) > 0 {
		return fmt.Errorf("%s", blockingPageIssues(issues))
	}
	return nil
}

// Missing deck-wide requirements are assigned to one unapproved page; they
// never trigger a new deck. Existing example/exercise sections are preferred.
func coursewareRepairTarget(d Deck, issue string, approved map[string]bool) int {
	match := regexp.MustCompile(`第(\d+)页`).FindStringSubmatch(issue)
	if len(match) > 1 {
		p, _ := strconv.Atoi(match[1])
		if p > 0 && p <= len(d.Slides) && !approved[d.Slides[p-1].ID] {
			return p
		}
		return 0
	}
	wanted := ""
	if strings.Contains(issue, "例题") {
		wanted = "example"
	}
	if strings.Contains(issue, "练习") && !strings.Contains(issue, "例题") {
		wanted = "exercise"
	}
	if strings.Contains(issue, "两道递进") {
		count := 0
		for _, sl := range d.Slides {
			if sl.Layout == "example" {
				count++
			}
		}
		if count >= 2 {
			wanted = "exercise"
		}
	}
	if strings.Contains(issue, "函数图像") {
		for i, sl := range d.Slides {
			if i < 3 || i == len(d.Slides)-1 || approved[sl.ID] {
				continue
			}
			if sl.Graph == nil || strings.Contains(issue, "对比") && len(sl.Graph.Series) < 2 {
				return i + 1
			}
		}
		return 0
	}
	for pass := 0; pass < 2; pass++ {
		for i, sl := range d.Slides {
			if i < 3 || i == len(d.Slides)-1 || approved[sl.ID] {
				continue
			}
			if wanted != "" {
				if sl.Layout == wanted || sl.Layout == "example" || sl.Layout == "exercise" {
					continue
				}
				label := "例题"
				if wanted == "exercise" {
					label = "练习"
				}
				if pass == 0 && !strings.Contains(sl.Title+sl.Section, label) {
					continue
				}
			} else if strings.Contains(issue, "图表") && (sl.Graph != nil || sl.Chart != nil || sl.Figure != nil || sl.Table != nil || sl.Diagram != "") {
				continue
			}
			return i + 1
		}
	}
	return 0
}

func (s *Server) coursewarePageInstructions(ctx context.Context, a Actor, id string, in TaskInput, d Deck, page int, issue, phase string) (TaskInput, string, int, error) {
	if d.Theme != "" {
		in.Theme = d.Theme
	}
	prompt := coursewarePageRepairSkill + coursewarePageContext(d, page)
	prompt += "\n表达由本页教学任务决定：观察关系突出实际学科图，计算推导保留关键步骤，对照突出同一比较条件，独立练习突出完整题设。每个图表必须让学生读出本页所需的量、关系或证据，在notes简述具体读图路径；没有具体任务或数据就不生成装饰图、占位表或空色块。相邻页重复图像时突出本页新增的对象/问题，必要复习保留；不为凑版式而换色、换题或拆页。"
	prompt += "\n独立练习所有上屏字段（bullets、focus、formula和图示）都不得提前给出待求答案；focus只能追问或提示方法，formula只能保留题设已给原式。要求配方、求顶点或最值时，不在formula追加配方结果或顶点式；完整推导与答案放answer和notes；answer按实际解题顺序每行一步，用换行分隔，供教师逐步揭示，不用分号自动拆解。题设已给顶点式、验证给定恒等式及例题讲解允许显示相应公式。读图题保留给定图像，独立作图题不提前画答案曲线。"
	if in.Subject == "物理" {
		prompt += "\n受力平衡不能代替浸入状态条件：F浮=G可能是漂浮或悬浮，不能据此推出V排=V物。只有全浸没才有V排=V物，漂浮通常V排<V物；题目及教师解释都须正确区分。"
	}
	prompt += "\n专业图示标题只用简短用途，尽量12字以内；函数公式与必要坐标由图例展示，长解释放notes。数学函数坐标默认无量纲，x/y每单位使用相同长度；物理函数必须明确对应物理量及单位。图内只截取坐标范围，不改变函数定义域。"
	prompt += "\n数学图中点和曲线均根据给定函数式计算；数据性质明确写‘按题设计算，非实测’，坐标比例写‘x/y每单位等长’，不使用含混的‘真实数据/真实坐标’措辞，也不虚构测量或教材核验。"
	if page == 1 && len(d.Slides) == 0 {
		tx, err := s.tx(ctx, a.TenantID)
		if err != nil {
			return in, "", 0, err
		}
		prior, err := previousTeachingFeedback(ctx, tx, a, in)
		tx.Rollback(ctx)
		if err != nil {
			return in, "", 0, err
		}
		prompt += prior
	}
	position := page
	if d.Plan != nil {
		row, err := teachingPageFor(d, page)
		if err != nil {
			return in, "", 0, err
		}
		position = row.Position
		prompt += "\n整课已确认教学规划（用途与约定，不是页面正文）：" + string(asJSON(d.Plan)) + "\n当前仅制作该教学位置：" + string(asJSON(row)) + "\n预览页不连续时，不把上一张预览当教学前页，按教学位置衔接。"
		prompt += "\n同一例题以plan.conventions中的符号、数值和条件为准；visual是表达草案，如果其中示例数字冲突，保留图示用途并采用conventions的数据，不另外换题设。实际不同例题必须明确区分，不能静默混用。"
	}
	if page <= len(d.Slides) {
		tx, err := s.tx(ctx, a.TenantID)
		if err != nil {
			return in, "", 0, err
		}
		feedback, err := feedbackInstructions(ctx, tx, id, d, d.Slides[page-1].ID)
		tx.Rollback(ctx)
		if err != nil {
			return in, "", 0, err
		}
		prompt += feedback
	}
	if d.Plan == nil {
		prompt += fmt.Sprintf("\n整课教学路线（只规划用途）：1学习目标，2真实问题导入，3核心概念，4探究证据，5例题，6独立练习，7误区辨析，末页总结迁移；当前第%d/%d页。已保存课件主题为%s。", page, in.Pages, d.Theme)
	}
	if phase == "preview" && d.Plan == nil {
		prompt += fmt.Sprintf("\n整套目标%d页，本阶段只生成前三页：封面/学习目标、真实问题导入、首个核心概念。只返回本次指定的一页。", in.Pages)
	}
	if page == 1 && len(d.Slides) == 0 {
		prompt += "\n首次lesson简述全课前置知识、重难点、各段时长与教学路线；未制作页只列任务，不写全文。"
	}
	if issue != "" {
		prompt += "\n本页需处理的问题：" + issue
	}
	if page > 3 {
		prompt += "\n已完成页面中待补齐的教学要求：" + strings.Join(coursewareQualityIssues(d, in.Pages), "；")
		prompt += "\n函数图像专项要求：" + strings.Join(coursewareFunctionIssues(in, d), "；")
	}
	return in, prompt, position, nil
}

func (s *Server) coursewareSinglePage(ctx context.Context, a Actor, id string, in TaskInput, sources []Source, d Deck, page int, issue, phase string) (Slide, string, string, error) {
	in, prompt, position, err := s.coursewarePageInstructions(ctx, a, id, in, d, page, issue, phase)
	if err != nil {
		return Slide{}, "", "", err
	}
	var last error
	var candidate Slide
	var candidateTitle, candidateLesson string
	for attempt := 0; attempt < 2; attempt++ {
		var next Deck
		if s.cfg.Mode == "sample" {
			next = sampleDeck(in)
			next.Slides = next.Slides[position-1 : position]
		} else {
			var err error
			s.coursewareEvent(ctx, a, id, "page_generation", fmt.Sprintf("教学位置第%d页正在生成内容，当前页最多自动修复三轮", position))
			next, err = s.coursewarePageRequest(ctx, a, id, in, sources, prompt, position)
			if err != nil {
				if candidate.Title != "" {
					return candidate, candidateTitle, candidateLesson, err
				}
				return Slide{}, "", "", err
			}
		}
		next.Generation = nil
		next.NeedsReview = true
		if d.Plan != nil {
			next.Slides[0].Order = position
			next.EditableFormulas = true
		}
		next.Theme = in.Theme
		if d.Title != "" {
			next.Title = d.Title
		}
		if page <= len(d.Slides) && d.Slides[page-1].Picture != nil {
			next.Slides[0].Picture = d.Slides[page-1].Picture
		}
		if next.Lesson == "" {
			next.Lesson = "教学路线按实际学科与已确认规划组织，具体教学任务与反馈详见每页讲解备注。"
		}
		last = coursewarePageIssue(next, 1, sources)
		if last == nil {
			var pptx []byte
			s.coursewareEvent(ctx, a, id, "page_render", fmt.Sprintf("教学位置第%d页内容已生成，正在排版并制作真实PPT截图", position))
			pptx, last = s.renderCoursewarePage(ctx, &next, page)
			if last == nil {
				candidate, candidateTitle, candidateLesson = next.Slides[0], next.Title, next.Lesson
			}
			var api APIError
			if last != nil && (!errors.As(last, &api) || api.Status >= 500 || api.Status == 429) {
				// Generated content is sound; renderer availability is not a reason
				// to spend another model call on the same page after a restart.
				return next.Slides[0], next.Title, next.Lesson, last
			}
			if last == nil && in.Detail != "" {
				s.coursewareEvent(ctx, a, id, "page_review", fmt.Sprintf("教学位置第%d页正在审核实际PPT画面，审核通过后保存", position))
				next.Plan = d.Plan
				check, err := s.inspectGeneratedPageWithNext(ctx, a, id, in, next, pptx, func() {
					if start, _ := ctx.Value(coursewareNextPageKey{}).(func(Deck)); start != nil {
						start(next)
					}
				})
				last = err
				if err != nil {
					if abort, _ := ctx.Value(coursewareAbortNextKey{}).(func()); abort != nil {
						abort()
					}
				}
				if d.PageChecks == nil {
					d.PageChecks = map[string]PageQualityCheck{}
				}
				checkID := fmt.Sprintf("position-%d", position)
				d.PageChecks[checkID] = check
				var unavailable pageReviewUnavailable
				if errors.As(err, &unavailable) {
					return next.Slides[0], next.Title, next.Lesson, err
				}
			}
		}
		if last == nil {
			return next.Slides[0], next.Title, next.Lesson, nil
		}
		if attempt == 0 && s.cfg.Mode != "sample" {
			s.planRepairEvent(ctx, a, id, fmt.Sprintf("正在调整第%d页的内容或图表，其他页面已保存，无需操作", position))
			prompt += "\n本页检查未通过：" + last.Error() + "\n待修页数据：" + string(asJSON(deckForTextModel(next)))
			var layout coursewareRenderError
			if errors.As(last, &layout) && layout.Code == "text_overflow" && (layout.Field == "graph" || layout.Field == "scientific") {
				prompt += "\n图示内部文字过密：专业图示/graph的title只写简短教学用途，尽量12字以内，不把公式、作图步骤和结论重复写进图示标题；函数表达式由曲线图自动生成图例，无须重复堆入title。每条曲线只标本页必要的至多两个mark_x，完整坐标计算与说明放notes；保留真实函数参数、有效坐标范围、必要条件及所有原始数值，不通过缩小刻度或错误改数来挤空间。"
			}
			if errors.As(last, &layout) && layout.Code == "text_overflow" && (layout.Field == "table" || layout.Field == "figure") {
				prompt += "\n改用紧凑的证据版式：表头仅写变量和单位，计算表达式放formula或notes，不把长句塞进表头；表格与公式同屏通常选2–3条足以验证本页结论的代表性记录，完整原始记录保留在notes，不改数值、不伪造数据。保留支持本页结论的关键对照与适用条件，缺失的已知条件明确补齐，不为省空间删去必要证据。流程图保留有意义的步骤短标签，详细操作放notes；避免表格、长流程说明、公式、提示条同时挤占空间。不改变本页教学用途，不新增或重写其他页。"
			}
		} else {
			break
		}
	}
	if candidate.Title != "" {
		return candidate, candidateTitle, candidateLesson, fmt.Errorf("第%d页未完成：%w", page, last)
	}
	return Slide{}, "", "", fmt.Errorf("第%d页未完成：%w", page, last)
}
