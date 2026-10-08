package main

import (
	"fmt"
	"reflect"
	"strings"
)

// Content is never silently truncated: shortened labels retain their complete
// value in notes; long cells become labelled, editable text continuation pages.
func shortLabel(value string, limit int, notes *string, label string) string {
	text := []rune(value)
	if len(text) <= limit {
		return value
	}
	*notes += "\n" + label + "（完整原文）：" + value
	return string(text[:limit-1]) + "…"
}

func textParts(value string, limit int) []string {
	runes := []rune(value)
	parts := []string{}
	for len(runes) > 0 {
		n := min(limit, len(runes))
		if n < len(runes) {
			for end := n; end > n/2; end-- {
				if strings.ContainsRune("。；！？;!?\n", runes[end-1]) {
					n = end
					break
				}
			}
		}
		parts = append(parts, string(runes[:n]))
		runes = runes[n:]
	}
	return parts
}

func continuation(sl Slide, label string, number int) Slide {
	sl.Title = shortLabel(sl.Title, 38, &sl.Notes, "页面标题") + fmt.Sprintf(" · %s%d", label, number)
	sl.Table = nil
	sl.Figure = nil
	sl.Chart = nil
	sl.Diagram = ""
	sl.Picture = nil
	sl.Formula = ""
	sl.Focus = ""
	sl.Emphasis = nil
	sl.Bullets = []string{fmt.Sprintf("原页内容接续 · %s %d", label, number)}
	return sl
}

func tableText(sl Slide) []string {
	lines := []string{}
	for r, row := range sl.Table.Rows {
		fields := []string{}
		for c, cell := range row {
			header := fmt.Sprintf("原始第%d列", c+1)
			if c < len(sl.Table.Headers) {
				header = sl.Table.Headers[c]
			}
			fields = append(fields, header+"："+cell)
		}
		lines = append(lines, fmt.Sprintf("第%d条记录｜%s", r+1, strings.Join(fields, "；")))
	}
	return lines
}

func tableRecordPages(sl Slide) []Slide {
	lines := tableText(sl)
	if sl.Table.Title != "" {
		sl.Notes += "\n原表格标题：" + sl.Table.Title
		sl.Title = sl.Table.Title
	}
	return labelledTextPages(sl, lines, "记录")
}

func labelledTextPages(sl Slide, lines []string, label string) []Slide {
	pieces := []string{}
	for _, line := range lines {
		pieces = append(pieces, textParts(line, 72)...)
	}
	pages := []Slide{}
	for i := 0; i < len(pieces); {
		part := continuation(sl, label, len(pages)+1)
		part.Layout = "steps"
		end, chars := i, 0
		for end < len(pieces) && end-i < 4 && chars+len([]rune(pieces[end])) <= 160 {
			chars += len([]rune(pieces[end]))
			end++
		}
		part.Bullets = pieces[i:end]
		pages = append(pages, part)
		i = end
	}
	return pages
}

// Normalize body density before validation. Additional pages contain the
// original wording; nothing is summarized away or sent back to a model.
func arrangeCoursewareText(d Deck) Deck {
	pages := []Slide{}
	changed := false
	for _, sl := range d.Slides {
		limit := 6
		if sl.Table != nil || sl.Figure != nil {
			limit = 2
		}
		tooLong := len(sl.Bullets) > limit
		for _, line := range sl.Bullets {
			tooLong = tooLong || len([]rune(line)) > 160
		}
		if !tooLong {
			pages = append(pages, sl)
			continue
		}
		pieces := []string{}
		for _, line := range sl.Bullets {
			pieces = append(pieces, textParts(line, 72)...)
		}
		if len(pieces) == 0 {
			pages = append(pages, sl)
			continue
		}
		sl.Bullets = pieces[:1]
		pages = append(pages, sl)
		pages = append(pages, labelledTextPages(sl, pieces[1:], "讲解续页")...)
		changed = true
	}
	d.Slides = pages
	if changed {
		d.Notice += " 过长正文已按原文拆为续页。"
	}
	return d
}

