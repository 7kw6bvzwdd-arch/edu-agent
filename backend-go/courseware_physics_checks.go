package main

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

type teachingCircuitValues struct {
	Connection string    `json:"connection"`
	Closed     bool      `json:"closed"`
	Resistance float64   `json:"total_resistance_ohm"`
	Current    float64   `json:"total_current_A"`
	Voltages   []float64 `json:"resistor_voltages_V"`
	Currents   []float64 `json:"resistor_currents_A"`
	Powers     []float64 `json:"resistor_powers_W"`
}

// Reuse the supported ideal circuit, not inferred wires from teacher answers.
func calculateTeachingCircuit(sl Slide) *teachingCircuitValues {
	d := sl.Scientific
	if d == nil || d.Kind != "circuit" || validateScientific(sl) != nil {
		return nil
	}
	v := &teachingCircuitValues{Connection: d.Connection, Closed: d.Closed == nil || *d.Closed}
	for _, r := range d.Resistances {
		if d.Connection == "series" {
			v.Resistance += r
		} else {
			v.Resistance += 1 / r
		}
	}
	if d.Connection == "parallel" {
		v.Resistance = 1 / v.Resistance
	}
	if v.Closed {
		v.Current = d.Voltage / v.Resistance
	}
	for _, r := range d.Resistances {
		i := v.Current
		if d.Connection == "parallel" && v.Closed {
			i = d.Voltage / r
		}
		v.Currents = append(v.Currents, i)
		v.Voltages = append(v.Voltages, i*r)
		v.Powers = append(v.Powers, i*i*r)
	}
	return v
}

const physicsNumber = `([0-9]+(?:\.[0-9]+)?)`

var physicsSupplyVoltage = regexp.MustCompile(`(?:电源|待测电压|被测电压)(?:低压档|电压|为|是|[：:=＝\s])*` + physicsNumber + `\s*V\b`)
var physicsReverseSupply = regexp.MustCompile(physicsNumber + `\s*V\s*(?:学生|直流|理想)?电源`)
var physicsObjectWeight = regexp.MustCompile(`(?:重力(?:为|是|G[=＝])?|物重(?:为|是)?|重)\s*` + physicsNumber + `\s*N\b`)
var physicsSelectedRange = regexp.MustCompile(`(?:量程(?:选|为|是|[：:=＝])?|(?:先)?(?:选用|使用|选择|选))\s*0\s*[—–－-]\s*` + physicsNumber + `\s*(V|N)\b`)
var physicsUniformRating = regexp.MustCompile(`额定功率\s*(均|都|各|每个)?\s*(?:为|是)?\s*` + physicsNumber + `\s*W\b`)
var physicsResistance = regexp.MustCompile(physicsNumber + `\s*Ω`)
var physicsExplicitWeight = regexp.MustCompile(`(?:\bG|重力|物重)\s*[=＝为]\s*` + physicsNumber + `\s*N\b`)
var physicsBuoyancyShares = regexp.MustCompile(`浮力分担了\s*` + physicsNumber + `\s*N(?:重力)?中的\s*` + physicsNumber + `\s*N`)
var physicsQuestionLabel = regexp.MustCompile(`([a-zA-Z])[.．、]`)
var physicsBalanceImpliesVolume = regexp.MustCompile(`(?:F浮=G|浮力等于重力|浮力与重力相等)(?:(?:就|可|能)?(?:推出|说明|因此|所以|则))?(?:漂浮)?[、，,]*(?:V排=V物|排液体积等于物体体积)`)

func singlePhysicsValue(pattern *regexp.Regexp, text string) (float64, bool) {
	values := pattern.FindAllStringSubmatch(text, -1)
	if len(values) == 0 {
		return 0, false
	}
	value, err := strconv.ParseFloat(values[0][1], 64)
	if err != nil || math.IsInf(value, 0) || value <= 0 {
		return 0, false
	}
	for _, match := range values[1:] {
		other, err := strconv.ParseFloat(match[1], 64)
		if err != nil || other != value {
			return 0, false // Multiple possible loads/voltages: do not bind by proximity.
		}
	}
	return value, true
}

