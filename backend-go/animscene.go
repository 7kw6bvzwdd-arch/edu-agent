package main

// Teaching animation engine v2: author data written by the model. Rendering,
// subtitles, timing and coordinates are computed by the program (docs/animation-system-design.md §3).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const animScriptVersion = "anim-v2"

type AnimScript struct {
	Version        string      `json:"version"`
	Title          string      `json:"title"`
	Archetype      string      `json:"archetype"`
	Representative string      `json:"representative"`
	Scenes         []AnimScene `json:"scenes"`
}
type AnimScene struct {
	ID        string         `json:"id"`
	Intent    string         `json:"intent"`
	Title     string         `json:"title"`
	Objects   []AnimObject   `json:"objects"`
	Narration []AnimSentence `json:"narration"`
	Actions   []AnimAction   `json:"actions"`
	Holds     []AnimHold     `json:"holds"`
}

// AnimObject has no coordinate fields: slot picks a layout region, params are subject parameters.
type AnimObject struct {
	ID     string             `json:"id"`
	Type   string             `json:"type"`
	Slot   string             `json:"slot"`
	Axes   string             `json:"axes,omitempty"`
	Line   string             `json:"line,omitempty"`
	Text   string             `json:"text,omitempty"`
	Items  []string           `json:"items,omitempty"`
	Params map[string]float64 `json:"params,omitempty"`
}

// AnimSentence: subtitles are derived from Display; Speech is what TTS reads.
type AnimSentence struct {
	ID      string `json:"id"`
	Display string `json:"display"`
	Speech  string `json:"speech"`
}
type AnimTrigger struct {
	Sentence string `json:"sentence,omitempty"`
	Anchor   string `json:"anchor,omitempty"`
	OffsetMS int    `json:"offset_ms,omitempty"`
	After    string `json:"after,omitempty"`
	Scene    string `json:"scene,omitempty"`
}
type AnimAction struct {
	ID         string      `json:"id"`
	Op         string      `json:"op"`
	Object     string      `json:"object"`
	Param      string      `json:"param,omitempty"`
	To         *float64    `json:"to,omitempty"`
	At         AnimTrigger `json:"at"`
	DurationMS int         `json:"duration_ms,omitempty"`
}

// AnimHold: After is a sentence ID, or empty for the scene end.
type AnimHold struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	After string `json:"after"`
	MS    int    `json:"ms"`
}

var (
	animIDPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	animObjectSlots = map[string]string{"axes": "main", "line": "main", "point": "main", "equation": "side", "text": "side", "panel": "side"}
	animParams      = map[string][]string{"axes": {"xmin", "xmax", "ymin", "ymax"}, "line": {"k", "b"}, "point": {"x"}}
	animTweenable   = map[string][]string{"line": {"k", "b"}, "point": {"x"}}
	animDefaultMS   = map[string]int{"show": 600, "hide": 400, "emphasize": 1200, "tween": 1500}
	animHoldTypes   = map[string]bool{"read": true, "observe": true, "ask": true, "await": true, "transition": true}
	// TTS reads these literally or not at all; formulas must be written out in words.
	animSpeechSymbols = "=+-−×÷*/^<>≤≥|()（）[]{}√"
	animNumber        = `(-?\d+(?:\.\d+)?)`
	animKFromTo       = regexp.MustCompile(`k\s*从\s*` + animNumber + `\s*\S{0,3}?到\s*` + animNumber)
	animKValue        = regexp.MustCompile(`k\s*=\s*` + animNumber)
	animLineFormula   = regexp.MustCompile(`y\s*=\s*(-?\d*(?:\.\d+)?)\s*x\s*(?:([+-])\s*(\d+(?:\.\d+)?))?`)
	animPointLabel    = regexp.MustCompile(`^\(\s*` + animNumber + `\s*,\s*` + animNumber + `\s*\)$`)
)

