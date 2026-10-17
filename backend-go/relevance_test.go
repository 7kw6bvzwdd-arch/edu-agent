package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRelevantPassagesRetainEvidenceBeyondPagePrefix(t *testing.T) {
	body := strings.Repeat("这是网站介绍和栏目说明，与教学问题无关。\n", 160) +
		"浮力实验：测量物体排开液体的体积。\nF浮 = ρ液 g V排。\n适用条件：液体密度近似不变；完全浸没后不随深度继续增大。\n安全提示：轻放玻璃器皿，及时擦干溢水。\n扫码关注，点击下载APP，领取浮力课程。\n八年级物理：欧姆定律 I=U/R。"
	got, info := relevantPassages("浮力", body, 1800)
	for _, want := range []string{"浮力实验", "F浮 =", "完全浸没", "轻放玻璃"} {
		if !strings.Contains(got, want) {
			t.Error("lost evidence", want, got)
		}
	}
	for _, bad := range []string{"网站介绍", "下载APP", "欧姆定律"} {
		if strings.Contains(got, bad) {
			t.Error("noise survived", bad)
		}
	}
	if info.Kept == 0 || info.Removed == 0 || info.Method != "lexical-context-v1" {
		t.Fatal("missing honest diagnostics", info)
	}
	if got, _ := relevantPassages("浮力", "八年级物理 人教版\n欧姆定律 I=U/R。", 1800); got != "" {
		t.Fatal("off-topic lesson accepted", got)
	}
}

func TestRelevantPassagesPreserveDisagreementAndNumbers(t *testing.T) {
	body := "浮力为5N，方向竖直向上。\n浮力为5N，方向竖直向上。\n浮力为6N，方向竖直向上。\n浮力不随深度改变。\n浮力随深度改变。"
	got, _ := relevantPassages("浮力", body, 1800)
	if strings.Count(got, "浮力为5N") != 1 || !strings.Contains(got, "6N") || !strings.Contains(got, "浮力不随") || !strings.Contains(got, "浮力随") {
		t.Fatal("dedup erased conflicting evidence", got)
	}
}

func TestWebRetrievalFiltersRawContentAndKeepsProvenance(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	search := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q map[string]any
		json.NewDecoder(r.Body).Decode(&q)
		if q["include_raw_content"] == false {
			t.Error("clean search body not requested")
		}
		respond(w, 200, map[string]any{"results": []map[string]any{
			{"title": "浮力研究", "url": "https://example.org/buoyancy", "raw_content": strings.Repeat("栏目导航和无关说明。\n", 400) + "浮力等于排开液体的重力。\n公式 F浮=ρ液gV排。"},
			{"title": "浮力研究", "url": "https://example.org/unrelated", "raw_content": "欧姆定律 I=U/R，电压单位为V。"},
		}})
	}))
	defer search.Close()
	s.cfg.Mode = "live"
	s.cfg.SearchURL = search.URL
	s.cfg.SearchKey = "test"
	sources, err := s.retrieve(context.Background(), a, "八年级 物理 人教版 浮力", false, true, "浮力")
	if err != nil || len(sources) != 1 {
		t.Fatal("irrelevant source leaked", err, sources)
	}
	v := sources[0]
	if !strings.Contains(v.Content, "ρ液") || v.URL != "https://example.org/buoyancy" || v.Hash == "" || v.RetrievedAt == "" || v.Filter == nil || v.Kind != "web_extracted" {
		t.Fatal("bad evidence/provenance", v)
	}
}
