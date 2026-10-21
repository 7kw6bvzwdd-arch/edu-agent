package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
)

type DrawingScene struct {
	DurationMS int             `json:"duration_ms"`
	Objects    []DrawingObject `json:"objects"`
	Actions    []DrawingAction `json:"actions,omitempty"`
}

// Every entry point decodes the same bounded schema, including manual edits.
// Unknown fields must fail before Go can discard them while forwarding to Python.
func (s *DrawingScene) UnmarshalJSON(data []byte) error {
	type sceneData DrawingScene
	var value sceneData
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	*s = DrawingScene(value)
	return nil
}

type DrawingObject struct {
	ID          string       `json:"id"`
	Type        string       `json:"type"`
	Allotrope   string       `json:"allotrope,omitempty"`
	X           float64      `json:"x"`
	Y           float64      `json:"y"`
	Width       float64      `json:"width,omitempty"`
	Height      float64      `json:"height,omitempty"`
	Color       string       `json:"color,omitempty"`
	Fill        string       `json:"fill,omitempty"`
	Text        string       `json:"text,omitempty"`
	FontSize    int          `json:"font_size,omitempty"`
	StrokeWidth float64      `json:"stroke_width,omitempty"`
	Points      [][2]float64 `json:"points,omitempty"`
	AttachTo    string       `json:"attach_to,omitempty"`
}
type DrawingAction struct {
	Object     string   `json:"object"`
	Op         string   `json:"op"`
	Sentence   string   `json:"sentence,omitempty"`
	StartMS    int      `json:"start_ms"`
	DurationMS int      `json:"duration_ms"`
	X          *float64 `json:"x,omitempty"`
	Y          *float64 `json:"y,omitempty"`
	To         *float64 `json:"to,omitempty"`
	Path       string   `json:"path,omitempty"`
}

var drawingColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var drawingID = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

