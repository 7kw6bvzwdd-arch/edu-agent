package main

import (
	"sort"
	"strings"
	"unicode"
)

// Scores describe lexical selection, never factual correctness or calibrated confidence.
type PassageFilter struct {
	Method    string   `json:"method"`
	Total     int      `json:"total"`
	Kept      int      `json:"kept"`
	Removed   int      `json:"removed"`
	Matched   []string `json:"matched"`
	Truncated bool     `json:"truncated"`
}

func relevanceTerms(q string) []string {
	// These lesson suffixes describe teaching intent, not the subject to retrieve.
	for _, suffix := range []string{"的背景与影响", "的背景和影响", "的图象和性质", "的图像和性质"} {
		q = strings.TrimSuffix(strings.TrimSpace(q), suffix)
	}
	for _, stop := range []string{"七年级", "八年级", "九年级", "初中", "人教版", "数学", "物理", "教学", "课件", "动画", "制作", "生成"} {
		q = strings.ReplaceAll(q, stop, " ")
	}
	seen := map[string]bool{}
	out := []string{}
	for _, term := range terms(q) {
		if !seen[term] {
			seen[term] = true
			out = append(out, term)
		}
	}
	return out
}

func passageNoise(p string) bool {
	for _, phrase := range []string{"点击下载", "下载app", "扫码关注", "扫码领取", "登录后查看", "会员专享", "相关推荐", "猜你喜欢", "版权所有", "广告合作"} {
		if strings.Contains(strings.ToLower(p), phrase) {
			return true
		}
	}
	return false
}

func supportingPassage(p string) bool {
	for _, marker := range []string{"=", "＝", "公式", "其中", "式中", "适用", "前提", "条件", "仅当", "不适用", "不能", "不随", "完全浸没", "注意", "安全", "防止", "图注", "图示", "单位"} {
		if strings.Contains(p, marker) {
			return true
		}
	}
	return false
}

func relevantPassages(query, body string, budget int) (string, *PassageFilter) {
	// ponytail: lexical rules can miss synonyms; use a calibrated semantic filter when recall data justifies it.
	info := &PassageFilter{Method: "lexical-context-v1", Matched: []string{}}
	keys := relevanceTerms(query)
	// Preserve sentence boundaries and symbols. No NFKC/unit conversion that could change equations.
	var paragraphs []string
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var part strings.Builder
		for _, r := range line {
			part.WriteRune(r)
			if (r == '。' || r == '！' || r == '？' || r == ';' || r == '；') && part.Len() > 600 {
				paragraphs = append(paragraphs, part.String())
				part.Reset()
			}
		}
		if part.Len() > 0 {
			paragraphs = append(paragraphs, part.String())
		}
	}
	info.Total = len(paragraphs)
	scores := make([]int, len(paragraphs))
	noise := make([]bool, len(paragraphs))
	seen := map[string]bool{}
	matched := map[string]bool{}
	for i, p := range paragraphs {
		normalized := strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return unicode.ToLower(r)
		}, p)
		noise[i] = passageNoise(p) || seen[normalized]
		seen[normalized] = true
		if noise[i] {
			continue
		}
		low := strings.ToLower(p)
		for _, key := range keys {
			if strings.Contains(low, key) {
				scores[i]++
				matched[key] = true
			}
		}
	}
	for _, key := range keys {
		if matched[key] {
			info.Matched = append(info.Matched, key)
		}
	}
	order := make([]int, 0)
	for i, score := range scores {
		if score > 0 {
			order = append(order, i)
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return scores[order[i]] > scores[order[j]] })
	kept := map[int]bool{}
	used := 0
	for _, i := range order {
		if kept[i] {
			continue
		}
		group := []int{i}
		// Keep adjacent formula/conditions/safety as one evidence group; never lone unsafe excerpts.
		for j := i - 1; j >= 0 && j >= i-2 && !noise[j] && supportingPassage(paragraphs[j]); j-- {
			group = append(group, j)
		}
		for j := i + 1; j < len(paragraphs) && j <= i+3 && !noise[j] && supportingPassage(paragraphs[j]); j++ {
			group = append(group, j)
		}
		cost := 0
		for _, j := range group {
			if !kept[j] {
				cost += len([]rune(paragraphs[j])) + 1
			}
		}
		if used+cost > budget {
			info.Truncated = true
			continue
		}
		for _, j := range group {
			kept[j] = true
		}
		used += cost
	}
	out := []string{}
	for i, p := range paragraphs {
		if kept[i] {
			out = append(out, p)
		}
	}
	info.Kept = len(out)
	info.Removed = info.Total - info.Kept
	return strings.Join(out, "\n"), info
}
