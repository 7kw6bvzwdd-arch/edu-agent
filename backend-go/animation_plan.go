package main

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// Scene data is interpreted by a trusted renderer, never executed as model-written code.
type AnimationScene struct {
	Goal      string             `json:"goal"`
	Duration  float64            `json:"duration_sec"`
	Caption   string             `json:"caption"`
	Formula   string             `json:"formula"`  // Plain Unicode; rendered as text, not executed LaTeX.
	Evidence  string             `json:"evidence"` // illustration, numeric, proof
	SourceIDs []string           `json:"source_ids"`
	Elements  []AnimationElement `json:"elements"`
	Checks    []GeometryCheck    `json:"checks"`
}
type AnimationElement struct {
	ID     string       `json:"id"`
	Kind   string       `json:"kind"` // polygon, line, arrow, circle, plot
	Label  string       `json:"label"`
	Points [][2]float64 `json:"points"`
	Radius float64      `json:"radius,omitempty"`
	Size   [2]float64   `json:"size,omitempty"`
	Angle  float64      `json:"angle,omitempty"`
	Move   [2]float64   `json:"move"`
	Color  string       `json:"color"` // blue, green, orange, purple
}

func (el *AnimationElement) UnmarshalJSON(data []byte) error {
	type plain AnimationElement
	var dimensions struct {
		Points [][]float64 `json:"points"`
		Move   []float64   `json:"move"`
		Size   []float64   `json:"size"`
	}
	if err := json.Unmarshal(data, &dimensions); err != nil {
		return err
	}
	for _, p := range dimensions.Points {
		if len(p) != 2 {
			return fmt.Errorf("坐标必须恰好包含x,y两个数")
		}
	}
	if dimensions.Move != nil && len(dimensions.Move) != 2 {
		return fmt.Errorf("移动向量必须恰好包含两个数")
	}
	if dimensions.Size != nil && len(dimensions.Size) != 2 {
		return fmt.Errorf("尺寸必须恰好包含两个数")
	}
	return json.Unmarshal(data, (*plain)(el))
}

type GeometryCheck struct {
	Kind    string   `json:"kind"` // right_angle, equal_length, equal_area, partition
	Objects []string `json:"objects"`
}
type AnimationQuality struct {
	Frames     []QualityFrame `json:"frames,omitempty"`
	Version    string         `json:"version"`
	Status     string         `json:"status"`
	Checks     []string       `json:"checks"`
	Visual     any            `json:"visual"`
	AcceptedBy int64          `json:"accepted_by,omitempty"`
}

