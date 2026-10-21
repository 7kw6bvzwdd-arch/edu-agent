package main

import (
	"regexp"
	"strconv"
	"strings"
)

func (s *Server) reviewPolicy() string {
	if s.cfg.CoursewareReviewPolicy == "" {
		return "all"
	} // Existing callers/tests keep their explicit legacy policy.
	return s.cfg.CoursewareReviewPolicy
}
func importantTeachingPage(d Deck) bool {
	for _, sl := range d.Slides {
		if sl.Layout == "example" || sl.Layout == "exercise" || sl.Answer != "" || sl.Formula != "" {
			return true
		}
		text := strings.Join(teachingSlideTextFields(sl), " ")
		for _, word := range []string{"推导", "证明", "定理", "结论", "史实", "公元", "年", "化学方程", "实验", "安全", "译文", "原文", "引用", "因此", "意味着", "由此"} {
			if strings.Contains(text, word) {
				return true
			}
		}

	}
	return false
}

type subjectCheck func(Deck) []PageQualityIssue

var subjectCheckers = map[string][]subjectCheck{
	"数学": {exerciseVisibilityIssues},
	"物理": {exerciseVisibilityIssues},
	"化学": {chemicalBalanceIssues},
	"历史": {historyTimelineIssues},
}

func subjectContentIssues(in TaskInput, d Deck) []PageQualityIssue {
	issues := append(teachingDeckArithmeticIssues(d), missingPictureTaskIssues(d)...)
	issues = append(issues, singleCellTaskIssues(d)...)
	for subject, checks := range subjectCheckers {
		if strings.Contains(in.Subject, subject) {
			for _, check := range checks {
				issues = append(issues, check(d)...)
			}
			break
		}
	}
	return issues
}

var chemicalEquation = regexp.MustCompile(`(?:[0-9]*[A-Z][A-Za-z0-9()]*[[:space:]]*[+][[:space:]]*)*[0-9]*[A-Z][A-Za-z0-9()]*[[:space:]]*(?:=|→|->)[[:space:]]*(?:[0-9]*[A-Z][A-Za-z0-9()]*[[:space:]]*[+][[:space:]]*)*[0-9]*[A-Z][A-Za-z0-9()]*`)
var chemicalAtom = regexp.MustCompile(`[A-Z][a-z]?[0-9]*`)
var coefficient = regexp.MustCompile(`^([0-9]+)?(.*)$`)

// Unsupported ions, isotopes and grouping are left to semantic review. Never
// label a partially parsed equation balanced. Simple molecular equations only.
func chemicalCounts(side string) (map[string]int, bool) {
	out := map[string]int{}
	for _, term := range strings.Split(side, "+") {
		parts := coefficient.FindStringSubmatch(strings.TrimSpace(term))
		if len(parts) != 3 || strings.ContainsAny(parts[2], "()") {
			return nil, false
		}
		n := 1
		if parts[1] != "" {
			n, _ = strconv.Atoi(parts[1])
			if n < 1 || n > 10000 {
				return nil, false
			}
		}
		atoms := chemicalAtom.FindAllString(parts[2], -1)
		if strings.Join(atoms, "") != parts[2] {
			return nil, false
		}
		for _, atom := range atoms {
			split := len(atom)
			for split > 0 && atom[split-1] >= '0' && atom[split-1] <= '9' {
				split--
			}
			k := 1
			if split < len(atom) {
				k, _ = strconv.Atoi(atom[split:])
				if k < 1 || k > 10000 {
					return nil, false
				}
			}
			out[atom[:split]] += n * k
		}
	}
	return out, len(out) > 0
}
func chemicalBalanceIssues(d Deck) []PageQualityIssue {
	issues := []PageQualityIssue{}
	for _, sl := range d.Slides {
		for _, field := range teachingSlideTextFields(sl) {
			field = strings.NewReplacer("₀", "0", "₁", "1", "₂", "2", "₃", "3", "₄", "4", "₅", "5", "₆", "6", "₇", "7", "₈", "8", "₉", "9").Replace(field)
			if strings.Contains(field, "请配平") || strings.Contains(field, "待配平") || strings.Contains(field, "未配平") || strings.Contains(field, "错误示例") || strings.Contains(field, "错误写法") {
				continue
			}
			for _, index := range chemicalEquation.FindAllStringIndex(field, -1) {
				equation := field[index[0]:index[1]]
				// Ions and state annotations must not be accepted as a complete molecule.
				suffix := strings.TrimSpace(field[index[1]:])
				if strings.HasPrefix(suffix, "^") || strings.HasPrefix(suffix, "+") || strings.HasPrefix(suffix, "-") {
					continue
				}
				sides := regexp.MustCompile(`=|→|->`).Split(equation, -1)
				if len(sides) != 2 {
					continue
				}
				left, ok := chemicalCounts(sides[0])
				right, ok2 := chemicalCounts(sides[1])
				if !ok || !ok2 {
					continue
				}
				balanced := len(left) == len(right)
				for atom, n := range left {
					balanced = balanced && right[atom] == n
				}
				if !balanced {
					issues = append(issues, PageQualityIssue{Object: equation, Reviewer: "content", Category: "knowledge", Certainty: "confirmed", Severity: "error", Evidence: "反应前后元素原子数不相等", Repair: "核对物质化学式和化学计量数，保持题设和反应条件"})
				}
			}
		}
	}
	return issues
}

var historyYear = regexp.MustCompile(`^(?:公元)?(前)?[[:space:]]*([0-9]{1,4})年`)

func historyTimelineIssues(d Deck) []PageQualityIssue {
	issues := []PageQualityIssue{}
	for _, sl := range d.Slides {
		if sl.Figure == nil || sl.Figure.Kind != "timeline" || strings.Contains(sl.Figure.Title, "倒序") || strings.Contains(sl.Figure.Title, "逆序") {
			continue
		}
		previous := -100000
		for _, item := range sl.Figure.Items {
			m := historyYear.FindStringSubmatch(strings.TrimSpace(item.Label))
			if len(m) != 3 {
				previous = -100000
				continue
			}
			year, _ := strconv.Atoi(m[2])
			if m[1] != "" {
				year = -year
			}
			if year < previous {
				issues = append(issues, PageQualityIssue{Object: sl.Figure.Title, Reviewer: "content", Category: "consistency", Certainty: "confirmed", Severity: "error", Evidence: "按时间顺序展示的节点年份发生倒退（公元前按负年份比较）", Repair: "核对事件年代，将节点按时间先后排列；需要倒序时明确标注"})
			}
			previous = year
		}
	}
	return issues
}
