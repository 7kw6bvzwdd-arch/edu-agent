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
	return d != nil && animContains([]string{"thin_lens", "molecule", "cell", "scene", "carbon"}, d.Kind)
}

func validateScienceVisual(d *ScientificDiagram) error {
	finite := func(x float64, lo, hi float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) && x >= lo && x <= hi }
	switch d.Kind {
	case "carbon":
		if !animContains([]string{"diamond", "graphite", "fullerene"}, d.Allotrope) {
			return fail(400, "碳结构仅支持diamond、graphite和fullerene，分别是局部金刚石网络、石墨层及C60笼")
		}
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
		if d.Kind == "carbon" {
			if m.Param != "progress" || m.To != 1 {
				return fail(400, "碳结构animation必须使用param为progress、to为1、duration_ms为1000至8000；末幕比较对象也相同，禁止highlight或rotate。外层tween也用progress到1")
			}
		} else if d.Kind == "thin_lens" {
			if m.Param != "object_distance_cm" || m.To < 1 {
				return fail(400, "thin_lens内部animation.param必须为object_distance_cm、to为1至300厘米的物距，例如animation:{param:object_distance_cm,to:15,duration_ms:3000}；内部不能使用progress/highlight。外层动作才是tween param=progress to=1。静态透镜直接省略animation")
			}
		} else if m.Param != "highlight" || m.To != 1 || d.Kind == "molecule" && (d.HighlightGroup == "" || d.HighlightGroup == "none") || d.Kind == "cell" && len(d.HighlightParts) == 0 {
			return fail(400, "请选择要渐进高亮的官能团或细胞结构")
		}
	}
	return nil
}

