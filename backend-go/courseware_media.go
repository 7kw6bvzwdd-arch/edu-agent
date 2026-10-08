package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

type TeachingPicture struct {
	DataURL string `json:"data_url"`
	Caption string `json:"caption"`
	Source  string `json:"source"`
}

func (s *Server) requireTeachingMediaRenderer(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", s.cfg.RenderURL+"/health", nil)
	if err != nil {
		return fail(503, "PPT排版服务地址无效")
	}
	res, err := s.http.Do(req)
	if err != nil {
		return fail(503, "PPT排版服务未就绪，请启动新版排版服务")
	}
	defer res.Body.Close()
	var health map[string]any
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&health) != nil || health["courseware_media_version"] != "media-v1" {
		return fail(503, "PPT排版服务尚未更新，请重启后端与排版服务后重试；未丢弃图解内容")
	}
	return nil
}

// Text-only model calls never receive uploaded binary data or mutate the original.
func slideForTextModel(sl Slide) Slide {
	if sl.Picture != nil {
		picture := *sl.Picture
		picture.DataURL = "[图片未发送，仅提供说明；不得评价画面准确性]"
		sl.Picture = &picture
	}
	return sl
}
func deckForTextModel(d Deck) Deck {
	d.Slides = append([]Slide(nil), d.Slides...)
	for i := range d.Slides {
		d.Slides[i] = slideForTextModel(d.Slides[i])
	}
	return d
}

type TeachingTable struct {
	Title   string     `json:"title,omitempty"`
	Headers []string   `json:"headers"`
	Rows    [][]string `json:"rows"`
}
type FigureItem struct {
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
}
type TeachingFigure struct {
	Kind   string       `json:"kind"`
	Title  string       `json:"title,omitempty"`
	Items  []FigureItem `json:"items"`
	Unit   string       `json:"unit,omitempty"`
	Values []float64    `json:"values,omitempty"`
}

