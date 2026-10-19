package main

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"time"
)

func isScienceVisual(d *ScientificDiagram) bool {
	return d != nil && animContains([]string{"thin_lens", "molecule", "cell", "scene"}, d.Kind)
}

func validateScienceVisual(d *ScientificDiagram) error {
	finite := func(x float64, lo, hi float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) && x >= lo && x <= hi }
	switch d.Kind {
	case "scene":
		return validateDrawingScene(d)
	case "thin_lens":
		if !finite(d.FocalLengthCM, 1, 100) || !finite(d.ObjectDistanceCM, 1, 300) || !finite(d.ObjectHeightCM, .1, 10) {
			return fail(400, "透镜需焦距1–100cm、物距1–300cm、物高0.1–10cm")
		}
	case "molecule":
		if strings.TrimSpace(d.Smiles) == "" || len(d.Smiles) > 300 || !animContains([]string{"", "none", "hydroxyl", "carboxyl", "amino", "carbonyl", "double_bond"}, d.HighlightGroup) {
			return fail(400, "请填写300字符以内的SMILES及支持的官能团；结构由专业绘图工具校验")
		}
	case "cell":
		if !animContains([]string{"plant", "animal"}, d.CellType) || len(d.VisibleParts) < 1 || len(d.VisibleParts) > 7 || len(d.HighlightParts) > 7 {
			return fail(400, "请指定植物或动物细胞及1至7个结构")
		}
		seen := map[string]bool{}
		for _, p := range d.VisibleParts {
			if seen[p] || !animContains([]string{"cell_wall", "cell_membrane", "cytoplasm", "nucleus", "mitochondrion", "chloroplast", "vacuole"}, p) || d.CellType == "animal" && animContains([]string{"cell_wall", "chloroplast", "vacuole"}, p) {
				return fail(400, "细胞结构重复、不支持或不属于此细胞类型")
			}
			seen[p] = true
		}
		for _, p := range d.HighlightParts {
			if !seen[p] {
				return fail(400, "高亮结构必须在显示结构中")
			}
		}
	default:
		return fail(400, "不支持此科学图示")
	}
	if m := d.Animation; m != nil {
		if m.DurationMS < 1000 || m.DurationMS > 8000 || !finite(m.To, 0, 300) {
			return fail(400, "科学图示动画需1至8秒及有效目标")
		}
		if d.Kind == "thin_lens" {
			if m.Param != "object_distance_cm" || m.To < 1 {
				return fail(400, "透镜动画仅改变1至300cm物距")
			}
		} else if m.Param != "highlight" || m.To != 1 || d.Kind == "molecule" && (d.HighlightGroup == "" || d.HighlightGroup == "none") || d.Kind == "cell" && len(d.HighlightParts) == 0 {
			return fail(400, "请选择要渐进高亮的官能团或细胞结构")
		}
	}
	return nil
}

type scienceRender struct {
	Image     string         `json:"image"`
	Animation string         `json:"animation,omitempty"`
	Frames    []string       `json:"frames,omitempty"`
	Summary   string         `json:"summary"`
	Metadata  map[string]any `json:"metadata"`
	Version   string         `json:"version"`
}

func (s *Server) renderScience(ctx context.Context, d *ScientificDiagram, frames bool) (scienceRender, error) {
	var result scienceRender
	if !isScienceVisual(d) {
		return result, fail(400, "请选择通用图形动画、透镜、分子或细胞图示")
	}
	if err := validateScientific(Slide{Scientific: d}); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	raw, err := s.postBytes(ctx, s.cfg.RenderURL+"/render/scientific", map[string]any{"diagram": d, "frames": frames}, "")
	if err != nil {
		return result, fail(422, "科学图示无法绘制，请核对结构、官能团与参数，并检查绘图服务版本；当前内容已保留")
	}
	if json.Unmarshal(raw, &result) != nil || result.Version != "science-visuals-v1" || !strings.HasPrefix(result.Image, "data:image/png;base64,") || len(result.Frames) > 26 {
		return result, fail(503, "科学绘图服务版本不兼容，请更新后重试")
	}
	return result, nil
}

