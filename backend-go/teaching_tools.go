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
	Kind             string            `json:"kind"`
	Title            string            `json:"title"`
	Sides            []float64         `json:"sides,omitempty"`
	Object           string            `json:"object,omitempty"`
	Unit             string            `json:"unit,omitempty"`
	Forces           []ForceVector     `json:"forces,omitempty"`
	Equilibrium      bool              `json:"equilibrium,omitempty"`
	Voltage          float64           `json:"voltage,omitempty"`
	Resistances      []float64         `json:"resistances,omitempty"`
	Connection       string            `json:"connection,omitempty"`
	Closed           *bool             `json:"closed,omitempty"`
	FocalLengthCM    float64           `json:"focal_length_cm,omitempty"`
	ObjectDistanceCM float64           `json:"object_distance_cm,omitempty"`
	ObjectHeightCM   float64           `json:"object_height_cm,omitempty"`
	Smiles           string            `json:"smiles,omitempty"`
	HighlightGroup   string            `json:"highlight_group,omitempty"`
	CellType         string            `json:"cell_type,omitempty"`
	VisibleParts     []string          `json:"visible_parts,omitempty"`
	HighlightParts   []string          `json:"highlight_parts,omitempty"`
	Animation        *ScienceAnimation `json:"animation,omitempty"`
	Scene            *DrawingScene     `json:"scene,omitempty"`
}

type ScienceAnimation struct {
	Param      string  `json:"param"`
	To         float64 `json:"to"`
	DurationMS int     `json:"duration_ms"`
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
	if d.Animation != nil && !isScienceVisual(d) {
		return fail(400, "该专业工具仅支持静态绘图；组合动画请使用通用图形工具")
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
	case "thin_lens", "molecule", "cell", "scene":
		return validateScienceVisual(d)
	default:
		return fail(400, "专业图示类型不支持")
	}
	return nil
}

const scientificContract = `
生成前核对：练习的决定性条件须在学生可见题目或已明确的全课约定中，不能从教师答案倒填题设；判断题允许正确答案为“信息不足”，禁止自行补连线或其他条件。独立作图练习不附上待作答案曲线，读图题与已解例题可显示。自编数值标“示意，非实测”或“按题设计算”，待测空表保留待测。对真实实验，测力计量程覆盖负载，电表量程覆盖待测量，电阻额定功率与发热条件匹配；试触不能使超量程安全，局部短路只作纸面故障分析。纯理论计算不虚构实际器材或测量记录。最终答案和纠错说明均独立验算，正确的教师延伸保留在备注。
课堂演示answer字段：练习页可提供单独的答案与关键依据（纯文字，最多2000字），默认不显示在学生画面，教师点击后才展示；完整教师讲解仍放notes。不得将答案写入练习bullets、focus或题目。其他页可省略answer。
专业图示scientific仅支持以下参数化模型，由程序计算图形和数值，不能用装饰代替证据。每页说明视觉方案与教学用途，允许函数图配教学表格、图片配表格、图片配流程图等互补组合。坐标图与专业图示需要足够绘图区；图表过密时分配到其他页，不为了装饰凑数量。
graph.series支持一次函数{kind:"linear",k:2,b:1}；二次函数{kind:"quadratic",a:1,b:0,c:0}表示y=ax²+bx+c，a非零；反比例{kind:"inverse",k:2}表示y=k/x，k非零且不标x=0。每条mark_x最多2个，坐标范围包含原点且容纳标记点。
graph可选animation:{series:0,param:"a",to:-1,duration_ms:2000}，从所选series的当前系数渐变到to，PPT导出为循环GIF；series为从0开始的曲线编号，时长1000至8000毫秒。只变化该函数已有的系数。二次函数a可连续跨0，a=0时明确不是二次函数；端点a非零。反比例k不能跨0。适合观察参数变化；需要在PPT里编辑曲线时不填animation，保留原生静态图。
scientific={kind:"triangle",title:"具体教学用途",sides:[3,4,5]}，三边为AB、AC、BC，程序按真实比例构造，必须满足三角不等式；这不是拼图证明。
scientific={kind:"forces",title:"物体的受力",object:"研究对象",unit:"N",equilibrium:true,forces:[{label:"重力",x:0,y:-10},{label:"支持力",x:0,y:10}]}，x向右y向上，箭头长度由实际分量计算。列全对象受到的力；equilibrium=true必须合力为零；不能把合力当额外作用力。
scientific={kind:"circuit",title:"电阻串并联",voltage:6,resistances:[10,20],connection:"series或parallel",closed:true}，理想直流电源、理想导线和正电阻，程序计算总电阻及总/支路电流；非实测示例在标题和notes标明。仅此受控电路模型，复杂实验装置需其他有出处的图示，不编造已支持类型。
scientific新增三类，由系统绘图，参数可编辑：
凸透镜{kind:"thin_lens",title:"观察物距与像距",focal_length_cm:10,object_distance_cm:30,object_height_cm:3}。理想薄透镜近轴模型，长度单位cm，焦距1–100、物距1–300、物高0.1–10。像距/放大率/光线由程序计算；物距等于焦距时不成有限远像，不把示意写成实测。可加animation:{param:"object_distance_cm",to:15,duration_ms:2000}，程序处理经过焦点的状态。
分子{kind:"molecule",title:"乙醇的羟基",smiles:"CCO",highlight_group:"hydroxyl"}。SMILES必须有依据，不确定名称与结构对应时标待核验；仅单一分子、最多80原子。highlight_group仅none/hydroxyl/carboxyl/amino/carbonyl/double_bond，所选基团必须存在。RDKit生成二维结构，不代表空间构型、反应可行性或机理已验证。可加animation:{param:"highlight",to:1,duration_ms:1500}渐进高亮，不能伪造反应动画。
细胞{kind:"cell",title:"植物细胞结构",cell_type:"plant",visible_parts:["cell_wall","cell_membrane","cytoplasm","nucleus","chloroplast","vacuole"],highlight_parts:["chloroplast"]}。cell_type为plant/animal，结构编号仅cell_wall/cell_membrane/cytoplasm/nucleus/mitochondrion/chloroplast/vacuole；highlight_parts必须属于visible_parts。animal不含cell_wall/chloroplast/中央液泡vacuole。此图是典型细胞的简化结构示意，非真实比例或显微照片，不推断所有植物细胞都有叶绿体。可加animation:{param:"highlight",to:1,duration_ms:1500}。三个新类型的动画时长1000–8000毫秒；PPT中为GIF，网页编辑保留参数，PPT内不是逐个结构可编辑对象。
` + generalSceneContract