type scienceRender struct {
	Native    bool           `json:"native,omitempty"`
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
	raw, err := s.postBytes(ctx, s.cfg.RenderURL+"/render/scientific", map[string]any{"diagram": d, "frames": frames, "native": frames}, "")
	if err != nil {
		return result, fail(422, "科学图示无法绘制，请核对结构、官能团与参数，并检查绘图服务版本；当前内容已保留")
	}
	if json.Unmarshal(raw, &result) != nil || result.Version != "science-visuals-v1" || !strings.HasPrefix(result.Image, "data:image/png;base64,") || len(result.Frames) > 26 {
		return result, fail(503, "科学绘图服务版本不兼容，请更新后重试")
	}
	if frames && d.Kind == "scene" && !result.Native {
		return result, fail(503, "通用动画需要支持连续绘制的绘图服务，请更新后重试")
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

func animScienceKey(d *ScientificDiagram) string { return animDigest([]any{animRendererVersion, d}) }

func animSceneScienceHash(scene AnimScene, f animWorkflow) string {
	hashes := []string{}
	for _, o := range scene.Objects {
		if o.Type == "scientific" {
			hashes = append(hashes, f.Visuals[animScienceKey(o.Scientific)].Hash)
		}
	}
	if len(hashes) == 0 {
		return ""
	}
	return animDigest(hashes)
}

// Freeze the exact image bytes reviewed by the teacher; full builds and free
// edits reuse them just as they reuse speech. Never silently replace a corrupt asset.
func (s *Server) animStoredScience(ctx context.Context, a Actor, t *animTask, sc AnimScript) (map[string]scienceRender, error) {
	if t.Flow.Visuals == nil {
		t.Flow.Visuals = map[string]animVisualAsset{}
	}
	result := map[string]scienceRender{}
	size := 0
	for _, scene := range sc.Scenes {
		for _, o := range scene.Objects {
			if o.Type != "scientific" {
				continue
			}
			key := animScienceKey(o.Scientific)
			saved, exists := t.Flow.Visuals[key]
			var bundle scienceRender
			if exists {
				raw, err := s.readStoredFile(ctx, saved.Path)
				if err != nil || animDigest(raw) != saved.Hash || json.Unmarshal(raw, &bundle) != nil {
					return nil, fail(422, "已保存图示损坏，请检查存储；不会静默替换已确认画面")
				}
			} else {
				var err error
				bundle, err = s.renderScience(ctx, o.Scientific, true)
				if err != nil {
					return nil, err
				}
				raw := asJSON(bundle)
				saved = animVisualAsset{Path: s.animPath(*t, key+"-visual.json"), Hash: animDigest(raw)}
				if err = s.writeStoredFile(ctx, saved.Path, raw); err != nil {
					return nil, err
				}
				t.Flow.Visuals[key] = saved
				if err = s.animCheckpoint(ctx, a, *t); err != nil {
					return nil, err
				}
			}
			if len(bundle.Frames) < 1 {
				return nil, fail(422, "科学图示动画帧缺失")
			}
			size += len(asJSON(bundle))
			if size > 20<<20 {
				return nil, fail(400, "科学图示动画超过20MB，请减少幕数")
			}
			result[scene.ID+"/"+o.ID] = bundle
		}
	}
	return result, nil
}

// Preview only a scene saved in this member's plan.
func (s *Server) animationVisual(w http.ResponseWriter, r *http.Request, a Actor) error {
	var in struct {
		Revision int    `json:"revision"`
		SceneID  string `json:"scene_id"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	tx, err := s.tx(r.Context(), a.TenantID)
	if err != nil {
		return err
	}
	t, err := animRead(r.Context(), tx, r.PathValue("id"), a.MemberID, false)
	tx.Rollback(r.Context())
	if err != nil {
		return err
	}
	if !allowed(a, "animation.render") {
		return fail(403, "没有动画制作权限")
	}
	if in.Revision != t.Revision {
		return fail(409, "方案已更新，请刷新图示")
	}
	for _, scene := range t.Flow.Script.Scenes {
		if scene.ID == in.SceneID {
			if t.Flow.Script.Archetype == "html.direct@1" {
				sc := t.Flow.Script
				sc.Scenes = []AnimScene{scene}
				sc.Representative = scene.ID
				durations := map[string]int{}
				for _, n := range scene.Narration {
					durations[n.ID] = estimateSpeechMS(n.Speech)
				}
				timeline, _ := buildAnimTimeline(sc, durations)
				picture := t.Input.AnimationPicture
				if picture != nil {
					var deck Deck
					deck, err = s.hydrateDeckAssets(context.WithValue(r.Context(), mediaActorKey{}, a), Deck{Slides: []Slide{{Picture: picture}}})
					if err != nil {
						return err
					}
					picture = deck.Slides[0].Picture
				}
				pkg := animPackageInput{Script: sc, Timeline: timeline, Voice: animVoiceEstimated, Picture: picture}
				// A title/thinking scene may legitimately be static during plan preview.
				probe := pkg
				probe.Script.Representative = ""
				if err = s.verifyAuthoredAnimation(r.Context(), probe); err != nil {
					return err
				}
				w.Header().Set("Cache-Control", "private, no-store")
				respond(w, 200, map[string]string{"html": string(buildAnimPackage(pkg)), "notice": "源码画面预览，语音与时长尚未合成；正式审核以代表幕为准"})
				return nil
			}
			for _, o := range scene.Objects {
				if o.Type == "scientific" {
					var result scienceRender
					if saved, exists := t.Flow.Visuals[animScienceKey(o.Scientific)]; exists {
						raw, readErr := s.readStoredFile(r.Context(), saved.Path)
						if readErr != nil || animDigest(raw) != saved.Hash || json.Unmarshal(raw, &result) != nil {
							return fail(422, "已保存图示损坏，请检查存储；不会静默替换已确认画面")
						}
						result.Frames = nil
					} else {
						result, err = s.renderScience(r.Context(), o.Scientific, false)
					}
					if err != nil {
						return err
					}
					w.Header().Set("Cache-Control", "private, no-store")
					respond(w, 200, result)
					return nil
				}
			}
		}
	}
	return fail(404, "本幕没有科学图示")
}

const animScienceContract = `
内部animation.param随专用类型选择：thin_lens只用object_distance_cm、molecule/cell只用highlight、carbon只用progress。scene没有animation字段，只用scene.actions。外层scientific组件的动作参数一律是progress，这两层不能混淆。静态科学对象省略内部animation，不要为强调图示创建无效参数。
通用讲解必须有教学图形，文字卡只能辅助，代表幕不能是纯文字。专用工具适配时必须使用，其他主题用scientific kind=scene组合图形与受控动作；只有配图时要诚实标明图示讲解。
碳单质结构的新构图优先用scientific.kind=scene，scene.objects内放type=carbon、allotrope=diamond/graphite/fullerene，给每个模型归一化中心x/y及width/height；可同屏比较多个结构和短标注，无须文字卡。程序提供真实连接拓扑，不能用矩形/圆点假装碳网络。金刚石为局部网络，中心4邻接、边缘截断，不是独立分子；reveal to=1表示从中心向外逐步构建。石墨是三层局部截取、实际多层；slide to在0至1内表示层间相对滑动、保持层内键，不是导电原因或断键仿真。C60为60碳90棱、12五元环20六元环，每碳连接3碳；rotate只用于观察视角，不能当作性质或单质转化。碳单质比较至少包含diamond与graphite的动态证据；提到C60则加fullerene。外层仍用句子锚点tween progress到1，内部sentence分别锚定所讲结构对应旁白，不挤在第一句同时完成。
构图示例：scientific:{kind:"scene",title:"连接方式为何不同",scene:{duration_ms:6000,objects:[{id:"diamond",type:"carbon",allotrope:"diamond",x:0.25,y:0.46,width:0.4,height:0.7},{id:"graphite",type:"carbon",allotrope:"graphite",x:0.75,y:0.46,width:0.4,height:0.7},{id:"label",type:"text",x:0.5,y:0.92,text:"同为碳，连接方式不同",font_size:30}],actions:[{object:"diamond",op:"reveal",to:1,start_ms:0,duration_ms:2500,sentence:"s1n1"},{object:"graphite",op:"slide",to:0.7,start_ms:3000,duration_ms:2500,sentence:"s1n2"}]}}。按本幕真实句子ID修改sentence，内部仅回退预览排程按start_ms，视频按实际句子边界触发。旧kind=carbon专用单对象仍兼容，但不能完成逐步构建或同屏对照。
通用讲解新增科学图示scientific组件。凸透镜成像、分子结构/官能团、典型细胞结构课题应优先使用，不能只给知识卡。
此组件每幕仅一个外层scientific对象；多个结构、图形和标注放scene.objects内部，可自由组合受控对象进行对照。外层对象为{id:"visual",type:"scientific",slot:"main",params:{progress:0},scientific:{...与课件scientific同格式...}}；不带text/items/kind，标题和条件写在scientific内。
仅本组件允许tween progress，0到1表示从科学图示animation定义的初始状态到终点；其余通用讲解组件仍不允许tween。动作使用句子锚点，明确duration_ms；旁白、动画与条件保持一致，不能声称完成任意实验仿真。
透镜scientific:{kind:"thin_lens",title:"凸透镜成像",focal_length_cm:10,object_distance_cm:30,object_height_cm:3,animation:{param:"object_distance_cm",to:15,duration_ms:3000}}；焦距1–100cm、物距1–300cm、物高0.1–10cm。程序计算像距/放大率/近轴光线，经过焦点时不成有限远像；标为理想模型非实测。
分子scientific:{kind:"molecule",title:"乙醇羟基",smiles:"CCO",highlight_group:"hydroxyl",animation:{param:"highlight",to:1,duration_ms:2000}}；单一分子、最多80原子、SMILES最多300字符，结构对应关系需依据，无法确定则待核验。官能团仅none/hydroxyl/carboxyl/amino/carbonyl/double_bond且必须存在。仅二维结构示意，不等于空间构型、反应或机理验证；none不能设置高亮动画。
细胞scientific:{kind:"cell",title:"典型植物细胞",cell_type:"plant",visible_parts:["cell_wall","cell_membrane","cytoplasm","nucleus","chloroplast","vacuole"],highlight_parts:["chloroplast"],animation:{param:"highlight",to:1,duration_ms:2000}}；cell_type为plant/animal；结构可选cell_wall/cell_membrane/cytoplasm/nucleus/mitochondrion/chloroplast/vacuole，高亮必须属于显示结构。动物细胞不含cell_wall/chloroplast/中央液泡vacuole。典型结构示意，非真实比例或显微照片；不泛化为所有植物细胞都有叶绿体。
三个新图示可省略animation静态显示，所有animation为1000–8000毫秒；动态图示至少有与旁白同步的progress变化，静态图示用show/emphasize动作。超出专用类型保留真实需求，优先使用scene组合实际教学图形与动作，或教师配图；无法表达则明确返回能力缺口，禁止交付纯文字冒充教学动画，不伪造支持能力。
` + generalSceneContract

func scienceAnimated(d *ScientificDiagram) bool {
	return d != nil && (d.Animation != nil || d.Kind == "scene" && d.Scene != nil && len(d.Scene.Actions) > 0)
}
