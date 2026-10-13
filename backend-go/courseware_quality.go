package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// A narrow deterministic guard for the observed category-only homework page.
// It does not try to prove arbitrary questions complete; diagrams and genuine
// prose still need the independent content reviewer.
func exerciseVisibilityIssues(d Deck) []PageQualityIssue {
	strip := strings.NewReplacer("两点法", "", "求解析式", "", "判断", "", "读图", "", "作图", "", "分层", "", "作业", "", "基础题", "", "巩固", "", "提升", "", "必做", "", "选做", "", "组", "", "由两点", "", "与", "", "A", "", "B", "", "C", "", "Ａ", "", "Ｂ", "", "Ｃ", "")
	issues := []PageQualityIssue{}
	for _, sl := range d.Slides {
		if sl.Layout != "exercise" || len(sl.Bullets) == 0 || sl.Graph != nil || sl.Scientific != nil || sl.Table != nil || sl.Figure != nil || sl.Chart != nil || sl.Picture != nil || sl.Diagram != "" || sl.Formula != "" {
			continue
		}
		classificationOnly := true
		for _, b := range sl.Bullets {
			remaining := strings.Map(func(r rune) rune {
				if unicode.IsSpace(r) || unicode.IsPunct(r) {
					return -1
				}
				return r
			}, strip.Replace(b))
			if remaining != "" {
				classificationOnly = false
				break
			}
		}
		if classificationOnly {
			issues = append(issues, PageQualityIssue{Object: sl.Title + "的可见题目", Reviewer: "content", Category: "pedagogy", Certainty: "confirmed", Severity: "error", Evidence: "可见正文只有作业分组、必做/选做或判断作图的分类，没有具体题设与任务；备注不能替代学生可见题目。", Repair: "将备注中已有的具体条件、数值与问题移入本页正文，保持答案仅在备注。"})
		}
	}
	return issues
}

// These checks catch thin or repetitive slide structure, not factual correctness.
func coursewareQualityIssues(d Deck, teachingPages int) []string {
	issues := []string{}
	titles, bodies, layouts := map[string]int{}, map[string]int{}, map[string]bool{}
	examples, exercises := 0, 0
	for i, sl := range d.Slides {
		page := i + 1
		if (sl.Table != nil || sl.Figure != nil) && len(sl.Bullets) > 2 {
			issues = append(issues, fmt.Sprintf("第%d页已有教学图解，正文保留1–2条短问题，其余讲解移到notes", page))
		}
		layout := sl.Layout
		if layout != "" {
			layouts[layout] = true
		}
		if layout == "example" {
			examples++
		}
		if layout == "exercise" {
			exercises++
		}
		title := strings.ToLower(strings.Join(strings.Fields(sl.Title), ""))
		if prior := titles[title]; prior > 0 {
			issues = append(issues, fmt.Sprintf("第%d页与第%d页重复标题", page, prior))
		}
		titles[title] = page
		if len(sl.Bullets) == 0 {
			issues = append(issues, fmt.Sprintf("第%d页正文为空，请补具体课堂内容", page))
		}
		body := make([]string, len(sl.Bullets))
		for j, bullet := range sl.Bullets {
			body[j] = strings.ToLower(strings.Join(strings.Fields(bullet), ""))
			if body[j] == "" {
				issues = append(issues, fmt.Sprintf("第%d页含空白正文要点", page))
			}
		}
		joined := strings.Join(body, "|")
		if prior := bodies[joined]; prior > 0 {
			issues = append(issues, fmt.Sprintf("第%d页与第%d页重复正文", page, prior))
		}
		bodies[joined] = page
		if i > 0 && len([]rune(strings.TrimSpace(sl.Notes))) < 25 {
			issues = append(issues, fmt.Sprintf("第%d页讲解备注过少，请写出依据、提问或答案", page))
		}
	}
	if len(layouts) == 1 && len(d.Slides) >= 8 {
		issues = append(issues, "整套课件版式重复，请核对教学任务是否采用合适表达，不为凑版式拆页")
	}
	if examples == 0 {
		issues = append(issues, "缺少例题解析页面")
	}
	if exercises == 0 {
		issues = append(issues, "缺少课堂练习页面")
	}
	// Layout continuations must not add new teaching requirements.
	if d.Plan == nil && teachingPages >= 12 && (examples < 2 || exercises < 2) {
		issues = append(issues, "12页及以上课件应包含至少两道递进例题和两道独立练习")
	}
	return issues
}

func coursewareFunctionIssues(in TaskInput, d Deck) []string {
	if !strings.Contains(in.Topic, "一次函数") {
		return nil
	}
	graphs, comparison := 0, false
	for _, sl := range d.Slides {
		if sl.Graph != nil {
			graphs++
			comparison = comparison || len(sl.Graph.Series) > 1
		}
	}
	issues := []string{}
	if graphs < 2 {
		issues = append(issues, "缺少函数图像：至少2页使用graph真实数值坐标图，分别支撑作图和性质解释")
	}
	if !comparison {
		issues = append(issues, "缺少函数图像对比：至少一页graph展示两条以上直线，使用相同坐标比例比较k或b")
	}
	return issues
}

func (s *Server) coursewarePreflight(ctx context.Context, d *Deck, teachingPages int) ([]string, error) {
	issues := coursewareQualityIssues(*d, teachingPages)
	// Invalid media must reach the same bounded draft repair as text overflow.
	if err := validateDeck(*d); err != nil {
		return append(issues, err.Error()), nil
	}
	if _, err := s.renderCourseware(ctx, d); err != nil {
		var validation APIError
		if errors.As(err, &validation) && validation.Status == 422 {
			return append(issues, validation.Message), nil
		}
		return nil, err
	}
	return issues, nil
}