func validateTeachingMedia(sl Slide) error {
	valid := func(s string, limit int, required bool) bool {
		return len([]rune(s)) <= limit && (!required || strings.TrimSpace(s) != "")
	}
	if !valid(sl.Focus, 60, false) {
		return fail(400, "核心问题或结论最多60字")
	}
	if len(sl.Emphasis) > 6 {
		return fail(400, "每页最多6个重点词")
	}
	for _, term := range sl.Emphasis {
		if !valid(term, 20, true) {
			return fail(400, "重点词不能为空且最多20字")
		}
	}
	if p := sl.Picture; p != nil {
		if !valid(p.Caption, 60, true) || !valid(p.Source, 300, true) || len(p.DataURL) > 180000 {
			return fail(400, "图片需填写说明与出处，或文件过大")
		}
		if sl.Chart != nil || sl.Diagram != "" || (sl.Table != nil && sl.Figure != nil) {
			return fail(400, "一页最多组合图片与表格或流程图，请移除冲突的图形")
		}
		header, data, ok := strings.Cut(p.DataURL, ",")
		want := map[string]string{"data:image/png;base64": "png", "data:image/jpeg;base64": "jpeg"}[header]
		if !ok || want == "" {
			return fail(400, "图片只接受上传的PNG/JPEG，不接受网址或本地路径")
		}
		raw, err := base64.StdEncoding.Strict().DecodeString(data)
		if err != nil || len(raw) > 128000 {
			return fail(400, "图片数据无效或超过128KB，请重新上传")
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
		if err != nil || format != want || cfg.Width < 32 || cfg.Height < 32 || cfg.Width > 1600 || cfg.Height > 1600 {
			return fail(400, "图片尺寸或格式无效，请重新上传")
		}
	}
	if (sl.Table != nil || sl.Figure != nil) && (sl.Chart != nil || sl.Diagram != "") {
		return fail(400, "表格/流程图与图表/固定示意图请分开放到不同页面")
	}
	if t := sl.Table; t != nil {
		if !valid(t.Title, 60, false) || len(t.Headers) < 2 || len(t.Headers) > 4 || len(t.Rows) < 1 || len(t.Rows) > 5 {
			return fail(400, "教学表格需要2–4列、1–5行数据，标题最多60字")
		}
		for _, h := range t.Headers {
			if !valid(h, 24, true) {
				return fail(400, "表头不能为空且最多24字")
			}
		}
		for _, row := range t.Rows {
			if len(row) != len(t.Headers) {
				return fail(400, "表格每行须与表头列数一致")
			}
			for _, c := range row {
				if !valid(c, 60, true) {
					return fail(400, "表格单元格不能为空且最多60字")
				}
			}
		}
	}
	if f := sl.Figure; f != nil {
		if (f.Kind != "flow" && f.Kind != "timeline" && f.Kind != "route" && f.Kind != "interval") || !valid(f.Title, 60, false) || len(f.Items) < 2 || len(f.Items) > 4 {
			return fail(400, "图解仅支持flow/timeline/route/interval，2–4个节点，标题最多60字")
		}
		if !valid(f.Unit, 12, f.Kind == "interval") || len(f.Values) > 4 {
			return fail(400, "区间图需填写最多12字的单位，最多4个数值")
		}
		if f.Kind == "interval" {
			if len(f.Values) != len(f.Items) {
				return fail(400, "区间数值必须与节点一一对应")
			}
			for i, v := range f.Values {
				if math.IsNaN(v) || math.IsInf(v, 0) || (i > 0 && v <= f.Values[i-1]) {
					return fail(400, "区间数值必须为有限且严格递增的数")
				}
			}
			if math.IsInf(f.Values[len(f.Values)-1]-f.Values[0], 0) {
				return fail(400, "区间跨度过大")
			}
		}
		for _, item := range f.Items {
			if !valid(item.Label, 24, true) || !valid(item.Detail, 60, false) {
				return fail(400, "图解节点标题不能为空且最多24字，说明最多60字")
			}
		}
	}
	return nil
}

const coursewareMediaContract = `
课堂图解字段（可省略，不要null）：
emphasis：0–6个重点词的字符串数组，每词最多20字，必须来自本页正文/表格/核心问题，只标关键概念或数值，不把整句标色。
picture仅供教师上传PNG/JPEG；模型不得生成图片data_url或伪造图片地址。配图不能代替实际图解或来源证据。
focus：最多60字的具体核心问题或结论，单独突出显示，不重复标题。导入与练习页只写问题/提示，不提前公布答案；答案与依据放notes，概念讲解与例题页可以展示结论。
table：{title:短标题,headers:[列名],rows:[[单元格文字]]}；2–4列、1–5行数据，全部字符串，表头最多24字，单元格最多60字。用于真实的条件对照、实验记录或分类，不用表格装饰普通段落。每行列数一致，未知数值用“待测”，不能编造实测。
figure：{kind:flow、timeline或route,title:短标题,items:[{label:短标签,detail:简短说明}]}；2–4个节点，标签最多24字，说明最多60字。flow表达真实的实验步骤/解题推理，timeline表达先后顺序（不按时间比例，不能充当坐标图）。route表达2–4个实际相关地点的连接关系（示意，不表示实际方向或距离；标签建议8字内，说明12字内），适合旅程、物流等情境，不能用于证明位移或冒充真实地图。只用与课题直接有关的节点，不写“观察→思考→总结”凑图。
table与figure可同页（建议3节点及简短标签）；两者出现时本页bullets只留1–2条短提问，证据显示在图表中，不重复大段文字。table/figure不能与chart/diagram同页，信息过多请分配到其他页。
数值区间用figure.kind=interval，另填unit（最多12字，如min）和values（与items一一对应的有限严格递增数值，如[0,30,32,60]）。线段长度由数值按比例绘制，自动显示相邻间隔及总跨度；时间先换算为同一计时起点下的数值，不能直接填08:30字符串。适合解释点与区间；不用于往返路线或非单调数据。标签尽量简短，完整解释放notes；优先独占一页，不与表格重复。
组合图表必须提供互补信息，不能把表格中的相同数据抄成时间线凑图；例如表格记录具体时刻，流程图解释如何计算停留时间。
至少2页使用与知识点相关的chart/diagram/table/figure；长课件建议三分之一页面含教学图表。不适合量化时用条件对照或具体推理图，不编造统计数据。封面可使用有关的diagram或精简figure代替纯装饰。
例题正文展示题设、关键推理或代入、结果检查（不能仅写“先整理条件再计算”）；完整讲法放notes。练习答案保留在notes。对照页写出具体对照对象，错误与纠正逐项配对。
参考格式（仅示范结构，内容必须换成本课题）：table:{title:"行程记录（教学示例）",headers:["站点","到站","离站"],rows:[["甲站","—","08:00"],["乙站","08:30","08:32"]]},figure:{kind:"flow",title:"求停留时间",items:[{label:"读取两个时刻",detail:"到站08:30，离站08:32"},{label:"计算时间间隔",detail:"08:32−08:30=2分钟"}]},focus:"08:30和停留2分钟表达的含义相同吗？",bullets:["请根据时刻表说明理由"]。不得复制该例到无关学科。
`
