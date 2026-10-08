package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// These checks catch thin or repetitive slide structure, not factual correctness.
func coursewareQualityIssues(d Deck, teachingPages int) []string {
	issues := []string{}
	titles, bodies, layouts := map[string]int{}, map[string]int{}, map[string]bool{}
	examples, exercises, visuals := 0, 0, 0
	for i, sl := range d.Slides {
		page := i + 1
		if sl.Table != nil || sl.Figure != nil || sl.Chart != nil || sl.Diagram != "" {
			visuals++
		}
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
	if len(layouts) < 4 {
		issues = append(issues, "整套课件版式过于单一，请按教学任务使用至少4种布局")
	}
	if examples == 0 {
		issues = append(issues, "缺少例题解析页面")
	}
	if exercises == 0 {
		issues = append(issues, "缺少课堂练习页面")
	}
	// Layout continuations must not add new teaching requirements.
	if teachingPages >= 12 && (examples < 2 || exercises < 2) {
		issues = append(issues, "12页及以上课件应包含至少两道递进例题和两道独立练习")
	}
	if visuals < 2 {
		issues = append(issues, "缺少教学图表：至少2页使用与知识点相关的图表、表格或图解，不能用装饰凑数")
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