// decodeAnimScript strictly decodes and validates author data. Unknown fields
// (subtitles, timelines, durations, coordinates) are build results or forbidden.
func decodeAnimScript(b []byte) (AnimScript, []string) {
	var sc AnimScript
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(&sc); e != nil {
		msg := e.Error()
		if i := strings.Index(msg, "unknown field "); i >= 0 {
			return sc, []string{"作者数据含不允许的字段" + strings.TrimPrefix(msg[i:], "unknown field ") + "：字幕、时间轴、音频时长与坐标由程序生成"}
		}
		return sc, []string{"作者数据不是有效JSON：" + msg}
	}
	return sc, validateAnimScript(sc)
}

func animRunes(s string, max int) bool { n := utf8.RuneCountInString(s); return n > 0 && n <= max }

func validateAnimScript(sc AnimScript) []string {
	var p []string
	add := func(format string, a ...any) { p = append(p, fmt.Sprintf(format, a...)) }
	if sc.Version != animScriptVersion {
		add("version必须为%s", animScriptVersion)
	}
	if sc.Archetype != "func.params@1" {
		add("archetype必须锁定版本，首版仅支持func.params@1")
	}
	if !animRunes(sc.Title, 24) {
		add("标题须为1–24字")
	}
	if len(sc.Scenes) < 1 || len(sc.Scenes) > 12 {
		add("幕数须为1–12")
	}
	sceneIDs, sentenceIDs := map[string]bool{}, map[string]bool{}
	var axesRange []float64
	representativeOK := false
	for _, s := range sc.Scenes {
		at := "幕" + s.ID
		if !animIDPattern.MatchString(s.ID) || sceneIDs[s.ID] {
			add("%s：幕ID无效或重复", at)
		}
		sceneIDs[s.ID] = true
		if !animRunes(s.Title, 20) || !animRunes(s.Intent, 40) {
			add("%s：标题须为1–20字，教学意图须为1–40字", at)
		}
		if len(s.Narration) < 1 || len(s.Narration) > 6 || len(s.Objects) > 8 || len(s.Actions) > 16 || len(s.Holds) > 6 {
			add("%s：每幕1–6句旁白、最多8个对象、16个动作、6个停顿", at)
		}
		sentences := map[string]bool{}
		for _, n := range s.Narration {
			if !animIDPattern.MatchString(n.ID) || sentenceIDs[n.ID] {
				add("%s：句子ID %q 无效或重复", at, n.ID)
			}
			sentenceIDs[n.ID], sentences[n.ID] = true, true
			if !animRunes(n.Display, 40) || !animRunes(n.Speech, 60) {
				add("%s：句子%s显示文本须为1–40字、朗读文本须为1–60字", at, n.ID)
			}
			if i := strings.IndexAny(n.Speech, animSpeechSymbols); i >= 0 {
				r, _ := utf8.DecodeRuneInString(n.Speech[i:])
				add("%s：句子%s朗读文本含符号%q，须转换为明确读法（如“等于”“负”）", at, n.ID, string(r))
			}
		}
		objects := map[string]AnimObject{}
		axesCount := 0
		for _, o := range s.Objects {
			oat := at + "对象" + o.ID
			if !animIDPattern.MatchString(o.ID) || objects[o.ID].ID != "" {
				add("%s：对象ID无效或重复", oat)
			}
			slot, known := animObjectSlots[o.Type]
			if !known {
				add("%s：不支持的组件类型%q", oat, o.Type)
				continue
			}
			if o.Slot != slot {
				add("%s：%s组件只能放在%s槽位", oat, o.Type, slot)
			}
			allowed := animParams[o.Type]
			for k := range o.Params {
				if !animContains(allowed, k) {
					add("%s：%s组件不接受参数%s", oat, o.Type, k)
				}
			}
			for _, k := range allowed {
				if v, ok := o.Params[k]; !ok || math.IsNaN(v) || math.IsInf(v, 0) {
					add("%s：缺少参数%s", oat, k)
				}
			}
			switch o.Type {
			case "axes":
				axesCount++
				r := []float64{o.Params["xmin"], o.Params["xmax"], o.Params["ymin"], o.Params["ymax"]}
				if r[1]-r[0] <= 0 || r[1]-r[0] > 40 || r[3]-r[2] <= 0 || r[3]-r[2] > 40 {
					add("%s：坐标范围须递增且跨度不超过40", oat)
				}
				if axesRange == nil {
					axesRange = r
				} else if fmt.Sprint(axesRange) != fmt.Sprint(r) {
					add("%s：func.params@1锁定坐标比例，各幕坐标范围须一致", oat)
				}
			case "line":
				if a, ok := objects[o.Axes]; !ok || a.Type != "axes" {
					add("%s：须引用本幕已声明的坐标系", oat)
				}
			case "point", "equation":
				if l, ok := objects[o.Line]; !ok || l.Type != "line" {
					add("%s：须引用本幕已声明的直线", oat)
				}
			}
			if o.Type == "text" && !animRunes(o.Text, 24) {
				add("%s：文字须为1–24字", oat)
			}
			if o.Type == "point" && o.Text != "" && !animRunes(o.Text, 10) {
				add("%s：标注须不超过10字", oat)
			}
			if o.Type == "panel" {
				if len(o.Items) < 1 || len(o.Items) > 4 {
					add("%s：对比面板须为1–4条", oat)
				}
				for _, it := range o.Items {
					if !animRunes(it, 16) {
						add("%s：面板每条须为1–16字", oat)
					}
				}
			}
			if o.Type != "text" && o.Type != "point" && o.Text != "" || o.Type != "panel" && len(o.Items) > 0 {
				add("%s：%s组件不接受text/items", oat, o.Type)
			}
			objects[o.ID] = o
		}
		if axesCount > 1 {
			add("%s：每幕最多一个坐标系", at)
		}
		lines := animLineStates(s)
		actions := map[string]bool{}
		for i, a := range s.Actions {
			aat := at + "动作" + a.ID
			if !animIDPattern.MatchString(a.ID) || actions[a.ID] {
				add("%s：动作ID无效或重复", aat)
			}
			o, ok := objects[a.Object]
			if !ok {
				add("%s：引用了不存在的对象%q", aat, a.Object)
			}
			if _, known := animDefaultMS[a.Op]; !known {
				add("%s：不支持的动作%q", aat, a.Op)
			}
			if a.Op == "tween" {
				if a.To == nil || !animContains(animTweenable[o.Type], a.Param) {
					add("%s：参数插值须给出该组件可变参数及目标值", aat)
				}
			} else if a.Param != "" || a.To != nil {
				add("%s：只有tween动作可带param/to", aat)
			}
			if a.DurationMS != 0 && (a.DurationMS < 200 || a.DurationMS > 8000) {
				add("%s：动作时长须为200–8000毫秒", aat)
			}
			t, set := a.At, 0
			for _, v := range []string{t.Sentence, t.After, t.Scene} {
				if v != "" {
					set++
				}
			}
			switch {
			case set != 1:
				add("%s：触发须且仅须指定sentence/after/scene之一", aat)
			case t.Sentence != "" && !sentences[t.Sentence]:
				add("%s：引用了本幕不存在的句子%q", aat, t.Sentence)
			case t.After != "" && !animEarlierAction(s.Actions[:i], t.After):
				add("%s：after须引用本幕更早的动作", aat)
			case t.Scene != "" && t.Scene != "start" && t.Scene != "end":
				add("%s：scene触发只能是start或end", aat)
			}
			if t.Anchor != "" && (t.Sentence == "" || t.Anchor != "begin" && t.Anchor != "end") || t.OffsetMS < 0 || t.OffsetMS > 5000 {
				add("%s：anchor须为begin/end，偏移须为0–5000毫秒", aat)
			}
			actions[a.ID] = true
		}
		for _, h := range s.Holds {
			if !animIDPattern.MatchString(h.ID) || !animHoldTypes[h.Type] || h.MS < 300 || h.MS > 10000 || h.After != "" && !sentences[h.After] {
				add("%s：停顿%s类型、时长（300–10000毫秒）或位置无效", at, h.ID)
			}
		}
		for _, l := range lines {
			p = append(p, animLineProblems(at, l, axesRange)...)
		}
		p = append(p, animNarrationProblems(at, s, lines)...)
		if s.ID == sc.Representative {
			for _, l := range lines {
				if len(l.tweens) > 0 {
					representativeOK = true
				}
			}
		}
	}
	if !representativeOK {
		add("代表幕须为已声明的幕，且包含直线参数的实际变化（不能用标题幕）")
	}
	return p
}

