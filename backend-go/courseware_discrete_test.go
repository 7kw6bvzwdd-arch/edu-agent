package main

import "testing"

func TestCountDomainCannotBecomeContinuousSegment(t *testing.T) {
	for _, tc := range []struct {
		name, notes string
		blocked     bool
	}{
		{"observed", "x是使用次数，只能取0到10的数。此时图象是连接(0,20)与(10,0)的一条线段，不是整条直线。", true},
		{"integer_points", "x是使用次数，x只能取0至10的整数。图象为11个离散点，不连接成线段。", false},
		{"continuous_distance", "x表示行驶距离，0≤x≤10。图象是连接两个端点的线段。", false},
		{"corrected_misconception", "x是使用次数，必须为整数。常见错误：图象是一条线段。纠正：应为离散点，不能连线。", false},
		{"question_only", "x是使用次数。提问：图象是一条线段吗？应答：不是，x只能取整数。", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := Deck{Slides: []Slide{{Title: "储值卡", Notes: tc.notes}}}
			if got := blockingPageIssues(teachingDiscreteCountIssues(d)) != ""; got != tc.blocked {
				t.Fatalf("blocked=%v want=%v", got, tc.blocked)
			}
		})
	}
}