const animationSceneContract = `
动画必须包含可执行分镜数据：每个slides项新增scene对象，字段：
goal(一个认知目标，最多60字)，duration_sec(秒)，caption(屏幕字幕，最多40字)，formula(Unicode公式，最多40字，不用LaTex；无则空串)，evidence(illustration/numeric/proof)，source_ids(实际来源ID数组，无则[])，elements，checks。
elements为1–6个几何对象：{id,kind,label,points,move:[dx,dy],color,radius}。
kind仅right_triangle/rectangle/polygon/line/arrow/circle/plot。优先使用right_triangle：points只有直角顶点，size:[直角边a,直角边b]均为正数，angle为绕该点旋转角度（度，默认0），程序计算另外两点，保证直角。rectangle：points一个左下角，size:[宽,高]为正数，angle同理，程序计算四角。不要手算旋转后的直角顶点。例如{id:"t",kind:"right_triangle",label:"直角三角形",points:[[-2,-1]],size:[2,2],angle:0,move:[0,0],color:"blue"}。polygon仅用于其他凸多边形，是按顺序给出的3–8个顶点；line/arrow恰好2点；circle一个圆心及正radius；plot为2–16个折线点。
points始终是嵌套的二维点数组（两层方括号），每个点是[x,y]两个有限数字。单锚点也必须写points:[[-1,-1]]，不能写points:[-1,-1]；箭头示例points:[[0,-1],[0,1]]，不能写扁平的四个数字。move:[dx,dy]、size:[宽,高]各只有一层方括号，不写第三个数、不用null或公式字符串。演示区x∈[-3.7,3.7]、y∈[-1.8,1.8]；移动后仍须在区内。move只做刚性平移，不改变尺寸与形状。标签label最多16字，统一放右侧图例，不手工定位文字。color只选blue/green/orange/purple。不支持任意脚本、外部资源和自定义相机。
checks为几何约束列表：{kind,objects:[对象ID,...]}。right_angle检查三角形存在直角；equal_length检查两条线长度相等；equal_area检查两个凸多边形面积相等；partition的第一个对象是目标凸多边形，其余为分割块，检查移动后全部在目标内、互不重叠、面积和等于目标。涉及拼图证明必须提供partition，不能把重叠或仅写“面积相等”当成证明。约束在移动后检查。非几何推理用illustration或numeric并保留人工核验，不虚构已证明。
evidence按每一幕的实际画面分别选择，不能因为课题叫“定理证明”就把所有幕标为proof。导入、构造、观察、代数变形、公式推导和总结使用illustration，在notes写明推理待教师核验；具体数值代入用numeric，不能声称已证明一般结论。只有本幕实际呈现完整分块覆盖、且checks含有效partition时才用proof；即使上一幕通过拼图校验，下一幕的代数推导也不是程序已验证的证明。不得为绕过校验简单改标签、删除失败约束或宣称结论已验证；教学目标明确要求拼图证明时必须保留完整拼图及约束。
相邻幕逐步呈现一个推理链，每幕仅一个目标；渲染器固定标题/图形/图例/字幕/公式区域，独立场景淡入淡出，不依赖上一幕遗留对象。公式只在教师可见的scene.formula中出现并与来源一致。
每幕必须自成完整画面：讲受力时同时包含受力物体和必要液面/容器，不只画一个孤立箭头；讲函数时同时画坐标轴和曲线；光学保留镜面、法线与光线。跨幕需要的背景对象须重复声明。图形不应过小，主要图形组合宽度建议3–6、高度2–3。不要把箭头长度当作无单位的真实测量。
精确动画每幕2–6秒，总20–45秒；情境视频scene是连续动作节点，总时长等于配置时长，画面只展示首幕caption/formula作为静态程序标注，因此首幕必须概括唯一核心问题。情境视频不使用proof。
`

func validateAnimationPlan(d Deck, sources []Source, seconds float64, cloud bool) error {
	if len(d.Slides) < 8 || len(d.Slides) > 12 {
		return fail(422, "动画需要8–12幕")
	}
	known := map[string]bool{}
	for _, s := range sources {
		known[s.ID] = true
	}
	for _, ref := range citation.FindAllString(d.Lesson+string(asJSON(d.Slides)), -1) {
		if !known[strings.Trim(ref, "[]")] {
			return fail(422, "动画内容引用不存在的来源ID")
		}
	}
	total := 0.0
	issues := []string{}
	for i, sl := range d.Slides {
		if sl.Scene != nil {
			total += sl.Scene.Duration
		}
		if err := validateAnimationScene(sl, i, known, cloud); err != nil {
			issues = append(issues, err.Error())
		}
	}
	if cloud {
		if math.Abs(total-seconds) > .05 {
			issues = append(issues, "动作节点总时长必须等于云视频配置时长")
		}
	} else if total < 20 || total > 45 {
		issues = append(issues, "精确动画总时长必须为20–45秒")
	}
	if len(issues) > 0 {
		return fail(422, strings.Join(issues, "；\n"))
	}
	return nil
}