func physicsClauses(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool { return strings.ContainsRune("。，；;\n", r) })
}

func physicsQuotedOrNegative(prefix string) bool {
	for _, term := range []string{"不能", "不可", "不允许", "禁止", "不得", "不要", "不用", "错误示例", "错误做法", "错解"} {
		if strings.Contains(prefix, term) {
			return true
		}
	}
	return false
}

func physicsActualExperiment(text string) bool {
	for _, clause := range physicsClauses(text) {
		if strings.Contains(clause, "若") || strings.Contains(clause, "如果") || strings.Contains(clause, "假设") || strings.Contains(clause, "理想") || strings.Contains(clause, "纸面") || physicsQuotedOrNegative(clause) {
			continue
		}
		for _, term := range []string{"真实实验", "实际实验", "操作方案", "实验方案", "演示要点", "演示操作"} {
			if strings.Contains(clause, term) {
				return true
			}
		}
	}
	return false
}

func teachingPhysicsConditionIssues(d Deck) []PageQualityIssue {
	issues := []PageQualityIssue{}
	add := func(object, evidence, repair string) {
		issues = append(issues, PageQualityIssue{Object: object, Reviewer: "content", Category: "conditions", Certainty: "confirmed", Severity: "error", Evidence: evidence, Repair: repair})
	}
	for _, sl := range d.Slides {
		// Only explicit affirmative implications in teaching explanations. Force
		// balance alone cannot establish immersion; other prose stays model-reviewed.
		for _, field := range []string{sl.Answer, sl.Notes} {
			for _, sentence := range strings.FieldsFunc(field, func(r rune) bool { return strings.ContainsRune("。；;\n", r) }) {
				normalized := strings.NewReplacer(" ", "", "＝", "=", "_", "", "{", "", "}", "", "\\text", "").Replace(sentence)
				for _, pos := range physicsBalanceImpliesVolume.FindAllStringIndex(normalized, -1) {
					if physicsQuotedOrNegative(normalized[:pos[0]]) || strings.Contains(normalized, "全浸没") || strings.Contains(normalized, "完全浸没") || strings.Contains(normalized, "错误说法") || strings.Contains(normalized, "？") || strings.Contains(normalized, "?") {
						continue
					}
					issues = append(issues, PageQualityIssue{Object: sentence, Reviewer: "content", Category: "knowledge", Certainty: "confirmed", Severity: "error", Evidence: "浮力等于重力只给出力的平衡，漂浮与悬浮均可能满足；排液体积等于物体体积还需要全浸没条件，不能由该平衡等式推出。", Repair: "只修正本页这段推论：区分受力平衡与浸入状态；全浸没才有V排=V物，漂浮通常V排<V物。保留原题、正确数值、其他页及答案隐藏方式。"})
				}
			}
		}
		fields := teachingSlideTextFields(sl)
		text := strings.Join(fields, "。")
		if weight, ok := singlePhysicsValue(physicsExplicitWeight, text); ok && sl.Layout != "exercise" {
			for _, field := range fields {
				for _, clause := range physicsClauses(field) {
					if physicsQuotedOrNegative(clause) || strings.Contains(clause, "错误说法") || strings.Contains(clause, "若") || strings.Contains(clause, "假设") {
						continue
					}
					for _, match := range physicsBuoyancyShares.FindAllStringSubmatch(clause, -1) {
						sharedWeight, _ := strconv.ParseFloat(match[1], 64)
						if math.Abs(sharedWeight-weight) > 1e-9*math.Max(1, weight) {
							issues = append(issues, PageQualityIssue{Object: match[0], Reviewer: "content", Category: "knowledge", Certainty: "confirmed", Severity: "error", Evidence: fmt.Sprintf("本页明确给出重力G=%.6g N。称重法中浮力分担的是总重力的一部分；原句把分担对象写为%.6g N，与给定总重力不一致。", weight, sharedWeight), Repair: "只修正这句力的对象说明：浮力与测力计拉力共同平衡物重，浮力分担总重力；保持已经正确的计算与其他页面。"})
						}
					}
				}
			}
		}
		voltage, hasVoltage := singlePhysicsValue(physicsSupplyVoltage, physicsReverseSupply.ReplaceAllString(text, "电源${1}V"))
		if sl.Layout != "exercise" && physicsActualExperiment(text) {
			parallel := sl.Scientific != nil && sl.Scientific.Kind == "circuit" && sl.Scientific.Connection == "parallel"
			parallel = parallel || strings.Contains(text, "两灯并联") || strings.Contains(text, "两电阻并联")
			weight, hasWeight := singlePhysicsValue(physicsObjectWeight, text)
			weighInAir := strings.Contains(text, "空气中称重") || strings.Contains(text, "空气中测重")
			for _, field := range fields {
				for _, clause := range physicsClauses(field) {
					for _, pos := range physicsSelectedRange.FindAllStringSubmatchIndex(clause, -1) {
						prefix := clause[:pos[0]]
						if physicsQuotedOrNegative(prefix) || strings.Contains(prefix, "若") || strings.Contains(prefix, "如果") || strings.Contains(prefix, "假设") {
							continue
						}
						maximum, err := strconv.ParseFloat(clause[pos[2]:pos[3]], 64)
						if err != nil || math.IsInf(maximum, 0) {
							continue
						}
						object, unit := clause[pos[0]:pos[1]], clause[pos[4]:pos[5]]
						if unit == "V" && hasVoltage && parallel && strings.Contains(field, "电压表") && maximum < voltage {
							add(object, fmt.Sprintf("本页明确并联且电源/待测电压为%g V，选用量程上限%g V，%g>%g；试触不能消除超量程。", voltage, maximum, voltage, maximum), "只修改本页电压表选用量程，使上限覆盖明确的待测电压；保留换接前断开开关，不把超量程试触写成安全。")
						}
						if unit == "N" && hasWeight && weighInAir && strings.Contains(field, "测力计") && maximum < weight {
							add(object, fmt.Sprintf("本页要求在空气中称重，物重%g N；所选测力计上限%g N，%g>%g，分度值不能替代量程。", weight, maximum, weight, maximum), "只修改测力计选用量程，使其覆盖空气中称重的完整负载；保留正确实验步骤和示意数据标记。")
						}
					}
				}
			}
		}
		if values := calculateTeachingCircuit(sl); values != nil && values.Closed {
			if hasVoltage && voltage != sl.Scientific.Voltage {
				continue // An actual experiment with a different source is not this model.
			}
			for _, field := range fields {
				for _, clause := range strings.FieldsFunc(field, func(r rune) bool { return strings.ContainsRune("。；;\n", r) }) {
					if !physicsActualExperiment(clause) {
						continue
					}
					if strings.Contains(clause, "限流") || strings.Contains(clause, "脉冲") || strings.Contains(clause, "占空") || strings.Contains(clause, "分压") || strings.Contains(clause, "滑动变阻器") {
						continue // These may change the actual operating point.
					}
					resistors := physicsResistance.FindAllStringSubmatch(clause, -1)
					mapped := true
					if len(resistors) > 0 {
						mapped = len(resistors) == len(sl.Scientific.Resistances)
						for i, resistor := range resistors {
							value, err := strconv.ParseFloat(resistor[1], 64)
							if i >= len(sl.Scientific.Resistances) || err != nil || value != sl.Scientific.Resistances[i] {
								mapped = false
							}
						}
					}
					if !mapped {
						continue // Explicitly different/unmapped components need review.
					}
					matches := physicsUniformRating.FindAllStringSubmatch(clause, -1)
					if len(matches) != 1 || (len(values.Powers) > 1 && matches[0][1] == "") {
						continue // Individual ratings need an explicit mapping, not a guess.
					}
					rating, err := strconv.ParseFloat(matches[0][2], 64)
					if err != nil || math.IsInf(rating, 0) {
						continue
					}
					for i, power := range values.Powers {
						if power > rating+1e-9*math.Max(1, power) {
							add(matches[0][0], fmt.Sprintf("按本页scientific实际连接计算：R%d=%.6g Ω，电流%.6g A，P=I²R=%.6g W，超过明确额定功率%.6g W。", i+1, sl.Scientific.Resistances[i], values.Currents[i], power, rating), "只修改本页真实实验方案：选择足够额定功率并检查限流和温升，或明确改为不实际接线的理想计算；保留正确电流答案。")
						}
					}
				}
			}
		}
		issues = append(issues, teachingUnsupportedCircuitAnswers(d, sl)...)
	}
	return issues
}

