package main

import (
	"math"
	"strings"
)

type ForceVector struct {
	Label string  `json:"label"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
}
type ScientificDiagram struct {
	Kind        string        `json:"kind"`
	Title       string        `json:"title"`
	Sides       []float64     `json:"sides,omitempty"`
	Object      string        `json:"object,omitempty"`
	Unit        string        `json:"unit,omitempty"`
	Forces      []ForceVector `json:"forces,omitempty"`
	Equilibrium bool          `json:"equilibrium,omitempty"`
	Voltage     float64       `json:"voltage,omitempty"`
	Resistances []float64     `json:"resistances,omitempty"`
	Connection  string        `json:"connection,omitempty"`
	Closed      *bool         `json:"closed,omitempty"`
}

func functionValue(s FunctionSeries, x float64) (float64, bool) {
	switch s.Kind {
	case "quadratic":
		return s.A*x*x + s.B*x + s.C, true
	case "inverse":
		if x == 0 {
			return 0, false
		}
		return s.K / x, true
	default:
		return s.K*x + s.B, true
	}
}
func validateScientific(sl Slide) error {
	d := sl.Scientific
	if d == nil {
		return nil
	}
	if sl.Graph != nil || sl.Chart != nil || sl.Table != nil || sl.Figure != nil || sl.Picture != nil || sl.Diagram != "" {
		return fail(400, "专业图示请独立展示，不与其他图形混排")
	}
	finite := func(n float64) bool { return !math.IsNaN(n) && !math.IsInf(n, 0) && math.Abs(n) <= 1000 }
	if strings.TrimSpace(d.Title) == "" || len([]rune(d.Title)) > 60 {
		return fail(400, "专业图示需填写60字以内标题")
	}
	switch d.Kind {
	case "triangle":
		if len(d.Sides) != 3 {
			return fail(400, "三角形需填写三条边的长度")
		}
		for _, n := range d.Sides {
			if !finite(n) || n <= 0 {
				return fail(400, "边长需为有限正数")
			}
		}
		a, b, c := d.Sides[0], d.Sides[1], d.Sides[2]
		if a+b <= c || a+c <= b || b+c <= a {
			return fail(400, "这三条边不能构成三角形")
		}
	case "forces":
		if strings.TrimSpace(d.Object) == "" || len([]rune(d.Object)) > 24 || d.Unit != "N" || len(d.Forces) < 1 || len(d.Forces) > 4 {
			return fail(400, "受力图需研究对象、N单位及1至4个力")
		}
		x, y, total := 0.0, 0.0, 0.0
		for _, f := range d.Forces {
			if strings.TrimSpace(f.Label) == "" || len([]rune(f.Label)) > 20 || !finite(f.X) || !finite(f.Y) || math.Hypot(f.X, f.Y) < .000001 {
				return fail(400, "力需明确名称及非零有限分量")
			}
			x += f.X
			y += f.Y
			total += math.Hypot(f.X, f.Y)
		}
		if d.Equilibrium && math.Hypot(x, y) > 1e-6*math.Max(1, total) {
			return fail(400, "标为平衡的受力图合力不为零，请核对所有力")
		}
	case "circuit":
		if !finite(d.Voltage) || d.Voltage <= 0 || len(d.Resistances) < 1 || len(d.Resistances) > 3 || (d.Connection != "series" && d.Connection != "parallel") {
			return fail(400, "电路需正电压、1至3个电阻及串联或并联连接")
		}
		for _, r := range d.Resistances {
			if !finite(r) || r < .01 {
				return fail(400, "电阻需为0.01至1000Ω，不能生成短路")
			}
		}
	default:
		return fail(400, "专业图示类型不支持")
	}
	return nil
}

const scientificContract = `
课堂演示answer字段：练习页可提供单独的答案与关键依据（纯文字，最多2000字），默认不显示在学生画面，教师点击后才展示；完整教师讲解仍放notes。不得将答案写入练习bullets、focus或题目。其他页可省略answer。
专业图示scientific仅支持以下参数化模型，由程序计算图形和数值，不能用装饰代替证据。一页只使用graph或scientific或其他媒体之一。
graph.series支持一次函数{kind:"linear",k:2,b:1}；二次函数{kind:"quadratic",a:1,b:0,c:0}表示y=ax²+bx+c，a非零；反比例{kind:"inverse",k:2}表示y=k/x，k非零且不标x=0。每条mark_x最多2个，坐标范围包含原点且容纳标记点。
scientific={kind:"triangle",title:"具体教学用途",sides:[3,4,5]}，三边为AB、AC、BC，程序按真实比例构造，必须满足三角不等式；这不是拼图证明。
scientific={kind:"forces",title:"物体的受力",object:"研究对象",unit:"N",equilibrium:true,forces:[{label:"重力",x:0,y:-10},{label:"支持力",x:0,y:10}]}，x向右y向上，箭头长度由实际分量计算。列全对象受到的力；equilibrium=true必须合力为零；不能把合力当额外作用力。
scientific={kind:"circuit",title:"电阻串并联",voltage:6,resistances:[10,20],connection:"series或parallel",closed:true}，理想直流电源、理想导线和正电阻，程序计算总电阻及总/支路电流；非实测示例在标题和notes标明。仅此受控电路模型，复杂实验装置需其他有出处的图示，不编造已支持类型。
`
