package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math/big"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var teachingNumericChain = regexp.MustCompile(`[0-9.+*/()= \t×÷−-]+`)
var teachingNumericPower = regexp.MustCompile(`(?:\([+-]?[0-9]+(?:\.[0-9]+)?\)|[0-9]+(?:\.[0-9]+)?)[²³]`)

// Only explicit scalar arithmetic is evaluated. Variables, TeX, general powers,
// percentages, unit-bearing operands and approximations stay with the reviewer.
// Literal squares/cubes are reduced first; symbolic/general powers stay outside
// this check. ParseExpr builds syntax; model response text is never executed.
func teachingArithmeticIssues(text string) []PageQualityIssue {
	text = strings.NewReplacer("×", "*", "·", "*", "÷", "/", "−", "-", "（", "(", "）", ")", "＝", "=").Replace(text)
	text = teachingNumericPower.ReplaceAllStringFunc(text, func(power string) string {
		chars := []rune(power)
		base := strings.Trim(string(chars[:len(chars)-1]), "()")
		if len(base) > 40 {
			return power
		}
		n, ok := new(big.Rat).SetString(base)
		if !ok {
			return power
		}
		value := new(big.Rat).Mul(n, n)
		if chars[len(chars)-1] == '³' {
			value.Mul(value, n)
		}
		result, exact := teachingFiniteDecimal(value)
		if !exact {
			return power
		}
		return "(" + result + ")"
	})
	issues := []PageQualityIssue{}
	for _, clause := range strings.FieldsFunc(text, func(r rune) bool { return strings.ContainsRune("。，；;\n", r) }) {
		for _, span := range teachingNumericChain.FindAllStringIndex(clause, -1) {
			chain := strings.TrimSpace(clause[span[0]:span[1]])
			if !strings.Contains(chain, "=") || len(chain) > 200 {
				continue
			}
			// A labeled wrong example is teaching material, not an asserted answer.
			prefix := clause[:span[0]]
			if strings.Contains(prefix, "错误示例") || strings.Contains(prefix, "错误做法") || strings.Contains(prefix, "错解") || strings.Contains(prefix, "错误算式") || strings.Contains(prefix, "误算") || strings.Contains(prefix, "错误计算") {
				continue
			}
			if span[0] > 0 {
				prev, _ := utf8.DecodeLastRuneInString(prefix)
				if prev == ':' && len(prefix) > 1 && prefix[len(prefix)-2] >= '0' && prefix[len(prefix)-2] <= '9' {
					continue // Ratio notation, not an independent left-hand operand.
				}
				if numericBoundaryUnsupported(prev) || (unicode.IsLetter(prev) && !unicode.Is(unicode.Han, prev) && !strings.HasPrefix(chain, "=")) {
					continue
				}
			}
			if span[1] < len(clause) {
				next, _ := utf8.DecodeRuneInString(clause[span[1]:])
				if numericBoundaryUnsupported(next) || (unicode.IsLetter(next) && !unicode.Is(unicode.Han, next) && !teachingNumericUnitSuffix(clause[span[1]:])) {
					continue
				}
			}
			parts := strings.Split(strings.TrimPrefix(chain, "="), "=")
			if len(parts) < 2 || len(parts) > 8 {
				continue
			}
			values := make([]*big.Rat, len(parts))
			valid := true
			for i, part := range parts {
				expr, err := parser.ParseExpr(strings.TrimSpace(part))
				if err != nil {
					valid = false
					break
				}
				values[i] = teachingNumericValue(expr)
				if values[i] == nil {
					valid = false
					break
				}
			}
			if !valid {
				continue
			}
			for i := 1; i < len(values); i++ {
				// Recurring decimals may be classroom rounding. Do not turn their
				// omitted approximation sign into a regeneration loop.
				want, exact := teachingFiniteDecimal(values[i-1])
				_, rightExact := teachingFiniteDecimal(values[i])
				if !exact || !rightExact || values[i-1].Cmp(values[i]) == 0 {
					continue
				}
				issues = append(issues, PageQualityIssue{Object: chain, Reviewer: "content", Category: "answer", Certainty: "confirmed", Severity: "error", Evidence: fmt.Sprintf("程序按四则运算核对：%s=%s，与等号后%s不相等。", strings.TrimSpace(parts[i-1]), want, strings.TrimSpace(parts[i])), Repair: "只修正此算式的错误结果及依赖该结果的解释，保留正确题设和其他页面；同时核对单位。"})
				break
			}
		}
	}
	return issues
}

