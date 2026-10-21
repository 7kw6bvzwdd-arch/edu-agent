package main

import (
	"encoding/json"
	"math"
	"strings"
)

type teachingDataChart struct {
	Labels      []string  `json:"labels"`
	Values      []float64 `json:"values"`
	Title       string    `json:"title"`
	Style       string    `json:"style"`
	XValues     []float64 `json:"x_values"`
	YErrors     []float64 `json:"y_errors"`
	XLabel      string    `json:"x_label"`
	YLabel      string    `json:"y_label"`
	SourceLabel string    `json:"source_label"`
}

func usesMatplotlib(sl Slide) bool {
	if sl.Graph != nil && (sl.Graph.Renderer == "matplotlib" || sl.Graph.Animation != nil) {
		return true
	}
	var c teachingDataChart
	return sl.Chart != nil && json.Unmarshal(asJSON(sl.Chart), &c) == nil && (c.Style == "scatter" || c.Style == "errorbar")
}

func validateDataChart(raw any) error {
	if raw == nil {
		return nil
	}
	var c teachingDataChart
	if json.Unmarshal(asJSON(raw), &c) != nil {
		return fail(400, "图表必须使用有效的数值数组")
	}
	n := len(c.Values)
	if n < 1 || n > 8 || len(c.Labels) != n || len([]rune(c.Title)) > 80 {
		return fail(400, "图表需1–8组标签与对应数值，标题最多80字")
	}
	for _, label := range c.Labels {
		if strings.TrimSpace(label) == "" || len([]rune(label)) > 18 || strings.Contains(label, "\n") {
			return fail(400, "图表标签不能为空，最多18字且不能换行")
		}
	}
	for _, value := range c.Values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fail(400, "图表数值必须有限")
		}
	}
	if c.Style != "scatter" && c.Style != "errorbar" {
		if c.Style != "" && c.Style != "bar" && c.Style != "line" {
			return fail(400, "不支持该图表类型")
		}
		if len(c.XValues) > 0 || len(c.YErrors) > 0 {
			return fail(400, "带数值横坐标或误差的数据请使用散点图或误差线图")
		}
		return nil
	}
	if len(c.XValues) != n || strings.TrimSpace(c.XLabel) == "" || strings.TrimSpace(c.YLabel) == "" || len([]rune(c.XLabel)) > 40 || len([]rune(c.YLabel)) > 40 || strings.TrimSpace(c.SourceLabel) == "" || len([]rune(c.SourceLabel)) > 100 {
		return fail(400, "实验图需对应的横坐标、40字以内轴名称及100字以内的数据来源说明")
	}
	if c.Style == "errorbar" && len(c.YErrors) != n || c.Style == "scatter" && len(c.YErrors) > 0 {
		return fail(400, "误差线图须为每个数据点填写非负误差；散点图不使用误差数组")
	}
	for _, values := range [][]float64{c.Values, c.XValues, c.YErrors} {
		for _, value := range values {
			if math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value) > 1000000 {
				return fail(400, "实验图数值必须有限且绝对值不超过1000000")
			}
		}
	}
	for _, value := range c.YErrors {
		if value < 0 {
			return fail(400, "误差大小不能为负数")
		}
	}
	return nil
}

const matplotlibPlotContract = `
原生点击动画：需要课堂逐条比较函数时可设graph.reveal_on_click:true，renderer必须为native且不能同时设置animation。坐标轴始终显示，最多4条series按数组顺序每次点击出现一条，曲线与标记、图例一起出现且在PPT中可编辑。网页与截图预览显示全部曲线；实际放映执行点击顺序。连续参数变化继续使用animation生成GIF；两种方式互斥。不把点击动画用于隐藏必需题设或提前泄露答案。
高清绘图：常规分类统计继续使用可编辑原生chart（bar/line）。非线性函数的精准高清图可在graph中增加renderer:"matplotlib"，原有一次/二次/反比例参数不变；需要PPT内逐条编辑线条时用renderer:"native"或省略。不要输出Python代码、文件路径、网址或任意表达式执行请求。
实验数值图使用chart新类型style:"scatter"（仅散点）或"errorbar"（散点与纵向误差线）。结构示例：{title:"电流与电压（示意数据，非实测）",style:"scatter",labels:["点1","点2","点3"],x_values:[1,2,5],values:[0.1,0.2,0.5],x_label:"电压 U / V",y_label:"电流 I / A",source_label:"示意数据，非实测"}。1–8个点，横纵坐标长度与labels一致，数值绝对值不超过1000000，轴标签说明量与单位（无量纲则说明）。真实数值间距由程序绘制，不能用等距类别轴替代。
只有提供了误差的明确含义和数值才用errorbar，另加与values等长、非负的y_errors。误差指每个点上下的误差大小，不是端点；source_label和notes说明SD/测量误差/其他含义，不能推断或编造误差。数据来源说明不能为空；自编数据显式标“示意数据，非实测”，不能冒充教师实测。这些图由Matplotlib生成图片；参数在网页可编辑并重新绘制，PPT内不是原生可编辑数据图表。
`