func validateDrawingScene(d *ScientificDiagram) error {
	scene := d.Scene
	if scene == nil {
		return fail(400, "scientific.kind=scene必须带scene:{duration_ms,objects,actions}")
	}
	if d.Animation != nil {
		return fail(400, "scientific.kind=scene禁止scientific.animation，请删除；仅用scene.actions定义内部动作，外层tween progress驱动")
	}
	if scene.DurationMS < 1000 || scene.DurationMS > 8000 {
		return fail(400, fmt.Sprintf("scene.duration_ms=%d，须1000至8000；不要把整幕旁白时长填成内部图形时长", scene.DurationMS))
	}
	if len(scene.Objects) < 1 || len(scene.Objects) > 24 || len(scene.Actions) > 48 {
		return fail(400, fmt.Sprintf("scene.objects数量%d须1至24，scene.actions数量%d须0至48", len(scene.Objects), len(scene.Actions)))
	}
	finite := func(v float64, lo, hi float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= lo && v <= hi }
	objects := map[string]DrawingObject{}
	for _, o := range scene.Objects {
		if !drawingID.MatchString(o.ID) || objects[o.ID].ID != "" {
			return fail(400, "通用图示对象"+o.ID+"的id必须唯一，仅用小写字母、数字、下划线或短横线，以小写字母开头，不能有大写字母")
		}
		if !animContains([]string{"rect", "ellipse", "text", "path", "polygon", "arrow", "carbon"}, o.Type) {
			return fail(400, "对象"+o.ID+" type仅rect/ellipse/text/path/polygon/arrow/carbon")
		}
		if o.Type == "carbon" && !animContains([]string{"diamond", "graphite", "fullerene"}, o.Allotrope) || o.Type != "carbon" && o.Allotrope != "" {
			return fail(400, "仅carbon需要allotrope=diamond/graphite/fullerene")
		}
		if !finite(o.X, 0, 1) || !finite(o.Y, 0, 1) {
			return fail(400, "对象"+o.ID+" x/y须为0至1归一化中心坐标")
		}
		if o.Color != "" && !drawingColor.MatchString(o.Color) || o.Fill != "" && !drawingColor.MatchString(o.Fill) {
			return fail(400, "对象"+o.ID+" color/fill仅#RRGGBB，省略fill表示不填充")
		}
		if o.FontSize != 0 && (o.FontSize < 22 || o.FontSize > 60) {
			return fail(400, fmt.Sprintf("对象%s font_size=%d，须22至60；不要缩小文字，用短标注", o.ID, o.FontSize))
		}
		if o.StrokeWidth != 0 && !finite(o.StrokeWidth, 1, 8) {
			return fail(400, "对象"+o.ID+" stroke_width须1至8")
		}
		if len([]rune(o.Text)) > 60 {
			return fail(400, "对象"+o.ID+" text最多60字")
		}
		path := animContains([]string{"path", "polygon", "arrow"}, o.Type)
		if path {
			min := 2
			if o.Type == "polygon" {
				min = 3
			}
			if len(o.Points) < min || len(o.Points) > 32 {
				return fail(400, fmt.Sprintf("对象%s points须%d至32个归一化坐标对", o.ID, min))
			}
			length := 0.
			for i, p := range o.Points {
				if !finite(p[0], 0, 1) || !finite(p[1], 0, 1) {
					return fail(400, fmt.Sprintf("对象%s points[%d]坐标须0至1", o.ID, i))
				}
				if i > 0 {
					length += math.Hypot(p[0]-o.Points[i-1][0], p[1]-o.Points[i-1][1])
				}
			}
			if length < .001 {
				return fail(400, "对象"+o.ID+" points不能全部重合")
			}
		} else if len(o.Points) > 0 {
			return fail(400, "对象"+o.ID+"仅path/polygon/arrow允许points")
		}
		if o.Width != 0 && !finite(o.Width, .005, 1) || o.Height != 0 && !finite(o.Height, .005, 1) {
			return fail(400, "对象"+o.ID+" width/height须0.005至1，不能使用像素")
		}
		if o.Type == "text" && (strings.TrimSpace(o.Text) == "" || strings.Contains(o.Text, "\n")) {
			return fail(400, "文字对象"+o.ID+" text须非空单行")
		}
		objects[o.ID] = o
	}
	routes := map[string]bool{}
	positions := map[string][2]float64{}
	for _, o := range scene.Objects {
		if o.AttachTo != "" && (o.Type != "text" || objects[o.AttachTo].ID == "" || objects[o.AttachTo].Type == "text" || objects[o.AttachTo].AttachTo != "") {
			return fail(400, "对象"+o.ID+"的attach_to须引用本图非文字且未附着的图形")
		}
		x, y := o.X, o.Y
		if len(o.Points) > 0 {
			x, y = 0, 0
			for _, p := range o.Points {
				x += p[0] / float64(len(o.Points))
				y += p[1] / float64(len(o.Points))
			}
		}
		positions[o.ID] = [2]float64{x, y}
	}
	for _, a := range scene.Actions {
		if objects[a.Object].AttachTo != "" && animContains([]string{"move", "follow_path", "rotate", "scale"}, a.Op) {
			return fail(400, "标注"+a.Object+"已通过attach_to跟随图形，不能再设置变换动作")
		}
		if a.Op == "follow_path" {
			routes[a.Path] = true
		}
	}
	tracks := map[string]int{}
	for _, a := range scene.Actions {
		if routes[a.Object] && animContains([]string{"move", "follow_path", "rotate", "scale"}, a.Op) {
			return fail(400, "路径对象"+a.Object+"被follow_path引用，不可再移动旋转缩放")
		}
		if objects[a.Object].ID == "" || !animContains([]string{"move", "follow_path", "rotate", "scale", "show", "hide", "emphasize", "reveal", "slide"}, a.Op) || a.StartMS < 0 || a.DurationMS < 100 || a.StartMS+a.DurationMS > scene.DurationMS {
			return fail(400, fmt.Sprintf("动作object=%s op=%s start_ms=%d duration_ms=%d：须引用有效对象及动作类型，start_ms>=0、duration_ms>=100、结束<=scene.duration_ms=%d", a.Object, a.Op, a.StartMS, a.DurationMS, scene.DurationMS))
		}
		if a.Op == "move" {
			if a.X == nil || a.Y == nil || !finite(*a.X, 0, 1) || !finite(*a.Y, 0, 1) {
				return fail(400, "动作"+a.Object+" move必须同时给x/y目标中心，范围0至1")
			}
		} else if a.X != nil || a.Y != nil {
			return fail(400, "动作"+a.Object+"仅move允许x/y，其他动作请删除x/y")
		}
		if a.Sentence != "" && !animIDPattern.MatchString(a.Sentence) {
			return fail(400, "内部动作sentence须为有效句子ID")
		}
		if (a.Op == "reveal" || a.Op == "slide") && (objects[a.Object].Type != "carbon" || a.Op == "slide" && objects[a.Object].Allotrope != "graphite") {
			return fail(400, "reveal仅用于carbon，slide仅用于graphite")
		}
		if animContains([]string{"rotate", "scale", "reveal", "slide"}, a.Op) {
			lo, hi := -360., 360.
			if a.Op == "scale" {
				lo, hi = .2, 3
			}
			if a.Op == "reveal" || a.Op == "slide" {
				lo, hi = 0, 1
			}
			if a.To == nil || !finite(*a.To, lo, hi) {
				return fail(400, fmt.Sprintf("动作%s %s须to目标值，范围%.1f至%.1f", a.Object, a.Op, lo, hi))
			}
		} else if a.To != nil {
			return fail(400, "动作"+a.Object+"仅rotate/scale/reveal/slide允许to")
		}
		if a.Op == "follow_path" {
			if objects[a.Path].Type != "path" || a.Path == a.Object {
				return fail(400, "动作"+a.Object+"的path必须引用另一个type=path对象")
			}
			points := objects[a.Path].Points
			start := positions[a.Object]
			if math.Hypot(start[0]-points[0][0], start[1]-points[0][1]) > .002 {
				return fail(400, fmt.Sprintf("对象%s当前位置(%.3f,%.3f)必须等于路径%s首点(%.3f,%.3f)；返向运动须反转points另建路径，初始位置不能改变路径方向", a.Object, start[0], start[1], a.Path, points[0][0], points[0][1]))
			}
			positions[a.Object] = points[len(points)-1]
		} else if a.Path != "" {
			return fail(400, "动作"+a.Object+"仅follow_path允许path字段")
		}
		if a.Op == "move" {
			positions[a.Object] = [2]float64{*a.X, *a.Y}
		}
		track := a.Op
		if track == "move" || track == "follow_path" {
			track = "position"
		}
		if track == "show" || track == "hide" {
			track = "opacity"
		}
		key := a.Object + "/" + track
		if a.StartMS < tracks[key] {
			return fail(400, fmt.Sprintf("动作%s %s起点%d小于前一同属性动作结束%d，请按时间声明且不要重叠", a.Object, track, a.StartMS, tracks[key]))
		}
		tracks[key] = a.StartMS + a.DurationMS
	}
	return nil
}

