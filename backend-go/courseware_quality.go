package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strings"
	"unicode"
)

// A narrow deterministic guard for the observed category-only homework page.
// It does not try to prove arbitrary questions complete; diagrams and genuine
// prose still need the independent content reviewer.
func exerciseVisibilityIssues(d Deck) []PageQualityIssue {
	strip := strings.NewReplacer("两点法", "", "求解析式", "", "判断", "", "读图", "", "作图", "", "分层", "", "作业", "", "基础题", "", "巩固", "", "提升", "", "必做", "", "选做", "", "组", "", "由两点", "", "与", "", "A", "", "B", "", "C", "", "Ａ", "", "Ｂ", "", "Ｃ", "")
	issues := exerciseCoordinateDisclosureIssues(d)
	issues = append(issues, exerciseVertexFormulaDisclosureIssues(d)...)
	issues = append(issues, exerciseExplicitSolutionIssues(d)...)
	for _, sl := range d.Slides {
		if sl.Layout != "exercise" || len(sl.Bullets) == 0 || sl.Graph != nil || sl.Scientific != nil || sl.Table != nil || sl.Figure != nil || sl.Chart != nil || sl.Picture != nil || sl.Diagram != "" || sl.Formula != "" {
			continue
		}
		classificationOnly := true
		for _, b := range sl.Bullets {
			remaining := strings.Map(func(r rune) rune {
				if unicode.IsSpace(r) || unicode.IsPunct(r) {
					return -1
				}
				return r
			}, strip.Replace(b))
			if remaining != "" {
				classificationOnly = false
				break
			}
		}
		if classificationOnly {
			issues = append(issues, PageQualityIssue{Object: sl.Title + "的可见题目", Reviewer: "content", Category: "pedagogy", Certainty: "confirmed", Severity: "error", Evidence: "可见正文只有作业分组、必做/选做或判断作图的分类，没有具体题设与任务；备注不能替代学生可见题目。", Repair: "将备注中已有的具体条件、数值与问题移入本页正文，保持答案仅在备注。"})
		}
	}
	return issues
}

// Explicit standalone solutions are unambiguous; arbitrary mathematical
// expressions and supplied answers to be evaluated still need semantic review.
func exerciseExplicitSolutionIssues(d Deck) []PageQualityIssue {
	issues := []PageQualityIssue{}
	explicit := regexp.MustCompile(`^(?:答案|解答|解)[：:]\s*\S`)
	for _, sl := range d.Slides {
		if sl.Layout != "exercise" {
			continue
		}
		fields := append([]string{sl.Title, sl.Focus, sl.Formula}, sl.Bullets...)
		if sl.Table != nil {
			fields = append(fields, sl.Table.Headers...)
			for _, row := range sl.Table.Rows {
				fields = append(fields, row...)
			}
		}
		for _, field := range fields {
			for _, line := range strings.Split(field, "\n") {
				if explicit.MatchString(strings.TrimSpace(line)) {
					issues = append(issues, PageQualityIssue{Object: line, Reviewer: "content", Category: "pedagogy", Certainty: "confirmed", Severity: "error", Evidence: "独立练习的学生可见字段直接显示以答案或解答开头的解答。", Repair: "将这条解答移到answer，每行一个讲解步骤；保留题设、数值、给定读图对象及其余页面。"})
				}
			}
		}
	}
	return issues
}

