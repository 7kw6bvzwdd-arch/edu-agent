package main

import (
	"encoding/json"
	"testing"
)

func TestTeachingPlanModelDependencyIntegers(t *testing.T) {
	for _, deps := range []string{`[1]`, `["1"]`, `[1,"2"]`, `[]`, `null`} {
		var page TeachingPage
		if err := json.Unmarshal([]byte(`{"position":3,"goal":"比较","task":"比较图像","minutes":3,"depends_on":`+deps+`}`), &page); err != nil {
			t.Fatalf("unambiguous model page integers rejected: %s: %v", deps, err)
		}
		if len(page.DependsOn) > 0 && page.DependsOn[0] != 1 {
			t.Fatal("dependency value changed")
		}
	}
	for _, deps := range []string{`["第一页"]`, `["1.5"]`, `[1.5]`, `[true]`, `[null]`, `["+1"]`} {
		var page TeachingPage
		if json.Unmarshal([]byte(`{"depends_on":`+deps+`}`), &page) == nil {
			t.Fatal("ambiguous dependency accepted", deps)
		}
	}
	var plan TeachingPlan
	if err := json.Unmarshal([]byte(`{"summary":"实际计划","pages":[{"position":1,"goal":"知识","task":"概念","minutes":3,"preview":true,"depends_on":[]},{"position":2,"goal":"应用","task":"例题","minutes":3,"preview":true,"depends_on":["1"]},{"position":3,"goal":"检查","task":"练习","minutes":3,"preview":true,"depends_on":["2"]}]}`), &plan); err != nil {
		t.Fatal(err)
	}
	if err := validateTeachingPlan(plan); err != nil {
		t.Fatal(err)
	}
	plan.Pages[2].DependsOn = []int{3}
	if validateTeachingPlan(plan) == nil {
		t.Fatal("self dependency bypassed validation")
	}
}