const generalSceneContract = `
通用图形动画工具：scientific.kind="scene"，用结构化数据组合图形、路径、标注和动作，由程序绘制、自动生成循环GIF并嵌入PPT；无动作则静态图。适用于已有专业工具以外的概念示意、实验步骤、空间关系和过程变化；先选真实教学任务，不强制每页动图。涉及透镜、分子和细胞时优先相应专业工具。通用图示按描述呈现，不是物理仿真或学科正确性证明，不把移动圆点称为真实微观过程。
格式示例：{kind:"scene",title:"沿路径移动（示意）",scene:{duration_ms:3000,objects:[{id:"route",type:"path",points:[[0.2,0.5],[0.4,0.3],[0.6,0.5]],color:"#60758a"},{id:"particle",type:"ellipse",x:0.2,y:0.5,width:0.04,height:0.06,fill:"#bc582b",color:"#bc582b"},{id:"label",type:"text",x:0.5,y:0.82,text:"观察路径与位置",font_size:28}],actions:[{object:"particle",op:"follow_path",path:"route",start_ms:0,duration_ms:2500},{object:"label",op:"show",start_ms:1500,duration_ms:500}]}}。
objects为1–24个，id是字母开头的小写字母/数字/短横线/下划线，最多32字符。type仅rect/ellipse/text/path/polygon/arrow/carbon。carbon必须加allotrope=diamond/graphite/fullerene，连接结构由程序提供；多个模型可在同一scene比较。x/y是对象中心，横向向右、纵向向下，0–1归一化绘图区坐标，非像素；width/height为画布宽高比例0.005–1。path/polygon/arrow使用2–32个points坐标对，polygon至少3点，箭头指向路径末端；路径中心由程序计算。color和fill仅#RRGGBB，fill省略表示不填充；stroke_width为1–8。文字单行最多60字、font_size为22–60，默认26。正文、标注和整段动作都要留足边距，不能超出绘图区；长文字用多个短标注，不缩成看不清的小字。
actions为0–48个；start_ms>=0，duration_ms>=100，动作结束不得超过scene.duration_ms（1000–8000）。op仅move（x/y目标中心）、follow_path（path引用同页type:path对象，沿折线路径匀弧长再缓动）、rotate（to目标角度，顺时针为正，-360至360）、scale（to目标缩放0.2–3）、show/hide/emphasize、reveal（仅carbon，见下文）、slide（仅graphite，见下文）。旋转缩放围绕对象中心；同一对象同一属性动作按时间声明且不重叠，移动与沿路径属于同一位置属性。被follow_path引用的路径须保持几何不变，不可移动、旋转、缩放或沿另一路径运动，可出现/消失/强调。可同屏组合多个图形、路径和标注，不得输出代码、脚本、SVG/HTML、URL或未知对象类型。绘图工具会拒绝越界和不支持动作，不能偷偷忽略失败。
视频用scientific组件复用同一个scene，其params:{progress:0}，tween progress从0到1驱动scene内整段时间轴；动作时长通常与scene.duration_ms相同。此时不再另填scientific.animation。每幕可以独占绘图区承载多个scene内部对象，仍可使用已有旁白、停顿和教师确认流程。
新增内部reveal（仅carbon，to=0至1，初始不可见）和slide（仅graphite，to=0至1，初始0），也可rotate观察模型。视频内部动作可加sentence=本幕旁白ID：从该句真实语音起点开始，时长取duration_ms与句长的较小者；start_ms仅供独立GIF预览回退，不叠加到句子开始。没有sentence时由外层progress驱动。内层不能写at/param；同一对象同属性不要混合多个句子绑定和progress排程。
移动名称标注可给type:text对象增加attach_to，值为目标图形ID，仅允许引用本图非文字且未附着图形；x/y为标注初始中心，程序继承目标平移和显隐，标注自身不能再move/follow_path/rotate/scale。未设置attach_to的文字为固定说明。follow_path必须从该对象当前位置开始；反向须另建points反序的path，不允许瞬移。请将往返物体错开时间或分轨，避免重叠遮挡。
通用视频完整幕示例（严格照此嵌套，按教学需求改内容）：{"id":"s1","title":"观察位置","intent":"观察球的位置变化","objects":[{"id":"visual","type":"scientific","slot":"main","params":{"progress":0},"scientific":{"kind":"scene","title":"物体位置示意","scene":{"duration_ms":3000,"objects":[{"id":"table","type":"rect","x":0.5,"y":0.5,"width":0.6,"height":0.04},{"id":"ball","type":"ellipse","x":0.2,"y":0.2,"width":0.06,"height":0.12,"fill":"#bc582b"}],"actions":[{"object":"ball","op":"move","x":0.5,"y":0.7,"start_ms":0,"duration_ms":3000}]}}}],"narration":[{"id":"s1n1","display":"观察球移动到桌面的下方。","speech":"观察球移动到桌面的下方。"}],"actions":[{"id":"change","op":"tween","object":"visual","param":"progress","to":1,"at":{"sentence":"s1n1","anchor":"begin"},"duration_ms":3000}],"holds":[{"id":"observe","type":"observe","after":"s1n1","ms":1500}]}。内部move只有x/y，不能带to/param/at。任何层都不要给通用scene加animation字段。

`
