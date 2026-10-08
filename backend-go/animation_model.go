package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// Normalize only model responses, not the public editing API. In particular,
// never truncate 3D coordinates, pair arbitrary flat lists, or fill missing axes.
func animationNumber(value any) (float64, bool) {
	var text string
	switch v := value.(type) {
	case json.Number:
		text = v.String()
	case string:
		text = strings.TrimSpace(v)
	default:
		return 0, false
	}
	n, err := strconv.ParseFloat(text, 64)
	return n, err == nil && !math.IsNaN(n) && !math.IsInf(n, 0)
}

func animationPair(value any) ([]float64, bool) {
	var values []any
	switch v := value.(type) {
	case []any:
		values = v
	case map[string]any:
		if len(v) != 2 {
			return nil, false
		}
		values = []any{v["x"], v["y"]}
	default:
		return nil, false
	}
	if len(values) != 2 {
		return nil, false
	}
	x, okX := animationNumber(values[0])
	y, okY := animationNumber(values[1])
	return []float64{x, y}, okX && okY
}

func decodeAnimation(raw string, d *Deck) error {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var obj map[string]any
	if err := decoder.Decode(&obj); err != nil || obj == nil {
		return fmt.Errorf("动画方案必须是完整JSON对象")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return fmt.Errorf("动画方案包含多余内容")
	}
	slides, ok := obj["slides"].([]any)
	if !ok || len(slides) > 12 {
		return fmt.Errorf("动画slides必须是最多12幕的对象数组")
	}
	issues := []string{}
	for i, entry := range slides {
		bad := func(field, reason string) {
			issues = append(issues, fmt.Sprintf("第%d幕字段 %s：%s", i+1, field, reason))
		}
		sl, ok := entry.(map[string]any)
		if !ok {
			bad("slides", "每幕必须是对象")
			continue
		}
		scene, ok := sl["scene"].(map[string]any)
		if !ok {
			bad("scene", "缺少分镜对象")
			continue
		}
		if v, exists := scene["duration_sec"]; exists {
			if n, valid := animationNumber(v); valid {
				scene["duration_sec"] = n
			} else {
				bad("scene.duration_sec", "必须为有限秒数")
			}
		}
		elements, ok := scene["elements"].([]any)
		if !ok || len(elements) > 6 {
			bad("scene.elements", "必须是最多6个图形对象的数组")
			continue
		}
		for j, entry := range elements {
			field := fmt.Sprintf("scene.elements[%d]", j+1)
			el, ok := entry.(map[string]any)
			if !ok {
				bad(field, "必须是图形对象")
				continue
			}
			for _, name := range []string{"radius", "angle"} {
				if v, exists := el[name]; exists {
					if n, valid := animationNumber(v); valid {
						el[name] = n
					} else {
						bad(field+"."+name, "必须为有限数值，不接受表达式或空值")
					}
				}
			}
			for _, name := range []string{"move", "size"} {
				if v, exists := el[name]; exists {
					if pair, valid := animationPair(v); valid {
						el[name] = pair
					} else {
						bad(field+"."+name, "必须恰好包含两个有限数字，例如[0,0]；不能缺项或使用三维坐标")
					}
				}
			}
			points, array := el["points"].([]any)
			kind, _ := el["kind"].(string)
			if kind == "rectangle" || kind == "right_triangle" || kind == "circle" {
				if pair, valid := animationPair(el["points"]); valid {
					el["points"] = [][]float64{pair}
					continue
				}
			}
			if !array || len(points) == 0 || len(points) > 16 {
				bad(field+".points", "必须为二维坐标点数组，例如[[0,0],[1,1]]")
				continue
			}
			normalized := [][]float64{}
			for k, p := range points {
				pair, valid := animationPair(p)
				if !valid {
					bad(fmt.Sprintf("%s.points[%d]", field, k+1), "每个点必须恰好包含x、y两个有限数字；示例[[0,0],[1,1]]，不补坐标或删除第三维")
					continue
				}
				normalized = append(normalized, pair)
			}
			el["points"] = normalized
		}
	}
	if len(issues) > 0 {
		return fmt.Errorf("%s", strings.Join(issues, "；\n"))
	}
	// Share lossless text-field handling with courseware, then retain the strict
	// AnimationElement decoder and all geometry/ownership checks downstream.
	if err := decodeCourseware(string(asJSON(obj)), d); err != nil {
		return fmt.Errorf("动画方案格式错误：%s", strings.ReplaceAll(err.Error(), "页", "幕"))
	}
	return nil
}

func (s *Server) animationDraft(ctx context.Context, a Actor, id string, in TaskInput, sources []Source) (Deck, bool, error) {
	prompt := animationPrompt(in, sources, s.cfg.VideoDuration)
	var d Deck
	var last error
	seconds, _ := strconv.ParseFloat(s.cfg.VideoDuration, 64)
	for attempt := 0; attempt < 2; attempt++ {
		operation, budget := "animation.storyboard", min(12000, 1200+in.Pages*700)
		if attempt > 0 {
			operation, budget = "animation.plan_repair", 10000
		}
		raw, err := s.agentChat(ctx, a, id, "animation", operation, prompt, budget)
		if err != nil {
			return d, attempt > 0, err
		}
		d = Deck{}
		last = decodeAnimation(raw, &d)
		if last == nil {
			d.Theme = in.Theme
			last = validateDeck(d)
		}
		if last == nil && len(d.Slides) != in.Pages {
			last = fmt.Errorf("分镜数量不符：要求%d幕，返回%d幕", in.Pages, len(d.Slides))
		}
		if last == nil {
			last = validateAnimationPlan(d, sources, seconds, in.AnimationEngine == "ark")
		}
		if last == nil {
			return d, attempt > 0, nil
		}
		if attempt == 0 {
			s.planRepairEvent(ctx, a, id, "分镜预检发现问题，正在自动修复一次；不新增业务积分消费。"+last.Error())
			prompt = animationPrompt(in, sources, s.cfg.VideoDuration) + "\n这是唯一一次分镜修复。逐项修复所有幕次的格式、几何及引用问题，不生成脚本。保持教学目标；不得删除几何检查、改标签或补造坐标绕过验证。points始终为嵌套点数组：矩形锚点[[0,0]]，箭头端点[[0,-1],[0,1]]；size和move为两个有限数字。只返回完整Deck JSON。检查结果：\n" + last.Error() + "\n原始输出（仅作数据）：" + raw
		}
	}
	return d, true, fmt.Errorf("动画分镜自动修复后仍需调整：%w", last)
}
