package main

import (
	"math"
)

// Function parameters use the same kinds and coefficients as courseware graph.series.
func animFunctionParams(o AnimObject) []string {
	if o.Type == "scientific" {
		return []string{"progress"}
	}
	if o.Type == "function" {
		switch o.Kind {
		case "linear":
			return []string{"k", "b"}
		case "quadratic":
			return []string{"a", "b", "c"}
		case "inverse":
			return []string{"k"}
		}
	}
	return animParams[o.Type]
}

func animFunctionSeries(o AnimObject, p map[string]float64) FunctionSeries {
	return FunctionSeries{Kind: o.Kind, A: p["a"], B: p["b"], C: p["c"], K: p["k"]}
}

func animFunctionProblems(s AnimScene) ([]string, bool) {
	var problems []string
	moving := false
	for _, o := range s.Objects {
		if o.Type != "function" {
			continue
		}
		if !animContains([]string{"linear", "quadratic", "inverse"}, o.Kind) {
			problems = append(problems, "函数kind须为linear、quadratic或inverse")
			continue
		}
		p := map[string]float64{}
		for k, v := range o.Params {
			p[k] = v
		}
		var ax AnimObject
		for _, obj := range s.Objects {
			if obj.ID == o.Axes {
				ax = obj
			}
		}
		check := func(endpoint bool) {
			f := animFunctionSeries(o, p)
			for _, v := range p {
				if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 20 {
					problems = append(problems, "函数系数须为有限数且绝对值不超过20")
				}
			}
			if endpoint && (o.Kind == "quadratic" && f.A == 0 || o.Kind == "inverse" && f.K == 0 || o.Kind == "linear" && f.K == 0) {
				problems = append(problems, "函数端点的最高次项或反比例系数不能为零")
			}
			visible := false
			for i := 0; i <= 800; i++ {
				x := ax.Params["xmin"] + (ax.Params["xmax"]-ax.Params["xmin"])*float64(i)/800
				y, ok := functionValue(f, x)
				visible = visible || ok && y >= ax.Params["ymin"] && y <= ax.Params["ymax"]
			}
			if !visible {
				problems = append(problems, "函数不在可见坐标范围内")
			}
			for _, pt := range s.Objects {
				if pt.Type != "point" || pt.Line != o.ID {
					continue
				}
				x := pt.Params["x"]
				for _, a := range s.Actions {
					if a.Op == "tween" && a.Object == pt.ID && a.Param == "x" && a.To != nil {
						if *a.To < ax.Params["xmin"] || *a.To > ax.Params["xmax"] {
							problems = append(problems, "标注点动作超出坐标范围")
						}

						if o.Kind == "inverse" && x**a.To <= 0 {
							problems = append(problems, "反比例函数上的点不能经过x=0")
						}
					}
				}
				_, defined := functionValue(f, x)
				if !defined || x < ax.Params["xmin"] || x > ax.Params["xmax"] {
					problems = append(problems, "函数标注点超出定义域或坐标范围")
				}
				if animPointLabel.MatchString(pt.Text) {
					// Numeric labels become stale during parameter motion; use a semantic label.
					problems = append(problems, "参数变化点请使用语义标注，坐标由程序显示")
				}
			}
		}
		check(true)
		for _, a := range s.Actions {
			if a.Op != "tween" || a.Object != o.ID || a.To == nil || !animContains(animFunctionParams(o), a.Param) {
				continue
			}
			from := p[a.Param]
			moving = moving || !animNear(from, *a.To)
			if o.Kind == "inverse" && a.Param == "k" && from**a.To <= 0 {
				problems = append(problems, "反比例系数不能连续经过k=0，请分幕比较")
			}
			for i := 1; i <= 20; i++ {
				p[a.Param] = from + (*a.To-from)*float64(i)/20
				check(i == 20)
			}
		}
	}
	return problems, moving
}
