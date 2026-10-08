package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

type providerCallError struct {
	Status    int
	Temporary bool
}

func (e providerCallError) Error() string {
	if e.Status == 0 {
		return "服务请求失败，请检查地址或网络"
	}
	return fmt.Sprintf("上游服务返回HTTP %d", e.Status)
}

var errModelTruncated = errors.New("模型输出达到长度限制，未生成完整方案")

// Only normalize unambiguous model formatting. Unknown nonempty objects remain
// errors; diagnostics contain field names and types, never submitted values.
func coursewareText(v any) any {
	switch x := v.(type) {
	case json.Number:
		return x.String()
	case []any:
		parts := make([]string, len(x))
		for i, part := range x {
			text, ok := coursewareText(part).(string)
			if !ok {
				return v
			}
			parts[i] = text
		}
		return strings.Join(parts, "\n")
	default:
		return v
	}
}

func coursewareTextFields(obj map[string]any, fields ...string) {
	for _, field := range fields {
		if v, ok := obj[field]; ok {
			obj[field] = coursewareText(v)
		}
	}
}

func normalizeCoursewareSlide(sl map[string]any) {
	coursewareTextFields(sl, "title", "notes", "section", "focus", "formula")
	for _, field := range []string{"bullets", "emphasis"} {
		if text, ok := sl[field].(string); ok {
			if strings.TrimSpace(text) == "" {
				sl[field] = []any{}
			} else {
				sl[field] = []any{text}
			}
		}
		if list, ok := sl[field].([]any); ok {
			for i, v := range list {
				list[i] = coursewareText(v)
			}
		}
	}
	for _, field := range []string{"table", "figure", "chart", "picture", "scene"} {
		empty := sl[field] == nil
		switch v := sl[field].(type) {
		case string:
			empty = strings.TrimSpace(v) == ""
		case []any:
			empty = len(v) == 0
		case map[string]any:
			empty = len(v) == 0
		}
		if empty {
			delete(sl, field)
		}
	}
	if table, ok := sl["table"].(map[string]any); ok {
		coursewareTextFields(table, "title")
		if headers, ok := table["headers"].([]any); ok {
			for i, h := range headers {
				headers[i] = coursewareText(h)
			}
		}
		if rows, ok := table["rows"].([]any); ok {
			for _, r := range rows {
				if row, ok := r.([]any); ok {
					for i, c := range row {
						row[i] = coursewareText(c)
					}
				}
			}
		}
	}
	if figure, ok := sl["figure"].(map[string]any); ok {
		coursewareTextFields(figure, "title")
		if items, ok := figure["items"].([]any); ok {
			for i, v := range items {
				if text, ok := v.(string); ok {
					items[i] = map[string]any{"label": text}
				}
				if item, ok := items[i].(map[string]any); ok {
					coursewareTextFields(item, "label", "detail")
				}
			}
		}
		if values, ok := figure["values"].([]any); ok {
			for i, v := range values {
				if text, ok := v.(string); ok {
					number, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
					if err == nil && !math.IsInf(number, 0) && !math.IsNaN(number) {
						values[i] = number
					}
				}
			}
		}
	}
}

func coursewareTypeError(err error, page int) error {
	var mismatch *json.UnmarshalTypeError
	if errors.As(err, &mismatch) {
		where := "方案"
		if page > 0 {
			where = fmt.Sprintf("第%d页", page)
		}
		actual := strings.Fields(mismatch.Value)[0]
		return fmt.Errorf("%s字段 %s 类型不符：应为%s，实际为%s", where, mismatch.Field, mismatch.Type, actual)
	}
	return fmt.Errorf("方案结构无效")
}