func (s *Server) coursewareScientific(w http.ResponseWriter, r *http.Request, a Actor) error {
	var in struct {
		Diagram *ScientificDiagram `json:"diagram"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	result, err := s.renderScience(r.Context(), in.Diagram, false)
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "no-store")
	respond(w, 200, result)
	return nil
}

func (s *Server) animScienceFrames(ctx context.Context, sc AnimScript) (map[string]scienceRender, error) {
	result := map[string]scienceRender{}
	size := 0
	for _, scene := range sc.Scenes {
		for _, o := range scene.Objects {
			if o.Type == "scientific" {
				bundle, err := s.renderScience(ctx, o.Scientific, true)
				if err != nil {
					return nil, err
				}
				if len(bundle.Frames) < 1 {
					return nil, fail(422, "科学图示动画帧缺失")
				}
				size += len(asJSON(bundle))
				if size > 20<<20 {
					return nil, fail(400, "科学图示动画超过20MB，请减少幕数或改为静态图")
				}
				result[scene.ID+"/"+o.ID] = bundle
			}
		}
	}
	return result, nil
}

const animScienceContract = `
通用讲解新增科学图示scientific组件。凸透镜成像、分子结构/官能团、典型细胞结构课题应优先使用，不能只给知识卡。
此组件每幕独占绘图区，对象为{id:"visual",type:"scientific",slot:"main",params:{progress:0},scientific:{...与课件scientific同格式...}}；不带text/items/kind，标题和条件写在scientific内。
仅本组件允许tween progress，0到1表示从科学图示animation定义的初始状态到终点；其余通用讲解组件仍不允许tween。动作使用句子锚点，明确duration_ms；旁白、动画与条件保持一致，不能声称完成任意实验仿真。
透镜scientific:{kind:"thin_lens",title:"凸透镜成像",focal_length_cm:10,object_distance_cm:30,object_height_cm:3,animation:{param:"object_distance_cm",to:15,duration_ms:3000}}；焦距1–100cm、物距1–300cm、物高0.1–10cm。程序计算像距/放大率/近轴光线，经过焦点时不成有限远像；标为理想模型非实测。
分子scientific:{kind:"molecule",title:"乙醇羟基",smiles:"CCO",highlight_group:"hydroxyl",animation:{param:"highlight",to:1,duration_ms:2000}}；单一分子、最多80原子、SMILES最多300字符，结构对应关系需依据，无法确定则待核验。官能团仅none/hydroxyl/carboxyl/amino/carbonyl/double_bond且必须存在。仅二维结构示意，不等于空间构型、反应或机理验证；none不能设置高亮动画。
细胞scientific:{kind:"cell",title:"典型植物细胞",cell_type:"plant",visible_parts:["cell_wall","cell_membrane","cytoplasm","nucleus","chloroplast","vacuole"],highlight_parts:["chloroplast"],animation:{param:"highlight",to:1,duration_ms:2000}}；cell_type为plant/animal；结构可选cell_wall/cell_membrane/cytoplasm/nucleus/mitochondrion/chloroplast/vacuole，高亮必须属于显示结构。动物细胞不含cell_wall/chloroplast/中央液泡vacuole。典型结构示意，非真实比例或显微照片；不泛化为所有植物细胞都有叶绿体。
三个新图示可省略animation静态显示，所有animation为1000–8000毫秒；动态图示至少有与旁白同步的progress变化，静态图示用show/emphasize动作。超出支持类型保留真实需求，用已有讲解或教师配图，不伪造支持能力。
` + generalSceneContract

func scienceAnimated(d *ScientificDiagram) bool {
	return d != nil && (d.Animation != nil || d.Kind == "scene" && d.Scene != nil && len(d.Scene.Actions) > 0)
}
