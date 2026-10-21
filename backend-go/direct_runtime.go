package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// Uses the renderer's bounded browser; no generated code runs in the API process.
func (s *Server) verifyAuthoredAnimation(ctx context.Context, in animPackageInput) error {
	if in.Script.Archetype != "html.direct@1" {
		return nil
	}
	scenes := []map[string]any{}
	for _, scene := range in.Script.Scenes {
		var span AnimSceneSpan
		for _, candidate := range in.Timeline.Scenes {
			if candidate.ID == scene.ID {
				span = candidate
				break
			}
		}
		duration := span.End - span.Start
		if duration <= 0 {
			return fmt.Errorf("missing scene timeline")
		}
		points := []int{1, duration - 1}
		for _, cue := range span.Sentences {
			points = append(points, (cue.Start+cue.End)/2-span.Start)
		}
		sort.Ints(points)
		frames := []map[string]any{}
		for _, time := range points {
			cues := map[string]any{}
			for _, cue := range span.Sentences {
				for _, sub := range span.Subtitles {
					if sub.ID == cue.ID {
						cue = sub
						break
					}
				}
				progress := max(0.0, min(1.0, float64(time+span.Start-cue.Start)/float64(cue.End-cue.Start)))
				cues[cue.ID] = map[string]any{"start": cue.Start - span.Start, "end": cue.End - span.Start, "progress": progress}
			}
			frames = append(frames, map[string]any{"time": time, "duration": duration, "progress": float64(time) / float64(duration), "cues": cues})
		}
		scenes = append(scenes, map[string]any{"id": scene.ID, "html": scene.HTML, "frames": frames})
	}
	picture := ""
	if in.Picture != nil {
		picture = in.Picture.DataURL
	}
	payload := map[string]any{"scenes": scenes, "representative": in.Script.Representative, "picture": picture}
	raw, err := s.postBytes(ctx, s.cfg.RenderURL+"/render/authored-animation-check", payload, "")
	if err != nil {
		return fmt.Errorf("动画浏览器验收未通过：%w", err)
	}
	var checked struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(raw, &checked) != nil || checked.Version != "authored-animation-v1" {
		return fmt.Errorf("请更新动画浏览器检查服务，不能跳过源码运行验收")
	}
	return nil
}

func authoredAnimationHTTPError(status int, body []byte) error {
	var response struct {
		Detail struct {
			Reason string `json:"reason"`
		} `json:"detail"`
	}
	_ = json.Unmarshal(body, &response)
	messages := map[string]string{
		"invalid_geometry":              "动画图形坐标包含NaN或无穷值，请修复SVG几何坐标",
		"nondeterministic_reverse_seek": "动画倒退定位结果不一致，请按输入帧重算而非累加",
		"no_visible_change":             "代表幕在语音cue采样点没有可见变化",
		"resource_blocked":              "动画引用了被禁止的外部资源",
		"script_error":                  "动画脚本运行失败，请检查renderFrame",
		"console_error":                 "动画页面产生浏览器运行错误",
		"browser_validation_failed":     "动画未通过浏览器运行验收",
	}
	if message := messages[response.Detail.Reason]; message != "" {
		return fail(status, message)
	}
	return fail(status, "动画未通过浏览器运行验收")
}
