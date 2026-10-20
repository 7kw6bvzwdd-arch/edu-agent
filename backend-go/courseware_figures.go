package main

import (
	"fmt"
	"math"
)

// Keep adjacent nodes across continuation boundaries. Interval values must be
// globally valid before splitting, so a bad relationship cannot disappear.
func arrangeCoursewareFigures(d Deck) Deck {
	pages := []Slide{}
	changed := false
	for _, original := range d.Slides {
		if original.Figure == nil {
			pages = append(pages, original)
			continue
		}
		f := *original.Figure
		limit := 4
		if f.Kind == "timeline" {
			limit = 6
		}
		if (f.Kind != "flow" && f.Kind != "timeline" && f.Kind != "route" && f.Kind != "interval") || len(f.Items) < 2 || len(f.Items) > 40 {
			pages = append(pages, original)
			continue
		}
		validValues := true
		if f.Kind == "interval" {
			validValues = len(f.Values) == len(f.Items)
			for i, v := range f.Values {
				if math.IsNaN(v) || math.IsInf(v, 0) || (i > 0 && v <= f.Values[i-1]) {
					validValues = false
				}
			}
			if len(f.Values) > 1 && math.IsInf(f.Values[len(f.Values)-1]-f.Values[0], 0) {
				validValues = false
			}
		} else if len(f.Values) > 4 {
			validValues = false
		}
		if !validValues {
			pages = append(pages, original)
			continue
		}
		sl := original
		f.Title = shortLabel(f.Title, 60, &sl.Notes, "图解标题")
		longText := false
		for _, item := range f.Items {
			longText = longText || len([]rune(item.Label)) > 24 || len([]rune(item.Detail)) > 60
		}
		if !longText && len(f.Items) <= limit && f.Title == original.Figure.Title {
			pages = append(pages, original)
			continue
		}
		var explanation []Slide
		if longText {
			// Preserve full explanations visibly instead of shrinking or dropping.
			lines := []string{}
			f.Items = append([]FigureItem(nil), f.Items...)
			for i, item := range f.Items {
				lines = append(lines, fmt.Sprintf("节点%d｜%s：%s", i+1, item.Label, item.Detail))
				f.Items[i].Label = shortLabel(item.Label, 24, &sl.Notes, "节点标签")
				f.Items[i].Detail = shortLabel(item.Detail, 60, &sl.Notes, "节点说明")
			}
			explanation = labelledTextPages(sl, lines, "图解说明")
			changed = true
		}
		for first, partNo := 0, 0; ; first, partNo = first+limit-1, partNo+1 {
			end := min(first+limit, len(f.Items))
			part := sl
			if partNo > 0 {
				part = continuation(sl, "图解续页", partNo)
				part.Bullets = []string{fmt.Sprintf("节点%d–%d · 与上一页首尾衔接", first+1, end)}
			}
			sub := f
			sub.Items = append([]FigureItem(nil), f.Items[first:end]...)
			if f.Kind == "interval" {
				sub.Values = append([]float64(nil), f.Values[first:end]...)
				part.Notes += "\n分段区间图：各续页按本页区间单独缩放，不跨页比较线段长度。"
			}
			part.Figure = &sub
			pages = append(pages, part)
			if end == len(f.Items) {
				break
			}
		}
		pages = append(pages, explanation...)
		changed = changed || len(f.Items) > limit || f.Title != original.Figure.Title
	}
	d.Slides = pages
	if changed {
		d.Notice += " 已自动整理图解续页，保留节点顺序、说明和区间数值。"
	}
	return d
}