func animContains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
func animEarlierAction(earlier []AnimAction, id string) bool {
	for _, a := range earlier {
		if a.ID == id {
			return true
		}
	}
	return false
}

type animTween struct {
	param, action string
	from, to      float64
}
type animLine struct {
	id     string
	k, b   float64 // initial
	tweens []animTween
	states [][2]float64 // (k, b) at start and after each tween
	points []AnimObject
}

// animLineStates replays line tweens in array order; the timeline later
// rejects tweens whose time order differs from array order.
func animLineStates(s AnimScene) []*animLine {
	var lines []*animLine
	byID := map[string]*animLine{}
	for _, o := range s.Objects {
		if o.Type == "line" {
			l := &animLine{id: o.ID, k: o.Params["k"], b: o.Params["b"]}
			l.states = [][2]float64{{l.k, l.b}}
			lines, byID[o.ID] = append(lines, l), l
		}
	}
	for _, o := range s.Objects {
		if l := byID[o.Line]; o.Type == "point" && l != nil {
			l.points = append(l.points, o)
		}
	}
	for _, a := range s.Actions {
		l := byID[a.Object]
		if a.Op != "tween" || l == nil || a.To == nil || (a.Param != "k" && a.Param != "b") {
			continue
		}
		cur := l.states[len(l.states)-1]
		next, i := cur, map[string]int{"k": 0, "b": 1}[a.Param]
		next[i] = *a.To
		l.tweens = append(l.tweens, animTween{a.Param, a.ID, cur[i], *a.To})
		l.states = append(l.states, next)
	}
	return lines
}

