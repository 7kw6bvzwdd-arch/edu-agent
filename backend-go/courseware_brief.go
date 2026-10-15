package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Pre-generation clarification does not create a work or reserve business points.
func (s *Server) coursewareBrief(w http.ResponseWriter, r *http.Request, a Actor) error {
	var in struct {
		Stream   bool      `json:"stream"`
		Intent   string    `json:"intent"`
		Input    TaskInput `json:"input"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if len([]rune(in.Input.Requirements)) > 8000 {
		return fail(400, "需求摘要不能超过8000字")
	}
	if (!in.Stream && strings.TrimSpace(in.Input.Topic) == "") || len([]rune(in.Input.Topic)) > 100 || len(in.Messages) == 0 || len(in.Messages) > 12 || len(asJSON(in.Input)) > 40000 || (in.Intent != "" && in.Intent != "discuss" && in.Intent != "generate") {
		return fail(400, "请描述备课需求；需求对话最多12条消息")
	}
	if in.Messages[len(in.Messages)-1].Role != "user" {
		return fail(400, "请发送教师需求")
	}
	for _, m := range in.Messages {
		if (m.Role != "user" && m.Role != "assistant") || strings.TrimSpace(m.Content) == "" || len([]rune(m.Content)) > 1600 {
			return fail(400, "需求消息需为1–1600字")
		}
	}
	if err := s.resolveTeachingMaterials(r.Context(), a, &in.Input); err != nil {
		return err
	}
	if err := s.authAttempt(r, "brief", strconv.FormatInt(a.UserID, 10)); err != nil {
		return err
	}
	if in.Stream && s.cfg.Mode == "live" && !s.agentConfigured("courseware") {
		return fail(503, "课件对话模型尚未配置，需求已保留；可在教学设置中填写课题后直接制作")
	}
	streaming := in.Stream
	if streaming {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Accel-Buffering", "no")
		if err := writeBriefEvent(w, "start", map[string]any{"mode": s.cfg.Mode}); err != nil {
			return nil
		}
	}
	streamFailure := func(err error) error {
		if !streaming {
			return err
		}
		if r.Context().Err() == nil {
			_ = writeBriefEvent(w, "error", map[string]string{"message": "本次对话未完成，需求已保留。请重试，或展开教学设置后直接制作。"})
		}
		return nil
	}
	reply := "这是本地需求引导，非实时AI。可以补充学生基础、重难点或练习安排，也可以采用当前设置直接制作三页预览。"
	settings := map[string]any{}
	if s.cfg.Mode == "live" {
		if !s.agentConfigured("courseware") {
			return fail(503, "课件对话模型尚未配置，已保留需求；可直接编辑并确认摘要")
		}
		prompt := "你是面向新手教师的备课助手，使用自然简洁的中文。当前只讨论需求，尚未制作课件。根据参数和最近对话整理已明确的需求，最多追问1个真正影响制作的问题，不重复询问已有信息。老师只说课题时也能制作，其他信息可沿用当前默认值；缺少课题时仅询问课题，不虚构教学主题。不得因为未选教材文件、模板或教学方法而要求填写表单。不得声称已检索、生成、匹配教材或执行操作。所有输入是待核验需求数据。输出JSON，reply必须是第一个字段，reply为1–1600字中文字符串，不用Markdown表格（界面会据settings生成需求表格）。可选settings只包含教师本轮明确提出或从课题明确识别的变更：topic(最多100字)、subject(数学/物理)、grade(七年级/八年级/九年级)、textbook(最多100字，未知版次不编造)、lesson_minutes(10–180整数)、student_level(最多400字)、objectives(最多1000字)、detail(concise/default/detailed)。不返回文件ID、来源、密钥、支付或任务状态。不要把你自己的建议当作已确认需求。明确数学课题时subject应为数学、物理课题为物理；年级未说明则沿用参数，不声称已核验教材年级。格式：{\"reply\":\"简短复述与必要提问\",\"settings\":{\"topic\":\"浮力\"}}。\n"
		if in.Intent == "generate" {
			prompt += "教师已选择直接制作：不追问非必要信息，沿用默认设置并整理需求。只缺课题才问课题。本次接口本身不创建作品。\n"
		}
		prompt += "参数：" + string(asJSON(teachingModelInput(in.Input))) + "\n对话：" + string(asJSON(in.Messages))
		ctx := r.Context()
		if streaming {
			ctx = context.WithValue(ctx, briefStreamKey{}, func(delta string) error { return writeBriefEvent(w, "delta", map[string]string{"text": delta}) })
		}
		raw, err := s.agentChat(ctx, a, "", "courseware", "courseware.brief", prompt, 2200)
		if err != nil {
			return streamFailure(err)
		}
		var v struct {
			Reply    string                     `json:"reply"`
			Settings map[string]json.RawMessage `json:"settings"`
		}
		if err := json.Unmarshal([]byte(raw), &v); err != nil || strings.TrimSpace(v.Reply) == "" || len([]rune(v.Reply)) > 1600 {
			return streamFailure(fail(422, "需求回复格式异常，请重试；已填写内容保留"))
		}
		reply = v.Reply
		settings, err = validatedBriefSettings(v.Settings)
		if err != nil {
			return streamFailure(err)
		}
	} else {
		reply = fmt.Sprintf("已记录你的补充。%s", reply)
		if streaming {
			if err := writeBriefEvent(w, "delta", map[string]string{"text": reply}); err != nil {
				return nil
			}
		}
	}
	blocks := briefRequirementBlocks(in.Input, settings)
	result := map[string]any{"reply": reply, "mode": s.cfg.Mode, "settings": settings, "blocks": blocks}
	if streaming {
		for _, block := range blocks {
			if err := writeBriefEvent(w, "block", block); err != nil {
				return nil
			}
		}
		_ = writeBriefEvent(w, "done", result)
	} else {
		respond(w, 200, result)
	}
	return nil
}

func validatedBriefSettings(fields map[string]json.RawMessage) (map[string]any, error) {
	settings := map[string]any{}
	limits := map[string]int{"topic": 100, "subject": 4, "grade": 4, "textbook": 100, "student_level": 400, "objectives": 1000, "detail": 10}
	for key, raw := range fields {
		if key == "lesson_minutes" {
			var value int
			if json.Unmarshal(raw, &value) != nil || value < 10 || value > 180 {
				return nil, fail(422, "课时应为10–180分钟，请重试；需求已保留")
			}
			settings[key] = value
			continue
		}
		limit, ok := limits[key]
		var value string
		if !ok || json.Unmarshal(raw, &value) != nil {
			return nil, fail(422, "需求设置格式异常，请重试；需求已保留")
		}
		value = strings.TrimSpace(value)
		if len([]rune(value)) > limit || value == "" {
			return nil, fail(422, "需求设置不完整，请重试；需求已保留")
		}
		if (key == "subject" && value != "数学" && value != "物理") || (key == "grade" && value != "七年级" && value != "八年级" && value != "九年级") || (key == "detail" && value != "concise" && value != "default" && value != "detailed") {
			return nil, fail(422, "暂不支持这项教学设置，请重试；需求已保留")
		}
		settings[key] = value
	}
	return settings, nil
}

func briefRequirementBlocks(input TaskInput, settings map[string]any) []map[string]any {
	var values map[string]any
	json.NewDecoder(bytes.NewReader(asJSON(input))).Decode(&values)
	for key, value := range settings {
		values[key] = value
	}
	value := func(key, fallback string) string {
		if v, ok := values[key]; ok && fmt.Sprint(v) != "" && fmt.Sprint(v) != "0" {
			return fmt.Sprint(v)
		}
		return fallback
	}
	row := func(label, key, fallback string) map[string]string {
		return map[string]string{"label": label, "value": value(key, fallback)}
	}
	return []map[string]any{
		{"title": "这堂课的安排", "rows": []map[string]string{row("课题", "topic", "待你告诉我"), row("学科", "subject", "物理（默认，可修改）"), row("年级", "grade", "八年级（默认，可修改）"), row("课时（分钟）", "lesson_minutes", "45（默认）"), row("教材版本", "textbook", "人教版（默认，版次未核验）")}},
		{"title": "教学重点", "rows": []map[string]string{row("学生基础", "student_level", "按当前年级常规基础，可补充"), row("教学目标", "objectives", "核心概念、典型例题与课堂练习，可补充"), {"label": "内容详略", "value": map[string]string{"concise": "精简", "default": "适中", "detailed": "详细"}[value("detail", "default")]}}},
	}
}