func arrangeCoursewareTables(d Deck) Deck {
	pages := []Slide{}
	changed := false
	for _, original := range d.Slides {
		if original.Table == nil {
			pages = append(pages, original)
			continue
		}
		sl := original
		// Clone both levels: generation, history restore and edits share snapshots.
		t := *original.Table
		t.Headers = append([]string(nil), t.Headers...)
		t.Rows = make([][]string, len(original.Table.Rows))
		cols := len(t.Headers)
		for i, row := range original.Table.Rows {
			t.Rows[i] = append([]string(nil), row...)
			cols = max(cols, len(row))
		}
		if cols == 0 && len(t.Rows) == 0 && t.Title == "" {
			sl.Table = nil
			pages = append(pages, sl)
			changed = true
			continue
		}
		// Resource limits remain enforced; don't expand arbitrarily large input.
		if cols > 40 || len(t.Rows) > 100 {
			pages = append(pages, sl)
			continue
		}
		cols = max(2, cols)
		incomplete := false
		for len(t.Headers) < cols {
			t.Headers = append(t.Headers, "")
			incomplete = true
		}
		for c, h := range t.Headers {
			if strings.TrimSpace(h) == "" {
				t.Headers[c] = fmt.Sprintf("第%d列（未命名）", c+1)
				incomplete = true
			}
		}
		if len(t.Rows) == 0 {
			t.Rows = [][]string{make([]string, cols)}
			incomplete = true
		}
		longCell := false
		for r, row := range t.Rows {
			for len(row) < cols {
				row = append(row, "")
				incomplete = true
			}
			for c, cell := range row {
				if strings.TrimSpace(cell) == "" {
					row[c] = "未提供"
					incomplete = true
				}
				if len([]rune(cell)) > 60 {
					longCell = true
				}
			}
			t.Rows[r] = row
		}
		if incomplete {
			sl.Notes += "\n表格有未提供或未命名字段，已明确标记；未推算或补造数据，请教师核对。"
			d.NeedsReview = true
		}
		t.Title = shortLabel(t.Title, 60, &sl.Notes, "表格标题")
		for c, h := range t.Headers {
			t.Headers[c] = shortLabel(h, 24, &sl.Notes, fmt.Sprintf("第%d列表头", c+1))
		}
		sl.Table = &t
		if longCell {
			// Preserve every cell and its row/column association on visible pages.
			records := tableRecordPages(sl)
			sl.Table = nil
			pages = append(pages, sl)
			pages = append(pages, records...)
			changed = true
			continue
		}
		if cols <= 4 && len(t.Rows) <= 5 {
			if reflect.DeepEqual(sl, original) {
				sl = original
			}
			pages = append(pages, sl)
			changed = changed || !reflect.DeepEqual(sl, original)
			continue
		}
		partNumber := 0
		// Repeat the identifying first column on every horizontal continuation.
		for start := 1; start < cols; start += 3 {
			indices := []int{0}
			for c := start; c < min(start+3, cols); c++ {
				indices = append(indices, c)
			}
			for first := 0; first < len(t.Rows); first += 5 {
				part := sl
				if partNumber > 0 {
					part = continuation(sl, "表格续页", partNumber)
				}
				partNumber++
				sub := TeachingTable{Title: t.Title}
				for _, c := range indices {
					sub.Headers = append(sub.Headers, t.Headers[c])
				}
				for _, row := range t.Rows[first:min(first+5, len(t.Rows))] {
					cells := []string{}
					for _, c := range indices {
						cells = append(cells, row[c])
					}
					sub.Rows = append(sub.Rows, cells)
				}
				part.Table = &sub
				if partNumber > 1 {
					base := sl.Title
					if t.Title != "" {
						base = t.Title
					}
					part.Title = shortLabel(base, 38, &part.Notes, "表格标题") + fmt.Sprintf("（续%d）", partNumber-1)
					part.Bullets = []string{fmt.Sprintf("第%d–%d条记录 · %s", first+1, min(first+5, len(t.Rows)), strings.Join(sub.Headers, " / "))}
				}
				pages = append(pages, part)
			}
		}
		changed = true
	}
	d.Slides = pages
	if changed {
		d.Notice += " 已自动整理表格并保留数据；超大表格分开展示，缺失值明确标记，完整长标签见备注。"
	}
	return d
}