// Report each scene to the single repair attempt, not only the earliest failing scene.
func validateAnimationScene(sl Slide, i int, known map[string]bool, cloud bool) error {
	bad := func(reason string) error { return fail(422, fmt.Sprintf("第%d幕：%s", i+1, reason)) }
	sc := sl.Scene
	if sc == nil {
		return bad("缺少结构化分镜，请重新研究或补齐分镜；旧脚本不能直接进入新版渲染")
	}
	for _, value := range []string{sl.Title, sc.Goal, sc.Caption, sc.Formula} {
		if strings.ContainsAny(value, "\r\n\t") {
			return bad("屏幕文字不能包含手工换行或制表符，由程序自动排版")
		}
	}
	if len([]rune(sl.Title)) > 28 || strings.TrimSpace(sc.Goal) == "" || len([]rune(sc.Goal)) > 60 || strings.TrimSpace(sc.Caption) == "" || len([]rune(sc.Caption)) > 40 || len([]rune(sc.Formula)) > 40 {
		return bad("标题/目标/字幕/公式超过可读限制或为空")
	}
	if math.IsNaN(sc.Duration) || math.IsInf(sc.Duration, 0) || sc.Duration <= 0 || (!cloud && (sc.Duration < 2 || sc.Duration > 6)) {
		return bad("每幕时长无效；精确动画要求2–6秒")
	}
	if sc.Evidence != "illustration" && sc.Evidence != "numeric" && sc.Evidence != "proof" {
		return bad("须区分示意、数值验证与证明")
	}
	if cloud && sc.Evidence == "proof" {
		return bad("情境素材不能作为数学证明")
	}
	for _, id := range sc.SourceIDs {
		if !known[id] {
			return bad("引用了不存在的来源ID")
		}
	}
	if len(sc.Elements) < 1 || len(sc.Elements) > 6 || len(sc.Checks) > 12 {
		return bad("每幕需要1–6个对象，最多12项几何检查")
	}
	objects := map[string]AnimationElement{}
	for _, el := range sc.Elements {
		if el.Kind == "right_triangle" || el.Kind == "rectangle" {
			if len(el.Points) != 1 || !(el.Size[0] > .05 && el.Size[0] <= 7.4 && el.Size[1] > .05 && el.Size[1] <= 7.4) || math.IsNaN(el.Angle) || math.IsInf(el.Angle, 0) || math.Abs(el.Angle) > 360 {
				return bad("参数化图形需要一个锚点、两条正边长及有限旋转角")
			}
			el = constructedElement(el)
		}
		if strings.ContainsAny(el.Label, "\r\n\t") {
			return bad("图例标签不能手工换行")
		}
		if el.ID == "" || len(el.ID) > 40 || objects[el.ID].ID != "" || len([]rune(el.Label)) > 16 {
			return bad("对象ID重复/为空或标签过长")
		}
		if el.Color != "blue" && el.Color != "green" && el.Color != "orange" && el.Color != "purple" {
			return bad("对象颜色须使用标准配色")
		}
		n := len(el.Points)
		switch el.Kind {
		case "polygon":
			if n < 3 || n > 8 || !convexPolygon(el.Points) {
				return bad("需要有面积的凸多边形，不能自交")
			}
		case "line", "arrow":
			if n != 2 || distance(el.Points[0], el.Points[1]) < 0.05 {
				return bad("线段/箭头需要两个不同端点")
			}
		case "circle":
			if n != 1 || !(el.Radius > 0 && el.Radius <= 1.8) {
				return bad("圆的圆心或半径无效")
			}
		case "plot":
			if n < 2 || n > 16 {
				return bad("折线需要2–16个采样点")
			}
		default:
			return bad("不支持的图形类型")
		}
		for _, p := range el.Points {
			for _, t := range []float64{0, 1} {
				x, y := p[0]+t*el.Move[0], p[1]+t*el.Move[1]
				radius := 0.0
				if el.Kind == "circle" {
					radius = el.Radius
				}
				if math.IsNaN(x) || math.IsNaN(y) || math.Abs(x)+radius > 3.7 || math.Abs(y)+radius > 1.8 {
					return bad("图形或移动路径超出演示区")
				}
			}
		}
		objects[el.ID] = el
	}
	partition := false
	for _, check := range sc.Checks {
		els := []AnimationElement{}
		for _, id := range check.Objects {
			el, ok := objects[id]
			if !ok {
				return bad("几何检查引用未知对象")
			}
			el.Points = translated(el)
			els = append(els, el)
		}
		valid := false
		switch check.Kind {
		case "right_angle":
			if len(els) == 1 && els[0].Kind == "polygon" && len(els[0].Points) == 3 {
				p := els[0].Points
				for j := 0; j < 3; j++ {
					u, v := subtract(p[(j+1)%3], p[j]), subtract(p[(j+2)%3], p[j])
					if math.Abs(u[0]*v[0]+u[1]*v[1]) < 1e-6 {
						valid = true
					}
				}
			}
		case "equal_length":
			if len(els) == 2 && len(els[0].Points) == 2 && len(els[1].Points) == 2 {
				valid = math.Abs(distance(els[0].Points[0], els[0].Points[1])-distance(els[1].Points[0], els[1].Points[1])) < 1e-6
			}
		case "equal_area":
			if len(els) == 2 && els[0].Kind == "polygon" && els[1].Kind == "polygon" {
				valid = math.Abs(polygonArea(els[0].Points)-polygonArea(els[1].Points)) < 1e-6
			}
		case "partition":
			valid = validPartition(els)
			partition = partition || valid
		}
		if !valid {
			return bad("几何检查未通过：" + check.Kind)
		}
	}
	if sc.Evidence == "proof" && !partition {
		return bad("本幕标为拼图证明，但缺少有效的分块覆盖校验。拼图须补齐目标图形、分块及覆盖检查；若本幕实际是代数推导或讲解，须明确标为待教师核验的推导示意，不能声称已通过证明检查")
	}
	return nil
}

