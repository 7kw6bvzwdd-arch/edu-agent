package main

import (
	"strings"
	"testing"
)

func TestSourceGroupsDoNotTreatRepostsAsIndependentEvidence(t *testing.T) {
	body := strings.Repeat("浮力等于排开液体的重力；需要明确液体密度和实际排开体积，完全浸没后体积不随深度增大。", 5)
	sources := []Source{{ID: "S1", URL: "https://www.pep.com.cn/lesson", Content: body}, {ID: "S2", URL: "https://math.pep.com.cn/other", Content: "不同的教学内容"}, {ID: "S3", URL: "https://example.org/repost", Content: body + "转载来源标记"}, {ID: "S4", URL: "https://school.edu.cn/experiment", Content: strings.Repeat("电路中的电流与导体电阻有关，实验改接时断开电源，严禁短路。", 8)}}
	groupSourceOrigins(sources)
	if sources[0].EvidenceGroup == "" || sources[0].EvidenceGroup != sources[1].EvidenceGroup || sources[0].EvidenceGroup != sources[2].EvidenceGroup {
		t.Fatal("same publisher/repost counted independently", sources)
	}
	if sources[3].EvidenceGroup == sources[0].EvidenceGroup {
		t.Fatal("unrelated source collapsed")
	}
	if sources[0].Content != body {
		t.Fatal("evidence body normalized or discarded")
	}
	if sources[0].Origin != "pep.com.cn" {
		t.Fatal("publisher origin incorrect", sources[0].Origin)
	}
	groupSourceOrigins(sources)
	if sources[0].EvidenceGroup != sources[2].EvidenceGroup {
		t.Fatal("snapshot grouping not stable")
	}
}
