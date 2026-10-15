package main

import "strings"

// Short original excerpts checked against OpenStax on 2026-10-13. These public
// CC BY textbooks are supplemental sources, not PEP text or grade certification.
// Each generated claim still needs an exact quote and independent review.
func coreTeachingSources(query string) []Source {
	entries := []struct{ topic, title, url, location, text string }{
		{"一次函数", "OpenStax Precalculus 2e · Linear Functions", "https://openstax.org/books/precalculus-2e/pages/2-1-linear-functions", "§2.1，Determining whether a Linear Function Is Increasing, Decreasing, or Constant，Figure 5 前段落",
			"For an increasing function , as with the train example, the output values increase as the input values increase. The graph of an increasing function has a positive slope. A line with a positive slope slants upward from left to right as in Figure 5 (a) . For a decreasing function , the slope is negative. The output values decrease as the input values increase."},
		{"二次函数", "OpenStax Precalculus 2e · Quadratic Functions", "https://openstax.org/books/precalculus-2e/pages/3-2-quadratic-functions", "§3.2，Forms of Quadratic Functions 定义框",
			"A quadratic function is a function of degree two. The graph of a quadratic function is a parabola."},
		{"浮力", "OpenStax University Physics Volume 1 · Archimedes’ Principle and Buoyancy", "https://openstax.org/books/university-physics-volume-1/pages/14-4-archimedes-principle-and-buoyancy", "§14.4，Archimedes’ Principle 定义框首句",
			"The buoyant force on an object equals the weight of the fluid it displaces. In equation form, Archimedes’ principle is"},
		{"串并联电路", "OpenStax University Physics Volume 2 · Resistors in Series and Parallel", "https://openstax.org/books/university-physics-volume-2/pages/10-2-resistors-in-series-and-parallel", "§10.2，Resistors in Series，Figure 10.12 前段落",
			"Since there is only one path for the charges to flow through, the current is the same through each resistor. The equivalent resistance of a set of resistors in a series connection is equal to the algebraic sum of the individual resistances."},
	}
	out := []Source{}
	for _, entry := range entries {
		if strings.Contains(query, entry.topic) {
			out = append(out, Source{Title: entry.title, URL: entry.url, Location: entry.location,
				Content: entry.text, Kind: "core_excerpt", Reliability: "已核对的OpenStax原文摘录（CC BY），非人教教材；具体结论仍需审核"})
		}
	}
	return out
}
