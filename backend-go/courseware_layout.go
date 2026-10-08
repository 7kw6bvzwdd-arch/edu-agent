package main

import (
	"context"
	"errors"
	"reflect"
)

type coursewareRenderError struct {
	APIError
	Code, Field string
	Page        int
}

func (e coursewareRenderError) Unwrap() error { return e.APIError }

// Repair deterministic layout failures only. Invalid formulas, images and
// numeric relationships remain errors; never silently delete them to pass.
func (s *Server) renderCourseware(ctx context.Context, d *Deck) ([]byte, error) {
	for attempt := 0; attempt < 32; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		b, err := s.renderPPTX(ctx, *d)
		if err == nil {
			return b, nil
		}
		var issue coursewareRenderError
		if !errors.As(err, &issue) || issue.Code != "text_overflow" || issue.Page < 1 || issue.Page > len(d.Slides) {
			return nil, err
		}
		i := issue.Page - 1
		sl := d.Slides[i]
		parts := []Slide{}
		switch issue.Field {
		case "table":
			if sl.Table == nil {
				return nil, err
			}
			if sl.Figure != nil || sl.Picture != nil {
				other := continuation(sl, "图解分栏", issue.Page)
				other.Figure, other.Picture = sl.Figure, sl.Picture
				sl.Figure = nil
				sl.Picture = nil
				parts = []Slide{sl, other}
			} else if len(sl.Table.Rows) > 1 {
				left, right := *sl.Table, *sl.Table
				cut := (len(left.Rows) + 1) / 2
				left.Rows = left.Rows[:cut]
				right.Rows = right.Rows[cut:]
				other := continuation(sl, "记录续页", issue.Page)
				other.Table = &right
				sl.Table = &left
				parts = []Slide{sl, other}
			} else {
				records := tableRecordPages(sl)
				sl.Table = nil
				parts = append([]Slide{sl}, records...)
			}
		case "bullets":
			if len(sl.Bullets) == 0 {
				return nil, err
			}
			if len(sl.Bullets) == 1 && len([]rune(sl.Bullets[0])) <= 72 {
				return nil, err
			}
			lines := sl.Bullets
			short := textParts(lines[0], 72)
			if len(short) == 0 {
				return nil, err
			}
			sl.Bullets = short[:1]
			rest := append(append([]string{}, short[1:]...), lines[1:]...)
			parts = append([]Slide{sl}, labelledTextPages(sl, rest, "讲解续页")...)
		case "title", "focus", "section":
			if issue.Field == "title" {
				sl.Title = shortLabel(sl.Title, 26, &sl.Notes, "标题")
			}
			if issue.Field == "focus" {
				sl.Focus = shortLabel(sl.Focus, 32, &sl.Notes, "核心问题")
			}
			if issue.Field == "section" {
				sl.Section = shortLabel(sl.Section, 24, &sl.Notes, "章节")
			}
			if reflect.DeepEqual(sl, d.Slides[i]) && sl.Table != nil && len(sl.Bullets) > 0 {
				other := continuation(sl, "表格展示", issue.Page)
				other.Table = sl.Table
				sl.Table = nil
				parts = []Slide{sl, other}
			} else {
				parts = []Slide{sl}
			}
		default:
			return nil, err
		}
		if len(d.Slides)-1+len(parts) > 100 || (len(parts) == 1 && reflect.DeepEqual(parts[0], d.Slides[i])) {
			return nil, err
		}
		next := append([]Slide{}, d.Slides[:i]...)
		next = append(next, parts...)
		next = append(next, d.Slides[i+1:]...)
		d.Slides = next
		if attempt == 0 {
			d.Notice += " 已自动修复文字或表格拥挤；采用续页或逐项记录保留原内容，请查看调整后的版式。"
		}
	}
	return nil, fail(422, "排版自动修复已达到安全次数上限，草稿已保留")
}