// A deliberately narrow guard for labeled questions with no wiring evidence.
// Actual diagrams, ambiguous/repeated labels and prior wiring conventions are
// left to the reviewer; arbitrary prose is not treated as a circuit netlist.
func teachingUnsupportedCircuitAnswers(d Deck, sl Slide) []PageQualityIssue {
	if sl.Layout != "exercise" || sl.Scientific != nil || sl.Picture != nil || sl.Figure != nil || sl.Table != nil || sl.Diagram != "" || sl.Graph != nil || sl.Chart != nil {
		return nil
	}
	if d.Plan != nil && physicsHasWiringCondition(d.Plan.Conventions) {
		return nil
	}
	if physicsHasWiringCondition(sl.Focus + " " + sl.Formula) {
		return nil
	}
	for _, bullet := range sl.Bullets {
		if strings.Contains(bullet, "所有") || strings.Contains(bullet, "各题") || strings.Contains(bullet, "约定") {
			if physicsHasWiringCondition(bullet) {
				return nil
			}
		}
	}
	visible := strings.Join(sl.Bullets, "；")
	labels := physicsQuestionLabel.FindAllStringSubmatchIndex(visible, -1)
	seen := map[string]int{}
	for _, pos := range labels {
		seen[strings.ToLower(visible[pos[2]:pos[3]])]++
	}
	issues := []PageQualityIssue{}
	for i, pos := range labels {
		label := strings.ToLower(visible[pos[2]:pos[3]])
		if seen[label] != 1 {
			continue
		}
		end := len(visible)
		if i+1 < len(labels) {
			end = labels[i+1][0]
		}
		question := visible[pos[1]:end]
		if !strings.Contains(question, "灯") || (!strings.Contains(question, "同时") && !strings.Contains(question, "并列")) {
			continue
		}
		wiring := strings.NewReplacer("未给连线方式", "", "未给连接方式", "", "未给连线", "", "不知道连接方式", "").Replace(question)
		known := false
		for _, term := range []string{"串联", "并联", "通路", "支路", "分支", "接在", "连接", "接成", "接到", "接入", "接于", "接电源", "接电池", "连线", "路径", "断丝", "断开", "熄灭", "仍亮", "照常", "互不影响", "电流", "电压", "电路图", "如图", "见图"} {
			known = known || strings.Contains(wiring, term)
		}
		if known {
			continue
		}
		answer := regexp.MustCompile(`(?i)(?:^|[；;。\s①②③④⑤⑥])` + label + `\s*[.．、:：]?\s*(?:为|是|应为|属于|判断为)?\s*(串联|并联)`)
		if pos := answer.FindStringIndex(sl.Answer); pos != nil {
			if physicsQuotedOrNegative(sl.Answer[:pos[0]]) {
				continue
			}
			match := sl.Answer[pos[0]:pos[1]]
			issues = append(issues, PageQualityIssue{Object: label + "题：" + strings.TrimSpace(question), Reviewer: "content", Category: "conditions", Certainty: "confirmed", Severity: "error", Evidence: "本小题仅给同时亮/灯的位置并列，没有实际连接或电流路径；教师答案“" + strings.TrimSpace(match) + "”不能倒填题设，同时亮不排除串联或并联。", Repair: "只修改该小题的答案与解释为信息不足、须看实际连接和电流路径，或补有依据的可见连线条件；不从原答案编造连接，保留其他题。"})
		}
	}
	return issues
}

func physicsHasWiringCondition(text string) bool {
	for _, term := range []string{"两灯串联", "两灯并联", "两盏灯串联", "两盏灯并联", "依次接成", "接在电源", "接到电源", "连接在电源", "相同两个节点", "同一对节点", "各自形成通路", "只有一条电流路径", "所有电路均为并联", "所有电路均为串联"} {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}
