package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
)

type coursewareRenderError struct {
	APIError
	Code, Field string
	Page        int
}

func (e coursewareRenderError) Unwrap() error { return e.APIError }

// Retry the renderer with the same data; an outage must not regenerate pages.
func (s *Server) renderCoursewareAvailable(ctx context.Context, d Deck) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := s.cfg.RenderURL + ":" + coursewareRenderKey(d)
	if cached := artifactCacheGet(ctx, key); cached != nil {
		coursewareMeasure(ctx, "render_cache_hit")()
		return cached, nil
	}
	defer coursewareMeasure(ctx, "render")()
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		b, err := s.renderPPTX(ctx, d)
		var api APIError
		var upstream providerCallError
		temporary := errors.As(err, &api) && (api.Status >= 500 || api.Status == 429) || errors.As(err, &upstream) && upstream.Temporary
		if err == nil {
			artifactCachePut(ctx, key, b)
		}
		if err == nil || !temporary || attempt >= 2 {
			return b, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// A checkpoint page cannot be split or change its neighbours. Compact only
// text identified by the renderer and keep each complete original in notes.
func (s *Server) renderCoursewarePage(ctx context.Context, d *Deck, page int) ([]byte, error) {
	original := d.Slides[0]
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		b, err := s.renderCoursewareAvailable(ctx, *d)
		if err == nil {
			return b, nil
		}
		var issue coursewareRenderError
		if !errors.As(err, &issue) {
			return nil, err
		}
		issue.APIError.Message = strings.Replace(issue.APIError.Message, "第1页：", fmt.Sprintf("第%d页：", page), 1)
		issue.Page = page
		if attempt >= 16 || issue.Code != "text_overflow" || !compactCoursewarePageText(&d.Slides[0], issue.Field) {
			return nil, issue
		}
		sl := &d.Slides[0]
		sl.Notes = original.Notes
		for _, value := range []struct{ label, before, after string }{
			{"标题", original.Title, sl.Title}, {"核心问题", original.Focus, sl.Focus}, {"章节", original.Section, sl.Section},
			{"正文要点", strings.Join(original.Bullets, "\n"), strings.Join(sl.Bullets, "\n")},
			{"流程说明", string(asJSON(original.Figure)), string(asJSON(sl.Figure))},
			{"函数图标题", graphTitle(original.Graph), graphTitle(sl.Graph)},
		} {
			if value.before != value.after {
				sl.Notes += "\n" + value.label + "（完整原文）：" + value.before
			}
		}
	}
}

func compactCoursewarePageText(sl *Slide, field string) bool {
	var ignored string
	compact := func(text string, floor, limit int) string {
		oneLine := strings.Join(strings.Fields(text), " ")
		if oneLine != text && len([]rune(oneLine)) <= limit {
			return oneLine
		}
		return shortLabel(oneLine, min(limit, max(floor, len([]rune(oneLine))*3/4)), &ignored, "")
	}
	var value *string
	floor, limit := 12, 26
	switch field {
	case "graph":
		if sl.Graph != nil && len([]rune(sl.Graph.Title)) > 12 {
			copy := *sl.Graph
			copy.Title = "函数图像"
			sl.Graph = &copy
			return true
		}
		if sl.Graph != nil && sl.Focus != "" {
			sl.Focus = ""
			return true
		}
		return false
	case "title":
		value = &sl.Title
	case "focus":
		value, floor, limit = &sl.Focus, 16, 32
	case "section":
		value, limit = &sl.Section, 24
	case "table", "figure":
		// A panel can overflow because neighbouring text reserves its space.
		// Release the optional focus strip before touching evidence; renderCoursewarePage
		// keeps its complete text in notes. Never truncate table cells or numeric data.
		if sl.Focus != "" {
			sl.Focus = ""
			return true
		}
		if compactCoursewarePageText(sl, "bullets") {
			return true
		}
		if field == "figure" && sl.Figure != nil && (sl.Figure.Kind == "flow" || sl.Figure.Kind == "timeline") {
			figure := *sl.Figure
			figure.Items = append([]FigureItem{}, figure.Items...)
			for i, item := range figure.Items {
				if item.Detail != "" {
					figure.Items[i].Detail = ""
					sl.Figure = &figure
					return true
				}
			}
		}
		return false
	case "bullets":
		before := strings.Join(sl.Bullets, "\n")
		sl.Bullets = append([]string{}, sl.Bullets...)
		for i, text := range sl.Bullets {
			oneLine := strings.Join(strings.Fields(text), " ")
			runes := []rune(oneLine)
			limit := min(120, max(18, len(runes)*3/4))
			// Keep complete sentences/steps. Cutting at an arbitrary character can
			// sever a number, unit or condition and trigger another review failure.
			cut := 0
			for j, r := range runes {
				if j+1 <= limit && strings.ContainsRune("。；;！？!?", r) {
					cut = j + 1
				}
			}
			if len(runes) > limit && cut > 0 {
				oneLine = string(runes[:cut])
			}
			sl.Bullets[i] = oneLine
		}
		if strings.Join(sl.Bullets, "\n") == before && len(sl.Bullets) > 1 {
			sl.Bullets = sl.Bullets[:len(sl.Bullets)-1]
		}
		return strings.Join(sl.Bullets, "\n") != before
	default:
		return false
	}
	before := *value
	*value = compact(before, floor, limit)
	return *value != before
}

func graphTitle(g *FunctionGraph) string {
	if g == nil {
		return ""
	}
	return g.Title
}

// Repair deterministic layout failures only. Invalid formulas, images and
// numeric relationships remain errors; never silently delete them to pass.
func (s *Server) renderCourseware(ctx context.Context, d *Deck) ([]byte, error) {
	if d.Generation != nil || d.Plan != nil {
		return s.renderCoursewareAvailable(ctx, *d)
	}
	for attempt := 0; attempt < 32; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		b, err := s.renderCoursewareAvailable(ctx, *d)
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