var completedSquareFormula = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]+)?\*?)?\(x[+-][0-9]+(?:\.[0-9]+)?\)\^2(?:[+-][0-9]+(?:\.[0-9]+)?)?$`)
var completingSquareTask = regexp.MustCompile(`配方|化为顶点式|求[^。！？?]{0,12}(?:顶点|对称轴|最小值|最大值)`)

// Recognize the observed completed-square solution appended to the given
// polynomial. Do not hide all exercise formulas: given vertex forms, identity
// verification, worked examples and other algebra stay independently reviewed.
func exerciseVertexFormulaDisclosureIssues(d Deck) []PageQualityIssue {
	issues := []PageQualityIssue{}
	normalize := func(text string) string {
		return strings.NewReplacer("²", "^2", "^{2}", "^2", "−", "-", "（", "(", "）", ")").Replace(visualFormulaContent(text))
	}
	for _, sl := range d.Slides {
		problem := normalize(strings.Join(sl.Bullets, "。"))
		parts := strings.Split(normalize(sl.Formula), "=")
		if sl.Layout != "exercise" || !completingSquareTask.MatchString(problem) || len(parts) < 3 || parts[0] != "y" {
			continue
		}
		given, derived := parts[1], parts[len(parts)-1]
		if !strings.Contains(given, "x^2") || !strings.Contains(problem, "y="+given) || !completedSquareFormula.MatchString(derived) || strings.Contains(problem, derived) {
			continue
		}
		issues = append(issues, PageQualityIssue{Object: sl.Formula, Reviewer: "content", Category: "pedagogy", Certainty: "confirmed", Severity: "error", Evidence: "正文要求学生用配方求顶点等结果，但可见公式已在题设多项式后给出配方后的顶点式答案，学生独立练习前就能读到解题结果。", Repair: "只将公式栏恢复为正文已给的原始函数式；配方过程及顶点式保留在answer或教师notes，保留题目、参数、图像和其他页面。"})
	}
	return issues
}

var readCoordinateTask = regexp.MustCompile(`(?:读出|求出|求|确定)[^。！？?]{0,12}坐标`)
var focusIntersectionCoordinate = regexp.MustCompile(`交点(?:坐标)?(?:为|是)?\(([+-]?[0-9]+(?:\.[0-9]+)?(?:/[0-9]+)?),([+-]?[0-9]+(?:\.[0-9]+)?(?:/[0-9]+)?)\)`)

// Catch the observed read-intersection task whose visible focus prints the
// actual answer. Do not infer arbitrary answers, hide supplied graphs, or
// rewrite a teacher's question. Other disclosures remain model-reviewed.
func exerciseCoordinateDisclosureIssues(d Deck) []PageQualityIssue {
	issues := []PageQualityIssue{}
	normalize := func(text string) string {
		text = strings.NewReplacer("（", "(", "）", ")", "，", ",", "−", "-").Replace(text)
		return strings.Join(strings.Fields(text), "")
	}
	for _, sl := range d.Slides {
		if sl.Layout != "exercise" || sl.Graph == nil || len(sl.Graph.Series) != 2 {
			continue
		}
		problem := normalize(strings.Join(sl.Bullets, "。"))
		if !strings.Contains(problem, "交") || !readCoordinateTask.MatchString(problem) {
			continue
		}
		match := focusIntersectionCoordinate.FindStringSubmatch(normalize(sl.Focus))
		if len(match) != 3 || strings.Contains(problem, "("+match[1]+","+match[2]+")") {
			continue
		}
		first, second := sl.Graph.Series[0], sl.Graph.Series[1]
		if (first.Kind != "" && first.Kind != "linear") || (second.Kind != "" && second.Kind != "linear") || first.K == second.K {
			continue
		}
		x := (second.B - first.B) / (first.K - second.K)
		y := first.K*x + first.B
		xAnswer, xOK := new(big.Rat).SetString(match[1])
		yAnswer, yOK := new(big.Rat).SetString(match[2])
		if !xOK || !yOK {
			continue
		}
		xGiven, _ := xAnswer.Float64()
		yGiven, _ := yAnswer.Float64()
		if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) || math.Abs(x-xGiven) > 1e-9 || math.Abs(y-yGiven) > 1e-9 {
			continue
		}
		issues = append(issues, PageQualityIssue{Object: sl.Focus, Reviewer: "content", Category: "pedagogy", Certainty: "confirmed", Severity: "error", Evidence: "正文要求学生读出交点坐标，但上屏提示已直接给出两条实际直线交点的坐标答案，练习尚未完成就能读到答案。", Repair: "只将提示中的答案坐标移到answer或教师notes，保留不含答案的追问、可见题设及给定读图图像；不改函数参数，不重做其他页。"})
	}
	return issues
}

// Only the observed explicit count/continuous-segment contradiction is handled
// here. Other domains and general mathematical modeling remain model-reviewed.
func teachingDiscreteCountIssues(d Deck) []PageQualityIssue {
	issues := []PageQualityIssue{}
	for _, sl := range d.Slides {
		text := strings.ReplaceAll(strings.Join(teachingSlideTextFields(sl), "。"), " ", "")
		if !strings.Contains(text, "x是使用次数") && !strings.Contains(text, "x表示使用次数") {
			continue
		}
		for _, sentence := range strings.FieldsFunc(text, func(r rune) bool { return r == '。' || r == '\n' || r == '；' }) {
			if strings.Contains(sentence, "错误") || strings.Contains(sentence, "吗") || strings.Contains(sentence, "？") || strings.Contains(sentence, "离散") {
				continue
			}
			if regexp.MustCompile(`图[象像]是[^。]{0,100}(?:一条|的)?线段`).MatchString(sentence) {
				issues = append(issues, PageQualityIssue{Object: sentence, Reviewer: "content", Category: "knowledge", Certainty: "confirmed", Severity: "error", Evidence: "本页明确x是使用次数，只能取非负整数；把实际图象断言为连续线段会包含不可取的非整数次数。", Repair: "保留题设和解析式，明确整数取值，图象表述改为对应的离散点；如果介绍连续延拓，须明确区分实际情境和延拓模型。"})
			}
		}
	}
	return issues
}

// These checks catch thin or repetitive slide structure, not factual correctness.
func coursewareQualityIssues(d Deck, teachingPages int) []string {
	issues := []string{}
	titles, bodies, layouts := map[string]int{}, map[string]int{}, map[string]bool{}
	examples, exercises := 0, 0
	for i, sl := range d.Slides {
		page := i + 1
		// The renderer enforces actual fit and readable type. A fixed bullet
		// count would rewrite already checked pages whose short questions fit.
		layout := sl.Layout
		if layout != "" {
			layouts[layout] = true
		}
		if layout == "example" {
			examples++
		}
		if layout == "exercise" {
			exercises++
		}
		title := strings.ToLower(strings.Join(strings.Fields(sl.Title), ""))
		if prior := titles[title]; prior > 0 {
			issues = append(issues, fmt.Sprintf("第%d页与第%d页重复标题", page, prior))
		}
		titles[title] = page
		if len(sl.Bullets) == 0 {
			issues = append(issues, fmt.Sprintf("第%d页正文为空，请补具体课堂内容", page))
		}
		body := make([]string, len(sl.Bullets))
		for j, bullet := range sl.Bullets {
			body[j] = strings.ToLower(strings.Join(strings.Fields(bullet), ""))
			if body[j] == "" {
				issues = append(issues, fmt.Sprintf("第%d页含空白正文要点", page))
			}
		}
		joined := strings.Join(body, "|")
		if prior := bodies[joined]; prior > 0 {
			issues = append(issues, fmt.Sprintf("第%d页与第%d页重复正文", page, prior))
		}
		bodies[joined] = page
		if i > 0 && len([]rune(strings.TrimSpace(sl.Notes))) < 25 {
			issues = append(issues, fmt.Sprintf("第%d页讲解备注过少，请写出依据、提问或答案", page))
		}
	}
	if len(layouts) == 1 && len(d.Slides) >= 8 {
		issues = append(issues, "整套课件版式重复，请核对教学任务是否采用合适表达，不为凑版式拆页")
	}
	if examples == 0 {
		issues = append(issues, "缺少例题解析页面")
	}
	if exercises == 0 {
		issues = append(issues, "缺少课堂练习页面")
	}
	// Layout continuations must not add new teaching requirements.
	if d.Plan == nil && teachingPages >= 12 && (examples < 2 || exercises < 2) {
		issues = append(issues, "12页及以上课件应包含至少两道递进例题和两道独立练习")
	}
	return issues
}

func coursewareFunctionIssues(in TaskInput, d Deck) []string {
	if !strings.Contains(in.Topic, "一次函数") {
		return nil
	}
	graphs, comparison := 0, false
	for _, sl := range d.Slides {
		if sl.Graph != nil {
			graphs++
			comparison = comparison || len(sl.Graph.Series) > 1
		}
	}
	issues := []string{}
	if graphs < 2 {
		issues = append(issues, "缺少函数图像：至少2页使用graph真实数值坐标图，分别支撑作图和性质解释")
	}
	if !comparison {
		issues = append(issues, "缺少函数图像对比：至少一页graph展示两条以上直线，使用相同坐标比例比较k或b")
	}
	return issues
}

func (s *Server) coursewarePreflight(ctx context.Context, d *Deck, teachingPages int) ([]string, error) {
	issues := coursewareQualityIssues(*d, teachingPages)
	// Invalid media must reach the same bounded draft repair as text overflow.
	if err := validateDeck(*d); err != nil {
		return append(issues, err.Error()), nil
	}
	if _, err := s.renderCourseware(ctx, d); err != nil {
		var validation APIError
		if errors.As(err, &validation) && validation.Status == 422 {
			return append(issues, validation.Message), nil
		}
		return nil, err
	}
	return issues, nil
}