func animLineProblems(at string, l *animLine, axes []float64) []string {
	var p []string
	for _, st := range l.states {
		k, b := st[0], st[1]
		if k == 0 {
			p = append(p, fmt.Sprintf("%s直线%s：k=0时不是一次函数", at, l.id))
		}
		if math.Abs(k) > 10 || math.Abs(b) > 20 {
			p = append(p, fmt.Sprintf("%s直线%s：k须在[-10,10]、b须在[-20,20]内", at, l.id))
		}
		if len(axes) == 4 {
			y1, y2 := k*axes[0]+b, k*axes[1]+b
			if math.Max(y1, y2) < axes[2] || math.Min(y1, y2) > axes[3] {
				p = append(p, fmt.Sprintf("%s直线%s：k=%g、b=%g时直线不在坐标范围内", at, l.id, k, b))
			}
		}
		for _, pt := range l.points {
			x := pt.Params["x"]
			if len(axes) == 4 && (x < axes[0] || x > axes[1]) {
				p = append(p, fmt.Sprintf("%s点%s：x超出坐标范围", at, pt.ID))
			}
			// A numeric label must match the computed point for every state the line passes through.
			if m := animPointLabel.FindStringSubmatch(pt.Text); m != nil {
				lx, _ := strconv.ParseFloat(m[1], 64)
				ly, _ := strconv.ParseFloat(m[2], 64)
				if !animNear(lx, x) || !animNear(ly, k*x+b) {
					p = append(p, fmt.Sprintf("%s点%s：标注%s与直线上的实际点(%g, %g)不一致", at, pt.ID, pt.Text, x, k*x+b))
				}
			}
		}
	}
	return p
}

