package main

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

func animIsShape(kind string) bool { return kind == "rectangle" || kind == "right_triangle" }

// Mathematical units: rotate the base shape around (cx,cy), then translate.
func animGeometryVertices(kind string, p map[string]float64) [][2]float64 {
	w, h := p["width"], p["height"]
	points := [][2]float64{{0, 0}, {w, 0}, {0, h}}
	if kind == "rectangle" {
		points = append(points[:2], [2]float64{w, h}, [2]float64{0, h})
	}
	angle := p["angle"] * math.Pi / 180
	c, s := math.Cos(angle), math.Sin(angle)
	for i, v := range points {
		x, y := v[0]-p["cx"], v[1]-p["cy"]
		points[i] = [2]float64{p["cx"] + x*c - y*s + p["tx"], p["cy"] + x*s + y*c + p["ty"]}
	}
	return points
}
func animGeometryArea(o AnimObject) float64 {
	area := o.Params["width"] * o.Params["height"]
	if o.Type == "right_triangle" {
		area /= 2
	}
	return area
}
func animGeometryProblems(s AnimScene) ([]string, bool) {
	var problems []string
	add := func(f string, args ...any) {
		problems = append(problems, fmt.Sprintf("幕%s：", s.ID)+fmt.Sprintf(f, args...))
	}
	var shape, axes AnimObject
	count, measures := 0, 0
	for _, o := range s.Objects {
		if animIsShape(o.Type) {
			shape = o
			count++
		}
		if o.Type == "axes" {
			axes = o
		}
		if o.Type == "measure" {
			measures++
		}
	}
	if count != 1 {
		add("几何原型每幕须且仅须一个矩形或直角三角形")
		return problems, false
	}
	if measures != 1 {
		add("每幕须且仅须一个几何测量面板")
	}
	if axes.ID == "" || shape.Axes != axes.ID {
		add("图形须引用本幕坐标系")
		return problems, false
	}
	for _, o := range []AnimObject{shape, axes} {
		for _, v := range o.Params {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				add("几何参数必须是有限数")
				return problems, false
			}
		}
	}
	sideCount := 0
	for _, o := range s.Objects {
		if o.Slot == "side" {
			sideCount++
		}
	}
	if sideCount > 2 {
		add("几何原型最多测量面板与一个辅助面板")
	}
	for _, o := range s.Objects {
		if o.Type == "panel" && len(o.Items) > 2 {
			add("几何辅助面板最多两条")
		}
	}
	for _, v := range shape.Params {
		if !animNear(v*100, math.Round(v*100)) {
			add("几何参数最多两位小数")
		}
	}
	ap := axes.Params
	for _, k := range []string{"xmin", "xmax", "ymin", "ymax"} {
		if math.Abs(ap[k]) > 20 {
			add("几何坐标边界须在负20至20内")
		}
	}
	if ap["xmax"]-ap["xmin"] < 2 || ap["ymax"]-ap["ymin"] < 2 {
		add("几何坐标跨度至少2个单位")
	}
	p := map[string]float64{}
	for k, v := range shape.Params {
		p[k] = v
	}
	for _, k := range []string{"width", "height"} {
		if p[k] < 0.5 || p[k] > 10 {
			add("图形边长须为0.5至10个单位")
		}
	}
	for _, k := range []string{"tx", "ty", "cx", "cy"} {
		if math.Abs(p[k]) > 20 {
			add("位移与旋转中心参数须在负20至20内")
		}
	}
	if math.Abs(p["angle"]) > 360 {
		add("角度须在负360至360度内")
	}
	if len(problems) > 0 {
		return problems, false
	}
	check := func() {
		points := animGeometryVertices(shape.Type, p)
		points = append(points, [2]float64{p["cx"] + p["tx"], p["cy"] + p["ty"]})
		for _, v := range points {
			if v[0] < ap["xmin"]+0.4 || v[0] > ap["xmax"]-0.4 || v[1] < ap["ymin"]+0.4 || v[1] > ap["ymax"]-0.4 {
				add("图形或旋转中心超出坐标范围，须为标签预留0.4单位边距")
				break
			}
		}
	}
	check()
	moving := false
	for _, a := range s.Actions {
		if a.Object != shape.ID || a.Op != "tween" || a.To == nil || !animContains(animTweenable[shape.Type], a.Param) {
			continue
		}
		from, to := p[a.Param], *a.To
		if math.IsNaN(to) || math.IsInf(to, 0) || math.Abs(to) > map[bool]float64{true: 360, false: 20}[a.Param == "angle"] {
			add("变换目标须为有限且在范围内的数值")
			continue
		}
		if !animNear(to*100, math.Round(to*100)) {
			add("几何变换目标最多两位小数")
		}
		if !animNear(from, to) {
			moving = true
		}
		// Check every circular-arc extremum, including extrema between keyframes.
		if a.Param == "angle" {
			base := map[string]float64{"width": p["width"], "height": p["height"]}
			for _, v := range animGeometryVertices(shape.Type, base) {
				phase := math.Atan2(v[1]-p["cy"], v[0]-p["cx"]) * 180 / math.Pi
				lo, hi := math.Min(from, to), math.Max(from, to)
				for n := int(math.Ceil((lo + phase) / 90)); float64(n)*90-phase < hi; n++ {
					p["angle"] = float64(n)*90 - phase
					check()
				}
			}
		}
		p[a.Param] = to
		check()
	}
	problems = append(problems, animGeometryNarrationProblems(s, shape)...)
	return problems, moving
}