func constructedElement(el AnimationElement) AnimationElement {
	if el.Kind != "right_triangle" && el.Kind != "rectangle" {
		return el
	}
	anchor := el.Points[0]
	w, h := el.Size[0], el.Size[1]
	points := [][2]float64{{0, 0}, {w, 0}}
	if el.Kind == "rectangle" {
		points = append(points, [2]float64{w, h})
	}
	points = append(points, [2]float64{0, h})
	angle := el.Angle * math.Pi / 180
	for i, p := range points {
		points[i] = [2]float64{anchor[0] + p[0]*math.Cos(angle) - p[1]*math.Sin(angle), anchor[1] + p[0]*math.Sin(angle) + p[1]*math.Cos(angle)}
	}
	el.Kind = "polygon"
	el.Points = points
	return el
}

// Local examples exercise different primitives; they remain explicitly labeled demonstrations.
func addSampleScenes(d *Deck, in TaskInput) {
	for i := range d.Slides {
		if len([]rune(d.Slides[i].Title)) > 28 {
			d.Slides[i].Title = string([]rune(d.Slides[i].Title)[:28])
		}
		el := AnimationElement{ID: "object", Kind: "polygon", Label: "示意对象", Color: "blue", Points: [][2]float64{{-2, -1}, {0, -1}, {0, 1}, {-2, 1}}, Move: [2]float64{.3, 0}}
		checks := []GeometryCheck{}
		caption := "观察对象变化，核对适用条件"
		switch in.Topic {
		case "勾股定理":
			el.Points = [][2]float64{{-2, -1}, {0, -1}, {-2, 1}}
			el.Label = "直角三角形"
			checks = []GeometryCheck{{Kind: "right_angle", Objects: []string{"object"}}}
			caption = "直角是前提，数值示意不是一般性证明"
		case "一次函数", "欧姆定律":
			el.Kind = "plot"
			el.Label = "关系示意，非实测"
			el.Points = [][2]float64{{-2, -1}, {-1, -.5}, {0, 0}, {1, .5}, {2, 1}}
			el.Move = [2]float64{}
			caption = "比较变化量；坐标比例和条件须核验"
		case "浮力":
			el.Kind = "arrow"
			el.Label = "浮力方向示意"
			el.Points = [][2]float64{{0, -1}, {0, 1}}
			el.Move = [2]float64{}
			caption = "浮力竖直向上，大小取决于排液体积"
		case "光的反射":
			el.Kind = "line"
			el.Label = "镜面示意"
			el.Points = [][2]float64{{-2, 0}, {2, 0}}
			el.Move = [2]float64{}
			caption = "角度相对法线测量，仍需核对光路"
		}
		d.Slides[i].Scene = &AnimationScene{Goal: "观察并解释本幕示意", Duration: 3, Caption: caption, Evidence: "illustration", SourceIDs: []string{}, Elements: []AnimationElement{el}, Checks: checks}
		if in.AnimationEngine == "ark" {
			d.Slides[i].Scene.Duration = 5 / float64(len(d.Slides))
		}
	}
}

