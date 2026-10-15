package main

import (
	"regexp"
	"strconv"
	"strings"
)

var volumeErrorClaim = regexp.MustCompile(`(?:结果|实际|正确)[^。；;]{0,8}?(放大|缩小|大|小)([0-9]+(?:\.[0-9]+)?)倍`)

// This is deliberately limited to explicit "unconverted volume directly used
// in SI" warnings. It is not a parser for arbitrary natural-language physics.
func teachingVolumeErrorFactors(d Deck) ([]PageQualityIssue, []string) {
	issues := []PageQualityIssue{}
	correct := []string{}
	for _, sl := range d.Slides {
		for _, sentence := range strings.FieldsFunc(strings.Join(sl.Bullets, "。")+"。"+sl.Notes+"。"+sl.Answer, func(r rune) bool { return r == '。' || r == '\n' }) {
			if len([]rune(sentence)) > 750 {
				continue
			}
			normalized := strings.NewReplacer("cm^3", "cm³", "cm3", "cm³", "mm^3", "mm³", "mm3", "mm³", "10⁶", "1000000", "10^6", "1000000", "10⁹", "1000000000", "10^9", "1000000000", "一百万", "1000000", "百万", "1000000", "十亿", "1000000000").Replace(sentence)
			if !strings.Contains(normalized, "未换算") || !strings.Contains(normalized, "直接代入") || !strings.Contains(strings.ToUpper(normalized), "SI") {
				continue
			}
			// An arbitrary SI formula may divide by V (density=m/V) or square
			// it. Require the supported buoyancy formula, not a topic keyword.
			formula := strings.NewReplacer("×", "", "·", "", "*", "").Replace(visualFormulaContent(sl.Formula))
			switch formula {
			case "F浮=ρ液gV排", "F浮=ρ水gV排", "F浮=ρgV排", "F浮=ρgV", "F=ρgV":
			default:
				continue
			}
			if strings.Contains(sentence, "密度") || strings.Contains(sentence, "m/V") {
				continue
			}
			factor := float64(0)
			if strings.Contains(normalized, "cm³") && !strings.Contains(normalized, "mm³") {
				factor = 1e6
			}
			if strings.Contains(normalized, "mm³") && !strings.Contains(normalized, "cm³") {
				factor = 1e9
			}
			claims := volumeErrorClaim.FindAllStringSubmatch(normalized, -1)
			if factor == 0 || len(claims) == 0 {
				continue
			}
			// When an explicitly quoted wrong method is corrected in the same sentence,
			// verify the correction, not the deliberately displayed wrong claim.
			claim := claims[0]
			if strings.Contains(normalized, "错误说法") && strings.Contains(normalized, "实际") {
				claim = claims[len(claims)-1]
			}
			value, err := strconv.ParseFloat(claim[2], 64)
			if err != nil {
				continue
			}
			if (claim[1] == "大" || claim[1] == "放大") && value == factor {
				correct = append(correct, sentence)
				continue
			}
			multiplier := strconv.FormatFloat(factor, 'f', 0, 64)
			unit, conversion := "cm³", "10⁻⁶"
			if factor == 1e9 {
				unit, conversion = "mm³", "10⁻⁹"
			}
			issues = append(issues, PageQualityIssue{Object: sentence, Category: "answer", Certainty: "confirmed", Severity: "error", Reviewer: "content", Evidence: "1 " + unit + "=" + conversion + " m³；未换算直接代入SI公式，错误值/正确值=" + multiplier + "，结果放大" + multiplier + "倍。", Repair: "只修正该未换算错误的倍率与方向为放大" + multiplier + "倍，保留正确题设、公式及最终答案。"})
		}
	}
	return issues, correct
}