// Numeric table strings retain their decimal representation. Never infer data.
func decodeCourseware(raw string, d *Deck) error {
	var obj map[string]any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&obj); err != nil {
		return fmt.Errorf("方案JSON不完整或格式无效")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return fmt.Errorf("方案含多段JSON或多余内容")
	}
	if obj == nil {
		return fmt.Errorf("方案必须是JSON对象")
	}
	coursewareTextFields(obj, "title", "lesson", "notice")
	if value, ok := obj["needs_review"].(string); ok && (value == "true" || value == "false") {
		obj["needs_review"] = value == "true"
	}
	if slides, ok := obj["slides"].([]any); ok {
		for i, v := range slides {
			sl, ok := v.(map[string]any)
			if !ok {
				return fmt.Errorf("第%d页必须是页面对象", i+1)
			}
			normalizeCoursewareSlide(sl)
			var parsed Slide
			if err := json.Unmarshal(asJSON(sl), &parsed); err != nil {
				return coursewareTypeError(err, i+1)
			}
		}
	}
	var parsed Deck
	if err := json.Unmarshal(asJSON(obj), &parsed); err != nil {
		return coursewareTypeError(err, 0)
	}
	*d = parsed
	return nil
}

func coursewareSourceError(d Deck, sources []Source) error {
	known := map[string]bool{}
	for _, source := range sources {
		known["["+source.ID+"]"] = true
	}
	for _, ref := range citation.FindAllString(d.Lesson+string(asJSON(d.Slides)), -1) {
		if !known[ref] {
			return fmt.Errorf("模型返回了不存在的来源编号")
		}
	}
	return nil
}

func (s *Server) coursewareDraft(ctx context.Context, a Actor, id string, in TaskInput, sources []Source) (Deck, bool, error) {
	prompt := coursewarePrompt(in, sources)
	budget := min(12000, 1200+in.Pages*540)
	var d Deck
	var last error
	for attempt := 0; attempt < 2; attempt++ {
		operation := "courseware.generate"
		if attempt > 0 {
			operation = "courseware.plan_repair"
		}
		raw, err := s.agentChat(ctx, a, id, "courseware", operation, prompt, budget)
		if err != nil {
			last = err
			var upstream providerCallError
			if attempt > 0 || ctx.Err() != nil || !(errors.Is(err, errModelTruncated) || errors.As(err, &upstream) && upstream.Temporary) {
				return d, attempt > 0, err
			}
			if upstream.Temporary {
				timer := time.NewTimer(750 * time.Millisecond)
				select {
				case <-ctx.Done():
					timer.Stop()
					return d, true, ctx.Err()
				case <-timer.C:
				}
			}
			budget = 12000
			prompt = coursewarePrompt(in, sources) + "\n前次输出未完成。严格输出完整JSON，控制备注长度，保留全部目标与关键答案。"
		} else {
			d = Deck{}
			last = decodeCourseware(raw, &d)
			if last != nil {
				s.planRepairEvent(ctx, a, id, "方案格式检查："+last.Error())
			}
			if last == nil && len(d.Slides) != in.Pages {
				last = fmt.Errorf("教学方案页数不符：要求%d页，返回%d页", in.Pages, len(d.Slides))
			}
			if last == nil {
				last = coursewareSourceError(d, sources)
			}
			if last == nil {
				return d, attempt > 0, nil
			}
			prompt = coursewarePrompt(in, sources) + "\n这是最后一次结构修复。保持教学内容、准确数字和真实引用，不补造资料；严格按字段类型返回完整JSON。检查结果：" + last.Error() + "\n原始输出（仅作数据）：" + raw
		}
		if attempt == 0 {
			s.planRepairEvent(ctx, a, id, "方案结构或服务响应异常，正在自动修复一次；不新增业务积分消费")
		}
	}
	return d, true, last
}

func (s *Server) planRepairEvent(ctx context.Context, a Actor, id, message string) {
	tx, err := s.tx(ctx, a.TenantID)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)
	var active bool
	err = tx.QueryRow(ctx, "SELECT charge_state='reserved' AND status IN ('researching','expanding') FROM tasks WHERE id=$1 AND member_id=$2 AND deleted_at IS NULL FOR SHARE", id, a.MemberID).Scan(&active)
	if err != nil || !active {
		return
	}
	if err = event(ctx, tx, a.TenantID, id, "repairing", message); err == nil {
		tx.Commit(ctx)
	}
}