func subtract(a, b [2]float64) [2]float64 { return [2]float64{a[0] - b[0], a[1] - b[1]} }
func cross(a, b [2]float64) float64       { return a[0]*b[1] - a[1]*b[0] }
func distance(a, b [2]float64) float64    { return math.Hypot(a[0]-b[0], a[1]-b[1]) }
func signedArea(p [][2]float64) float64 {
	a := 0.0
	for i := range p {
		a += cross(p[i], p[(i+1)%len(p)])
	}
	return a / 2
}
func polygonArea(p [][2]float64) float64 { return math.Abs(signedArea(p)) }
func convexPolygon(p [][2]float64) bool {
	for i := range p {
		for j := 0; j < i; j++ {
			if distance(p[i], p[j]) < 1e-6 {
				return false
			}
		}
	}
	if polygonArea(p) < 1e-6 {
		return false
	}
	sign := math.Copysign(1, signedArea(p))
	for i := range p {
		for j := range p {
			if cross(subtract(p[(i+1)%len(p)], p[i]), subtract(p[j], p[i]))*sign < -1e-7 {
				return false
			}
		}
		if distance(p[i], p[(i+1)%len(p)]) < 1e-6 {
			return false
		}
	}
	return true
}
func translated(el AnimationElement) [][2]float64 {
	out := make([][2]float64, len(el.Points))
	for i, p := range el.Points {
		out[i] = [2]float64{p[0] + el.Move[0], p[1] + el.Move[1]}
	}
	return out
}

// Convex polygon clipping: shared edges are allowed, overlapping interiors are not.
func intersection(a, b [][2]float64) [][2]float64 {
	out := append([][2]float64{}, a...)
	sign := math.Copysign(1, signedArea(b))
	for i, p := range b {
		q := b[(i+1)%len(b)]
		input := out
		out = nil
		if len(input) == 0 {
			break
		}
		for j, end := range input {
			start := input[(j+len(input)-1)%len(input)]
			ds := cross(subtract(q, p), subtract(start, p)) * sign
			de := cross(subtract(q, p), subtract(end, p)) * sign
			if (ds >= -1e-9) != (de >= -1e-9) {
				t := ds / (ds - de)
				out = append(out, [2]float64{start[0] + t*(end[0]-start[0]), start[1] + t*(end[1]-start[1])})
			}
			if de >= -1e-9 {
				out = append(out, end)
			}
		}
	}
	return out
}
func validPartition(els []AnimationElement) bool {
	if len(els) < 3 {
		return false
	}
	for _, el := range els {
		if el.Kind != "polygon" {
			return false
		}
	}
	sum := 0.0
	for i := 1; i < len(els); i++ {
		area := polygonArea(els[i].Points)
		sum += area
		if math.Abs(polygonArea(intersection(els[i].Points, els[0].Points))-area) > 1e-6 {
			return false
		}
		for j := 1; j < i; j++ {
			if polygonArea(intersection(els[i].Points, els[j].Points)) > 1e-6 {
				return false
			}
		}
	}
	return math.Abs(sum-polygonArea(els[0].Points)) < 1e-6
}
