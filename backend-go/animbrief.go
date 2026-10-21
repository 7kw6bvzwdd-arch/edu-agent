package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Structured controls win. Older clients can still specify a numeric range in
// requirements; convert it once at creation and persist the accepted contract.
var animDurationRange = regexp.MustCompile(`([0-9]+)\s*(秒|分钟|分)?\s*[-–—~～至到]\s*([0-9]+)\s*(秒|分钟|分)`)

func animNormalizeBrief(in *TaskInput) error {
	if in.AnimationOpening == "" {
		in.AnimationOpening = "natural"
		if animHasAny(in.Requirements, "直接开始", "不要开场", "无需开场", "不需要开场") {
			in.AnimationOpening = "direct"
		}
	}
	if in.AnimationOpening != "natural" && in.AnimationOpening != "direct" {
		return fmt.Errorf("开场请选择自然引入或直接开始")
	}
	if in.AnimationMinSeconds == 0 && in.AnimationMaxSeconds == 0 {
		in.AnimationMinSeconds, in.AnimationMaxSeconds = 60, 90
		if m := animDurationRange.FindStringSubmatch(in.Requirements); m != nil {
			lo, _ := strconv.Atoi(m[1])
			hi, _ := strconv.Atoi(m[3])
			unit := m[2]
			if unit == "" {
				unit = m[4]
			}
			if unit != "秒" {
				lo *= 60
			}
			if m[4] != "秒" {
				hi *= 60
			}
			in.AnimationMinSeconds, in.AnimationMaxSeconds = lo, hi
		}
	}
	if in.AnimationMinSeconds < 30 || in.AnimationMaxSeconds > 180 || in.AnimationMinSeconds > in.AnimationMaxSeconds {
		return fmt.Errorf("自动播放时长须在30至180秒内，最短不能超过最长")
	}
	return nil
}

func animDurationProblem(in TaskInput, ms int) string {
	lo, hi := in.AnimationMinSeconds, in.AnimationMaxSeconds
	if lo == 0 && hi == 0 {
		lo, hi = 30, 180
	} // historical task
	if ms < lo*1000 || ms > hi*1000 {
		return fmt.Sprintf("真实成片时长%.1f秒，不符合目标%d至%d秒；请返回方案调整旁白或教学停顿后重新确认，已生成音频保留", float64(ms)/1000, lo, hi)
	}
	return ""
}

// Empty preferences identify historical tasks; do not rewrite their deliveries.
func animOpeningProblems(sc AnimScript, in TaskInput) []string {
	if in.AnimationOpening != "natural" || len(sc.Scenes) == 0 {
		return nil
	}
	for i, n := range sc.Scenes[0].Narration {
		if i == 2 {
			break
		}
		if animHasAny(strings.ToLower(n.Speech), "同学", "今天", "这节课", "本节课", "一起", "想一想", "你有没有", "为什么", "你知道", "today", "let's", "have you", "why") {
			return nil
		}
	}
	return []string{"第一幕前两句朗读缺少自然引入，请围绕本课问题邀请学生观察或思考，不要只陈述定义"}
}

func animAuthorProblems(sc AnimScript, in TaskInput) []string {
	return append(animVisualProblems(sc, in), animOpeningProblems(sc, in)...)
}

// ponytail: conservative planning estimate, calibrated from six Seed Audio probes;
// actual decoded audio remains the delivery authority. Never retime approved text.
func animPlanBudgetProblems(sc AnimScript, in TaskInput) []string {
	if in.AnimationMaxSeconds == 0 {
		return nil
	}
	ms := map[string]int{}
	for _, scene := range sc.Scenes {
		for _, n := range scene.Narration {
			d, latin := 300, false
			for _, r := range n.Speech {
				if r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
					if !latin {
						d += 450
					}
					latin = true
				} else {
					latin = false
					if !unicode.IsSpace(r) {
						d += 300
					}
				}
			}
			ms[n.ID] = d
		}
	}
	tl, _ := buildAnimTimeline(sc, ms)
	if tl.TotalMS > in.AnimationMaxSeconds*1000 {
		return []string{fmt.Sprintf("方案预估%.1f秒，超过目标%d至%d秒。请精简旁白和重复解释，保留有意义的观察停顿；中文按每字约0.3秒、英文每词约0.45秒另加句间间隔与停顿规划，实测仍以音频为准", float64(tl.TotalMS)/1000, in.AnimationMinSeconds, in.AnimationMaxSeconds)}
	}
	return nil
}

// Require explicit narration cues only for newly authored/changed text. Existing
// saved plans remain playable and local revision does not rewrite other scenes.
func animTextCueProblems(sc, old AnimScript) []string {
	var problems []string
	for _, scene := range sc.Scenes {
		for _, o := range scene.Objects {
			if !animContains([]string{"card", "timeline", "relationship", "comparison", "illustration"}, o.Type) || len(o.ItemSentences) == len(o.Items) {
				continue
			}
			unchanged := false
			for _, previous := range old.Scenes {
				if previous.ID == scene.ID {
					for _, item := range previous.Objects {
						if item.ID == o.ID && animDigest(item) == animDigest(o) && animDigest(previous.Narration) == animDigest(scene.Narration) {
							unchanged = true
						}
					}
				}
			}
			if !unchanged {
				problems = append(problems, fmt.Sprintf("幕%s对象%s：新生成或修改的讲解组件须提供item_sentences，与items逐条绑定本幕旁白，避免整框文字同时出现", scene.ID, o.ID))
			}
		}
	}
	return problems
}