const animGeometryNumber = `(-?[0-9]+(?:[.][0-9]+)?|[负零〇一二两三四五六七八九十百点]+)`

var animAreaClaim = regexp.MustCompile(`面积(?:仍然是|仍是|仍为|等于|是|为)[ ]*` + animGeometryNumber)
var animMoveClaim = regexp.MustCompile(`向(右|左|上|下)平移[ ]*` + animGeometryNumber)
var animAngleClaim = regexp.MustCompile(`旋转[ ]*` + animGeometryNumber + `[ ]*度`)

func animReadNumber(s string) (float64, bool) {
	if v, e := strconv.ParseFloat(s, 64); e == nil {
		return v, true
	}
	negative := strings.HasPrefix(s, "负")
	s = strings.TrimPrefix(s, "负")
	parts := strings.Split(s, "点")
	if len(parts) > 2 {
		return 0, false
	}
	digits := map[rune]int{'零': 0, '〇': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	n, d := 0, 0
	for _, ch := range parts[0] {
		if ch == '十' || ch == '百' {
			unit := 10
			if ch == '百' {
				unit = 100
			}
			if d == 0 {
				d = 1
			}
			n += d * unit
			d = 0
		} else if v, ok := digits[ch]; ok {
			d = d*10 + v
		} else {
			return 0, false
		}
	}
	value := float64(n + d)
	if len(parts) == 2 {
		scale := 0.1
		for _, ch := range parts[1] {
			v, ok := digits[ch]
			if !ok {
				return 0, false
			}
			value += float64(v) * scale
			scale /= 10
		}
	}
	if negative {
		value = -value
	}
	return value, true
}
func animGeometryNarrationProblems(s AnimScene, shape AnimObject) []string {
	var out []string
	area := animGeometryArea(shape)
	values := map[string]float64{}
	for k, v := range shape.Params {
		values[k] = v
	}
	type motion struct {
		sentence, param string
		delta           float64
	}
	var moves []motion
	for _, a := range s.Actions {
		if a.Object == shape.ID && a.Op == "tween" && a.To != nil {
			moves = append(moves, motion{a.At.Sentence, a.Param, *a.To - values[a.Param]})
			values[a.Param] = *a.To
		}
	}
	texts := append([]AnimSentence{}, s.Narration...)
	for _, o := range s.Objects {
		if o.Text != "" {
			texts = append(texts, AnimSentence{ID: o.ID, Display: o.Text})
		}
		for _, it := range o.Items {
			texts = append(texts, AnimSentence{ID: o.ID, Display: it})
		}
	}
	for _, n := range texts {
		for _, text := range []string{n.Display, n.Speech} {
			if strings.ContainsAny(text, "？?") {
				continue
			}
			for _, m := range animAreaClaim.FindAllStringSubmatch(text, -1) {
				if v, ok := animReadNumber(m[1]); ok && !animNear(v, area) {
					out = append(out, fmt.Sprintf("幕%s句子%s：面积描述与程序计算的%g不一致", s.ID, n.ID, area))
				}
			}
			relevant := []motion{}
			for _, m := range moves {
				if m.sentence == n.ID {
					relevant = append(relevant, m)
				}
			}
			if len(relevant) == 0 && len(moves) == 1 {
				relevant = moves
			}
			for _, m := range animMoveClaim.FindAllStringSubmatch(text, -1) {
				v, ok := animReadNumber(m[2])
				if !ok {
					continue
				}
				param, sign := "tx", 1.0
				if m[1] == "上" || m[1] == "下" {
					param = "ty"
				}
				if m[1] == "左" || m[1] == "下" {
					sign = -1
				}
				found := false
				for _, move := range relevant {
					found = found || (move.param == param && animNear(move.delta, v*sign))
				}
				if !found {
					out = append(out, fmt.Sprintf("幕%s句子%s：平移方向或距离与动作不一致", s.ID, n.ID))
				}
			}
			ccw := strings.Contains(text, "逆时针")
			cw := strings.Contains(strings.ReplaceAll(text, "逆时针", ""), "顺时针")
			if ccw != cw {
				found := false
				for _, move := range relevant {
					found = found || (move.param == "angle" && ((ccw && move.delta > 0) || (cw && move.delta < 0)))
				}
				if !found {
					out = append(out, fmt.Sprintf("幕%s句子%s：旋转方向与角度变化不一致", s.ID, n.ID))
				}
			}
			for _, m := range animAngleClaim.FindAllStringSubmatch(text, -1) {
				v, ok := animReadNumber(m[1])
				if !ok {
					continue
				}
				found := false
				for _, move := range relevant {
					found = found || (move.param == "angle" && animNear(math.Abs(move.delta), v))
				}
				if !found {
					out = append(out, fmt.Sprintf("幕%s句子%s：旋转角度与动作不一致", s.ID, n.ID))
				}
			}
		}
	}
	return out
}

// Prefer the explicit topic; supporting notes must not silently switch archetypes.
func animRequestedArchetype(in TaskInput) string {
	if in.Subject != "" && in.Subject != "数学" {
		return "lesson.explain@1"
	}
	for _, text := range []string{in.Topic, in.Requirements} {
		if strings.Contains(text, "一次函数") {
			return "func.params@1"
		}
		if animHasAny(text, "平移", "旋转", "矩形", "直角三角形") {
			return "geo.transform@1"
		}
	}
	return "lesson.explain@1"
}
