package main

import (
	"bytes"
	"encoding/json"
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
}
type DrawingAction struct {
	Object     string   `json:"object"`
	Op         string   `json:"op"`
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
	failScene := func() error {
		return fail(400, "通用图示需1至24个受控对象、0至48个有效动作，时长1至8秒；检查对象引用、路径、范围与动作重叠")
	}
	scene := d.Scene
	if scene == nil || d.Animation != nil || scene.DurationMS < 1000 || scene.DurationMS > 8000 || len(scene.Objects) < 1 || len(scene.Objects) > 24 || len(scene.Actions) > 48 {
		return failScene()
	}
	finite := func(v float64, lo, hi float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= lo && v <= hi }
	objects := map[string]DrawingObject{}
	for _, o := range scene.Objects {
		if !drawingID.MatchString(o.ID) || objects[o.ID].ID != "" {
			return fail(400, "通用图示对象"+o.ID+"的id必须唯一，仅用小写字母、数字、下划线或短横线，以小写字母开头，不能有大写字母")
		}
		if !animContains([]string{"rect", "ellipse", "text", "path", "polygon", "arrow"}, o.Type) || !finite(o.X, 0, 1) || !finite(o.Y, 0, 1) || o.Color != "" && !drawingColor.MatchString(o.Color) || o.Fill != "" && !drawingColor.MatchString(o.Fill) || o.FontSize != 0 && (o.FontSize < 22 || o.FontSize > 60) || o.StrokeWidth != 0 && !finite(o.StrokeWidth, 1, 8) || len([]rune(o.Text)) > 60 {
			return failScene()
		}
		path := animContains([]string{"path", "polygon", "arrow"}, o.Type)
		if path {
			min := 2
			if o.Type == "polygon" {
				min = 3
			}
			if len(o.Points) < min || len(o.Points) > 32 {
				return failScene()
			}
			length := 0.
			for i, p := range o.Points {
				if !finite(p[0], 0, 1) || !finite(p[1], 0, 1) {
					return failScene()
				}
				if i > 0 {
					length += math.Hypot(p[0]-o.Points[i-1][0], p[1]-o.Points[i-1][1])
				}
			}
			if length < .001 {
				return failScene()
			}
		} else if len(o.Points) > 0 {
			return failScene()
		}
		if o.Width != 0 && !finite(o.Width, .005, 1) || o.Height != 0 && !finite(o.Height, .005, 1) {
			return failScene()
		}
		if o.Type == "text" && (strings.TrimSpace(o.Text) == "" || strings.Contains(o.Text, "\n")) {
			return failScene()
		}
		objects[o.ID] = o
	}
	routes := map[string]bool{}
	for _, a := range scene.Actions {
		if a.Op == "follow_path" {
			routes[a.Path] = true
		}
	}
	tracks := map[string]int{}
	for _, a := range scene.Actions {
		if routes[a.Object] && animContains([]string{"move", "follow_path", "rotate", "scale"}, a.Op) {
			return failScene()
		}
		if objects[a.Object].ID == "" || !animContains([]string{"move", "follow_path", "rotate", "scale", "show", "hide", "emphasize"}, a.Op) || a.StartMS < 0 || a.DurationMS < 100 || a.StartMS+a.DurationMS > scene.DurationMS {
			return failScene()
		}
		if a.Op == "move" {
			if a.X == nil || a.Y == nil || !finite(*a.X, 0, 1) || !finite(*a.Y, 0, 1) {
				return failScene()
			}
		} else if a.X != nil || a.Y != nil {
			return failScene()
		}
		if a.Op == "rotate" || a.Op == "scale" {
			lo, hi := -360., 360.
			if a.Op == "scale" {
				lo, hi = .2, 3
			}
			if a.To == nil || !finite(*a.To, lo, hi) {
				return failScene()
			}
		} else if a.To != nil {
			return failScene()
		}
		if a.Op == "follow_path" {
			if objects[a.Path].Type != "path" || a.Path == a.Object {
				return failScene()
			}
		} else if a.Path != "" {
			return failScene()
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
			return failScene()
		}
		tracks[key] = a.StartMS + a.DurationMS
	}
	return nil
}

const generalSceneContract = `
通用图形动画工具：scientific.kind="scene"，用结构化数据组合图形、路径、标注和动作，由程序绘制、自动生成循环GIF并嵌入PPT；无动作则静态图。适用于已有专业工具以外的概念示意、实验步骤、空间关系和过程变化；先选真实教学任务，不强制每页动图。涉及透镜、分子和细胞时优先相应专业工具。通用图示按描述呈现，不是物理仿真或学科正确性证明，不把移动圆点称为真实微观过程。
格式示例：{kind:"scene",title:"沿路径移动（示意）",scene:{duration_ms:3000,objects:[{id:"route",type:"path",points:[[0.2,0.5],[0.4,0.3],[0.6,0.5]],color:"#60758a"},{id:"particle",type:"ellipse",x:0.2,y:0.5,width:0.04,height:0.06,fill:"#bc582b",color:"#bc582b"},{id:"label",type:"text",x:0.5,y:0.82,text:"观察路径与位置",font_size:28}],actions:[{object:"particle",op:"follow_path",path:"route",start_ms:0,duration_ms:2500},{object:"label",op:"show",start_ms:1500,duration_ms:500}]}}。
objects为1–24个，id是字母开头的小写字母/数字/短横线/下划线，最多32字符。type仅rect/ellipse/text/path/polygon/arrow。x/y是对象中心，横向向右、纵向向下，0–1归一化绘图区坐标，非像素；width/height为画布宽高比例0.005–1。path/polygon/arrow使用2–32个points坐标对，polygon至少3点，箭头指向路径末端；路径中心由程序计算。color和fill仅#RRGGBB，fill省略表示不填充；stroke_width为1–8。文字单行最多60字、font_size为22–60，默认26。正文、标注和整段动作都要留足边距，不能超出绘图区；长文字用多个短标注，不缩成看不清的小字。
actions为0–48个；start_ms>=0，duration_ms>=100，动作结束不得超过scene.duration_ms（1000–8000）。op仅move（x/y目标中心）、follow_path（path引用同页type:path对象，沿折线路径匀弧长再缓动）、rotate（to目标角度，顺时针为正，-360至360）、scale（to目标缩放0.2–3）、show/hide/emphasize。旋转缩放围绕对象中心；同一对象同一属性动作按时间声明且不重叠，移动与沿路径属于同一位置属性。被follow_path引用的路径须保持几何不变，不可移动、旋转、缩放或沿另一路径运动，可出现/消失/强调。可同屏组合多个图形、路径和标注，不得输出代码、脚本、SVG/HTML、URL或未知对象类型。绘图工具会拒绝越界和不支持动作，不能偷偷忽略失败。
视频用scientific组件复用同一个scene，其params:{progress:0}，tween progress从0到1驱动scene内整段时间轴；动作时长通常与scene.duration_ms相同。此时不再另填scientific.animation。每幕可以独占绘图区承载多个scene内部对象，仍可使用已有旁白、停顿和教师确认流程。
`
