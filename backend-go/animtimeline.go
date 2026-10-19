package main

// Timeline = actual audio durations ⊕ teaching holds. Only the program writes it.

import (
	"fmt"
	"unicode/utf8"
)

const (
	animSentenceGapMS = 250
	animMinTotalMS    = 30_000
	animMaxTotalMS    = 180_000
)

type AnimTimeline struct {
	TotalMS  int             `json:"total_ms"`
	Scenes   []AnimSceneSpan `json:"scenes"`
	Warnings []string        `json:"warnings,omitempty"`
}
type AnimSceneSpan struct {
	Subtitles []AnimSpan `json:"subtitles,omitempty"`
	ID        string     `json:"id"`
	Start     int        `json:"start"`
	End       int        `json:"end"`
	Sentences []AnimSpan `json:"sentences"`
	Holds     []AnimSpan `json:"holds"`
	Actions   []AnimSpan `json:"actions"`
}
type AnimSpan struct {
	ID    string `json:"id"`
	Type  string `json:"type,omitempty"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// estimateSpeechMS is only for sample mode and plan previews (~4.3 Chinese chars/s).
func estimateSpeechMS(speech string) int { return 300 + 230*utf8.RuneCountInString(speech) }

// buildAnimTimeline lays out scenes sequentially in global teaching time.
// audioMS must hold the decoded duration of every sentence. In auto play an
// await hold lasts its ms; in step mode the player freezes time at its start.
func buildAnimTimeline(sc AnimScript, audioMS map[string]int) (AnimTimeline, []string) {
	var tl AnimTimeline
	var p []string
	cursor := 0
	for _, s := range sc.Scenes {
		span := AnimSceneSpan{ID: s.ID, Start: cursor}
		sentences := map[string]AnimSpan{}
		holdsAfter := func(after string) {
			for _, h := range s.Holds {
				if h.After == after {
					span.Holds = append(span.Holds, AnimSpan{ID: h.ID, Type: h.Type, Start: cursor, End: cursor + h.MS})
					cursor += h.MS
				}
			}
		}
		for _, n := range s.Narration {
			d, ok := audioMS[n.ID]
			if !ok || d <= 0 {
				p = append(p, fmt.Sprintf("幕%s句子%s缺少实际音频时长", s.ID, n.ID))
				d = 1
			}
			sp := AnimSpan{ID: n.ID, Start: cursor, End: cursor + d}
			span.Sentences, sentences[n.ID] = append(span.Sentences, sp), sp
			cursor = sp.End + animSentenceGapMS
			holdsAfter(n.ID)
		}
		holdsAfter("")
		natural := cursor
		ends := map[string]int{}
		lastTween := map[string]AnimSpan{}
		for _, a := range s.Actions {
			start := span.Start
			switch {
			case a.At.Sentence != "":
				sp := sentences[a.At.Sentence]
				start = sp.Start
				if a.At.Anchor == "end" {
					start = sp.End
				}
			case a.At.After != "":
				start = ends[a.At.After]
			case a.At.Scene == "end":
				start = natural
			}
			start += a.At.OffsetMS
			d := a.DurationMS
			if d == 0 {
				d = animDefaultMS[a.Op]
			}
			sp := AnimSpan{ID: a.ID, Type: a.Op, Start: start, End: start + d}
			span.Actions, ends[a.ID] = append(span.Actions, sp), sp.End
			if sp.End > cursor {
				cursor = sp.End
			}
			if a.Op == "tween" {
				key := a.Object + "." + a.Param
				for _, o := range s.Objects {
					if o.ID == a.Object && animIsShape(o.Type) {
						key = a.Object + ".transform"
						break
					}
				}
				if prev, ok := lastTween[key]; ok && sp.Start < prev.End {
					p = append(p, fmt.Sprintf("幕%s动作%s与%s在%s上时间重叠或先后顺序与声明顺序不一致", s.ID, a.ID, prev.ID, key))
				}
				lastTween[key] = sp
			}
		}
		span.End = cursor
		tl.Scenes = append(tl.Scenes, span)
	}
	tl.TotalMS = cursor
	if tl.TotalMS < animMinTotalMS || tl.TotalMS > animMaxTotalMS {
		tl.Warnings = append(tl.Warnings, fmt.Sprintf("成片时长%.1f秒，不在30秒–3分钟范围内；超过三分钟建议拆成多个知识点", float64(tl.TotalMS)/1000))
	}
	return tl, p
}

func animEstimatedProblems(sc AnimScript) []string {
	ms := map[string]int{}
	for _, scene := range sc.Scenes {
		for _, n := range scene.Narration {
			ms[n.ID] = estimateSpeechMS(n.Speech)
		}
	}
	_, problems := buildAnimTimeline(sc, ms)
	return problems
}