func numericBoundaryUnsupported(r rune) bool {
	return strings.ContainsRune(",^_%≈≃≠≤≥<>⁰¹²³⁴⁵⁶⁷⁸⁹⁻⁺{}[]\\", r)
}

func teachingNumericUnitSuffix(text string) bool {
	word := strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) || unicode.Is(unicode.Han, r) })
	if len(word) == 0 {
		return false
	}
	return strings.Contains("|N|W|A|V|J|Ω|Pa|kg|g|m|cm|mm|s|", "|"+word[0]+"|")
}

func teachingNumericValue(expr ast.Expr) *big.Rat {
	switch v := expr.(type) {
	case *ast.BasicLit:
		if v.Kind != token.INT && v.Kind != token.FLOAT {
			return nil
		}
		if strings.ContainsAny(v.Value, "eEpPxXoObB_") {
			return nil
		}
		if v.Kind == token.INT {
			if n, ok := new(big.Int).SetString(v.Value, 10); ok {
				return new(big.Rat).SetInt(n)
			}
		}
		n, _ := new(big.Rat).SetString(v.Value)
		return n
	case *ast.ParenExpr:
		return teachingNumericValue(v.X)
	case *ast.UnaryExpr:
		n := teachingNumericValue(v.X)
		if n != nil && v.Op == token.SUB {
			return new(big.Rat).Neg(n)
		}
		if v.Op == token.ADD {
			return n
		}
	case *ast.BinaryExpr:
		a, b := teachingNumericValue(v.X), teachingNumericValue(v.Y)
		if a == nil || b == nil {
			return nil
		}
		n := new(big.Rat)
		switch v.Op {
		case token.ADD:
			return n.Add(a, b)
		case token.SUB:
			return n.Sub(a, b)
		case token.MUL:
			return n.Mul(a, b)
		case token.QUO:
			if b.Sign() != 0 {
				return n.Quo(a, b)
			}
		}
	}
	return nil
}

func teachingFiniteDecimal(n *big.Rat) (string, bool) {
	denominator := new(big.Int).Set(n.Denom())
	places := 0
	for _, factor := range []int64{2, 5} {
		count := 0
		divisor := big.NewInt(factor)
		for new(big.Int).Mod(denominator, divisor).Sign() == 0 {
			denominator.Quo(denominator, divisor)
			count++
		}
		if count > places {
			places = count
		}
	}
	if denominator.Cmp(big.NewInt(1)) != 0 || places > 24 {
		return "", false
	}
	return n.FloatString(places), true
}

func teachingDeckArithmeticIssues(d Deck) []PageQualityIssue {
	issues := []PageQualityIssue{}
	for _, sl := range d.Slides {
		for _, field := range teachingSlideTextFields(sl) {
			issues = append(issues, teachingArithmeticIssues(field)...)
		}
	}
	return issues
}

func teachingSlideTextFields(sl Slide) []string {
	fields := []string{sl.Title, sl.Focus, sl.Formula, sl.Notes, sl.Answer}
	fields = append(fields, sl.Bullets...)
	if sl.Table != nil {
		fields = append(fields, sl.Table.Title)
		fields = append(fields, sl.Table.Headers...)
		for _, row := range sl.Table.Rows {
			fields = append(fields, row...)
		}
	}
	if sl.Figure != nil {
		fields = append(fields, sl.Figure.Title)
		for _, item := range sl.Figure.Items {
			fields = append(fields, item.Label, item.Detail)
		}
	}
	return fields
}