func animNear(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// animNarrationProblems checks narration claims against the actual parameters.
// Direction follows the sign of k; steepness follows |k| only, regardless of sign.
func animNarrationProblems(at string, s AnimScene, lines []*animLine) []string {
	var p []string
	var kt []animTween
	for _, l := range lines {
		for _, t := range l.tweens {
			if t.param == "k" {
				kt = append(kt, t)
			}
		}
	}
	all := ""
	for _, n := range s.Narration {
		all += n.Display + n.Speech
	}
	for _, t := range kt {
		if t.from*t.to < 0 && !strings.Contains(all, "不是一次函数") {
			p = append(p, fmt.Sprintf("%s动作%s：k从%g连续变到%g会经过k=0，须在旁白中说明此时不是一次函数，或拆成两段演示", at, t.action, t.from, t.to))
		}
	}
	for _, n := range s.Narration {
		text, nat := n.Display+" "+n.Speech, at+"句子"+n.ID
		question := strings.ContainsAny(text, "？?")
		steeper := animHasAny(text, "变陡", "更陡", "越陡", "陡峭")
		flatter := animHasAny(text, "变平", "更平", "变缓", "更缓", "平缓")
		if !question && (steeper || flatter) && steeper != flatter {
			if len(kt) > 0 {
				for _, t := range kt {
					if steeper && !(math.Abs(t.to) > math.Abs(t.from)) || flatter && !(math.Abs(t.to) < math.Abs(t.from)) {
						word := map[bool]string{true: "变陡要求|k|增大", false: "变平要求|k|减小"}[steeper]
						p = append(p, fmt.Sprintf("%s：%s，但动作%s中k从%g到%g（|k|从%g到%g）", nat, word, t.action, t.from, t.to, math.Abs(t.from), math.Abs(t.to)))
					}
				}
			} else if m := animKValue.FindAllStringSubmatch(n.Display, -1); len(m) == 2 && strings.Contains(n.Display, "比") {
				a, _ := strconv.ParseFloat(m[0][1], 64)
				b, _ := strconv.ParseFloat(m[1][1], 64)
				if steeper && !(math.Abs(a) > math.Abs(b)) || flatter && !(math.Abs(a) < math.Abs(b)) {
					p = append(p, fmt.Sprintf("%s：陡缓比较只看|k|，k=%g与k=%g的说法与|k|不符", nat, a, b))
				}
			}
		}
		// Display and speech are checked separately so a contradiction between them cannot cancel out.
		for _, field := range []string{n.Display, n.Speech} {
			up, down := strings.Contains(field, "上升"), strings.Contains(field, "下降")
			if question || up == down {
				continue
			}
			for _, l := range lines {
				for _, st := range l.states {
					if up && st[0] <= 0 || down && st[0] >= 0 {
						p = append(p, fmt.Sprintf("%s：旁白称直线%s，但直线%s出现k=%g", nat, map[bool]string{true: "上升", false: "下降"}[up], l.id, st[0]))
						break
					}
				}
			}
		}
		for _, m := range animKFromTo.FindAllStringSubmatch(n.Display, -1) {
			from, _ := strconv.ParseFloat(m[1], 64)
			to, _ := strconv.ParseFloat(m[2], 64)
			found := false
			for _, t := range kt {
				found = found || animNear(t.from, from) && animNear(t.to, to)
			}
			if !found {
				p = append(p, fmt.Sprintf("%s：旁白说k从%g到%g，本幕没有对应的参数变化", nat, from, to))
			}
		}
		for _, m := range animLineFormula.FindAllStringSubmatch(n.Display, -1) {
			k, b := animFormulaCoefficients(m)
			found := false
			for _, l := range lines {
				for _, st := range l.states {
					found = found || animNear(st[0], k) && animNear(st[1], b)
				}
			}
			if !found {
				p = append(p, fmt.Sprintf("%s：旁白中的%s与本幕直线参数不一致", nat, strings.TrimSpace(m[0])))
			}
		}
	}
	return p
}

func animHasAny(s string, words ...string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

func animFormulaCoefficients(m []string) (k, b float64) {
	switch m[1] {
	case "":
		k = 1
	case "-":
		k = -1
	default:
		k, _ = strconv.ParseFloat(m[1], 64)
	}
	if m[3] != "" {
		b, _ = strconv.ParseFloat(m[3], 64)
		if m[2] == "-" {
			b = -b
		}
	}
	return
}
