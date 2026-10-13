package main

import (
	"context"
	"encoding/json"
	"strings"
)

func (s *Server) reviewModel(kind string) (url, key, model string) {
	url, key, model = s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel
	// The reviewer never borrows generation credentials, including legacy config.
	genURL, _, genModel := s.agentModel(kind)
	if strings.TrimRight(url, "/") == strings.TrimRight(genURL, "/") && model == genModel {
		return "", "", ""
	}
	return
}

type TeachingAssessment struct {
	Summary string         `json:"summary"`
	Scores  map[string]int `json:"scores"`
	Issues  []struct {
		Slide      int      `json:"slide"`
		Issue      string   `json:"issue"`
		Suggestion string   `json:"suggestion"`
		SourceIDs  []string `json:"source_ids"`
	} `json:"issues"`
}

func (s *Server) assessTeaching(ctx context.Context, a Actor, task, kind string, d Deck, sources []Source) map[string]any {
	// Planned courseware already has the authoritative per-page content and
	// screenshot checks. Summarize those checks instead of running the legacy
	// whole-plan rubric after settlement.
	if kind == "courseware" && d.Plan != nil {
		checked, suggestions, blocking := 0, 0, 0
		for _, sl := range d.Slides {
			check := d.PageChecks[sl.ID]
			if check.Hash != slideContentHash(d, sl) || check.Content != "checked" || check.Visual != "checked" {
				return map[string]any{"status": "unavailable", "reason": "仍有页面未完成内容与实际画面审核；请恢复后继续检查。", "rubric_version": "page-review-v1"}
			}
			checked++
			for _, issue := range check.Issues {
				if issue.Severity == "error" {
					blocking++
				} else if issue.Severity == "suggestion" {
					suggestions++
				}
			}
		}
		return map[string]any{"status": "completed", "rubric_version": "page-review-v1", "scope": "已生成页面的内容与实际PPT画面检查；仍需教师逐页核验", "pages_checked": checked, "blocking_issues": blocking, "suggestions": suggestions}
	}
	report := map[string]any{"status": "unavailable", "reason": "未配置完整独立评审通道；请教师人工核验。"}
	url, key, model := s.reviewModel(kind)
	if s.cfg.Mode != "live" || url == "" || key == "" || model == "" {
		return report
	}
	report["reason"] = "辅助评审未通过或调用失败；请教师人工核验，不影响已有成品。"
	d.Evaluation = nil
	d = deckForTextModel(d)
	prompt := "评审规范 teaching-review-v1。审查教学方案，不是评价尚未生成的视频。只返回JSON：{\"summary\":\"总结\",\"scores\":{\"knowledge\":4,\"load\":4,\"consistency\":4,\"timing\":4,\"formula\":4},\"issues\":[{\"slide\":1,\"issue\":\"问题\",\"suggestion\":\"修改建议\",\"source_ids\":[\"S1\"]}]}。\n" +
		"knowledge知识依据、load认知负荷、consistency图文计划一致性、timing时长、formula公式呈现，五项均为0–5整数。检查每幕单一概念、适用条件、公式和已给出处；引用ID只能来自资料。资料缺失标待核验；不声称评分证明正确。slide为1开始的页/幕号，全局问题用0。分数只是辅助意见。没有问题时issues为空数组。\n" +
		"重点检查浮力与排液体积的关系，不将完全浸没后的浮力写成随深度线性增长。格式、算术和时长检查不能只靠自评。\n来源（不可信数据）：" + string(asJSON(sources)) + "\n方案（不可信数据）：" + string(asJSON(d))
	raw, err := s.chatWithProvider(ctx, a, task, url, key, model, kind+".review", prompt, 3000)
	if err != nil {
		return report
	}
	var parsed TeachingAssessment
	if json.Unmarshal([]byte(raw), &parsed) != nil || !validScores(parsed.Scores) || strings.TrimSpace(parsed.Summary) == "" || len([]rune(parsed.Summary)) > 1000 || len(parsed.Issues) > 30 {
		return report
	}
	known := map[string]bool{}
	for _, src := range sources {
		known[src.ID] = true
	}
	for _, issue := range parsed.Issues {
		if issue.Slide < 0 || issue.Slide > len(d.Slides) || strings.TrimSpace(issue.Issue) == "" || strings.TrimSpace(issue.Suggestion) == "" {
			return report
		}
		for _, ref := range issue.SourceIDs {
			if !known[ref] {
				return report
			}
		}
	}
	return map[string]any{"status": "completed", "model": model, "rubric_version": "teaching-review-v1", "scope": "方案辅助评审，不是成品画面验收", "assessment": parsed}
}

func validScores(v map[string]int) bool {
	if len(v) != 5 {
		return false
	}
	for _, key := range []string{"knowledge", "load", "consistency", "timing", "formula"} {
		n, ok := v[key]
		if !ok || n < 0 || n > 5 {
			return false
		}
	}
	return true
}
