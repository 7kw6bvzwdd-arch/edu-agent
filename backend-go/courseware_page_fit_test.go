package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestCoursewareTableSpaceRepair(t *testing.T) {
	url := os.Getenv("TEST_RENDER_URL")
	if url == "" {
		t.Skip("set TEST_RENDER_URL for real PPTX integration")
	}
	s := &Server{cfg: Config{Mode: "sample", RenderURL: url}, http: &http.Client{}}
	d := Deck{Title: "浮力", Theme: "classroom", EditableFormulas: true, Slides: []Slide{{
		Title: "控制变量探究浮力", Layout: "comparison", Focus: "保持其他条件相同，比较排开体积和液体密度。",
		Formula: "F_{浮}=G-F_{拉}", Bullets: []string{"同一物体逐步浸入水中，记录示数。"},
		Table: &TeachingTable{Title: "实验记录", Headers: []string{"阶段", "拉力/N", "浮力/N"}, Rows: [][]string{
			{"半浸入", "2.2", "0.5"}, {"浸没水", "1.7", "1.0"}, {"浸没盐水", "1.6", "1.1"},
		}}, Notes: "物体重力为2.7N；指导学生每次只改变一个变量，并依据测量值计算浮力。",
	}}}
	original := d.Slides[0]
	_, err := s.renderPPTX(context.Background(), d)
	var issue coursewareRenderError
	if !errors.As(err, &issue) || issue.Field != "table" {
		t.Fatalf("expected table-space overflow, got %v", err)
	}
	pptx, err := s.renderCoursewarePage(context.Background(), &d, 1)
	if err != nil {
		t.Fatalf("same-page table repair still stops generation: %v", err)
	}
	if len(pptx) < 1000 || string(pptx[:2]) != "PK" || len(d.Slides) != 1 {
		t.Fatal("real single-page PPTX missing")
	}
	if !reflect.DeepEqual(original.Table, d.Slides[0].Table) || original.Formula != d.Slides[0].Formula {
		t.Fatal("evidence or formula changed")
	}
	for _, text := range append([]string{original.Focus}, original.Bullets...) {
		if !strings.Contains(d.Slides[0].Focus+strings.Join(d.Slides[0].Bullets, "\n")+d.Slides[0].Notes, text) {
			t.Fatal("teaching text lost")
		}
	}
	// The actual model also returned a narrow three-step flow beside the table.
	// Its explanatory annotations must be retained in notes when the panel is tight.
	d.Slides[0].Figure = &TeachingFigure{Kind: "flow", Title: "实验步骤", Items: []FigureItem{
		{Label: "测重力", Detail: "竖直悬挂铝块，读出G=2.7 N"},
		{Label: "增大浸入体积", Detail: "逐次读F拉，算F浮=G−F拉"},
		{Label: "换盐水浸没", Detail: "同一体积比较不同液体的F浮"},
	}}
	figure := *d.Slides[0].Figure
	_, err = s.renderCoursewarePage(context.Background(), &d, 1)
	if err != nil {
		t.Fatalf("real model table/flow combination still stops generation: %v", err)
	}
	if !reflect.DeepEqual(original.Table, d.Slides[0].Table) || original.Formula != d.Slides[0].Formula {
		t.Fatal("evidence changed")
	}
	for i, item := range figure.Items {
		if d.Slides[0].Figure.Items[i].Label != item.Label || !strings.Contains(string(asJSON(d.Slides[0].Figure))+d.Slides[0].Notes, item.Detail) {
			t.Fatal("experiment flow content lost")
		}
	}
}

func TestCoursewareCaptionDoesNotCutUnitsOrConditions(t *testing.T) {
	text := "同一铝块完全浸没于盐水（1.2×10³ kg/m³）中，保持排液体积不变并读出F拉"
	sl := Slide{Bullets: []string{text}}
	compactCoursewarePageText(&sl, "bullets")
	if sl.Bullets[0] != text {
		t.Fatalf("caption cut a condition or unit: %s", sl.Bullets[0])
	}
}

func TestCoursewarePreviewOverflowRetryHTTP(t *testing.T) {
	for _, kind := range []string{"title-breaks", "media-caption", "table-panel"} {
		t.Run(kind, func(t *testing.T) {
			s := worksServer(t)
			token := worksLogin(t, s, "teacher")
			in := TaskInput{Type: "courseware", Topic: "一次函数的图像与性质", Pages: 12, Theme: "classroom", UseKB: true, PreviewFirst: true, IdempotencyKey: newID()}
			id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
			d := sampleDeck(in)
			assignSlideIDs(&d)
			candidate := d
			candidate.Slides = []Slide{d.Slides[2]}
			candidate.Slides[0].Title = "一次函数\n图像与性质\n探究与结论"
			if kind == "media-caption" || kind == "table-panel" {
				candidate.Slides[0].Title = "一次函数的图像"
				candidate.Slides[0].Table = &TeachingTable{Headers: []string{"x", "y"}, Rows: [][]string{{"0", "1"}, {"1", "3"}}}
				candidate.Slides[0].Formula = "y=2x+1"
				candidate.Slides[0].Focus = "图像是一条直线"
				candidate.Slides[0].Bullets = []string{strings.Repeat("观察函数图像并结合数据解释变化规律。", 4), strings.Repeat("比较相邻点之间的关系并记录推理依据。", 4), strings.Repeat("根据函数表达式检查每一步计算的结果。", 4)}
				if kind == "table-panel" {
					candidate.Slides[0].Table.Title = "取值记录"
					candidate.Slides[0].Table.Rows = [][]string{{"0", "1"}, {"1", "3"}, {"2", "5"}}
					candidate.Slides[0].Bullets = []string{"结合取值表观察一次函数的变化规律。"}
				}
			}
			// Real renderer reproduces both the explicit title breaks and the caption
			// consuming the table's space. A model that repeats the same text cannot fix it.
			if _, err := s.renderPPTX(context.Background(), candidate); err == nil {
				t.Fatal("fixture did not reproduce text overflow")
			}
			original := candidate.Slides[0]
			d.Slides = d.Slides[:2]
			d.Generation = &CoursewareCheckpoint{Version: coursewarePageRepairVersion, Phase: "preview", RepairPage: 3}
			const resume = "已恢复逐页检查点，继续只处理问题页和未完成页；未变更页面的审核保留。"
			d.Notice = "请教师核验。 " + strings.Repeat(resume+" ", 3)
			if err := s.finish(context.Background(), 1, id, "failed", "第3页排版失败", "", d); err != nil {
				t.Fatal(err)
			}
			route := "/api/tasks/" + id
			worksCall(t, s, "POST", route+"/retry", worksLogin(t, s, "other"), nil, 404)
			child := worksObject(t, worksCall(t, s, "POST", route+"/retry", token, nil, 202))["id"].(string)
			route = "/api/tasks/" + child
			retry := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
			if count := strings.Count(retry["result"].(map[string]any)["notice"].(string), resume); count != 1 {
				t.Errorf("duplicated continuation notice: %d", count)
			}
			calls := 0
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var req struct{ Messages []struct{ Content string } }
				json.NewDecoder(r.Body).Decode(&req)
				prompt := req.Messages[len(req.Messages)-1].Content
				if !strings.Contains(prompt, "本批只返回第3至第3页") {
					t.Error("regenerated saved pages")
				}
				respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(asJSON(candidate))}}}})
			}))
			defer provider.Close()
			s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "test", "test"
			tx, err := s.tx(context.Background(), 1)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = tx.Exec(context.Background(), "UPDATE tasks SET mode='live' WHERE id=$1", child); err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := s.workOne(context.Background()); err != nil {
				t.Fatal(err)
			}
			done := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
			if done["status"] != "awaiting_preview" {
				t.Fatalf("overflow still stops page 3: %v %v (model calls %d)", done["status"], done["error"], calls)
			}
			var result Deck
			json.Unmarshal(asJSON(done["result"]), &result)
			if len(result.Slides) != 3 || !reflect.DeepEqual(result.Slides[:2], d.Slides) {
				t.Fatal("saved pages changed or extra page inserted")
			}
			fixed := result.Slides[2]
			for _, text := range append([]string{original.Title}, original.Bullets...) {
				if !strings.Contains(fixed.Notes+fixed.Title+strings.Join(fixed.Bullets, "\n"), text) {
					t.Fatal("original content lost")
				}
			}
			if !reflect.DeepEqual(fixed.Table, original.Table) || fixed.Formula != original.Formula {
				t.Fatal("teaching data changed")
			}
			if calls != 1 {
				t.Fatalf("layout used extra model calls: %d", calls)
			}
			artifact := worksCall(t, s, "GET", route+"/preview-pptx", token, nil, 200).Body.Bytes()
			if len(artifact) < 1000 || string(artifact[:2]) != "PK" {
				t.Fatal("real PPTX missing")
			}
			worksCall(t, s, "POST", route+"/cancel", token, nil, 200)
		})
	}
}

func TestCoursewareSinglePageErrorUsesActualIndex(t *testing.T) {
	renderer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			respond(w, 200, map[string]string{"courseware_media_version": "media-v1"})
			return
		}
		respond(w, 422, map[string]any{"detail": map[string]any{"code": "formula_invalid", "field": "formula", "page": 1}})
	}))
	defer renderer.Close()
	s := &Server{cfg: Config{Mode: "sample", RenderURL: renderer.URL}, http: renderer.Client()}
	in := TaskInput{Topic: "浮力", Pages: 12, Theme: "classroom"}
	d := sampleDeck(in)
	d.Slides = d.Slides[:2]
	_, _, _, err := s.coursewareSinglePage(context.Background(), Actor{}, "", in, nil, d, 3, "", "preview")
	var issue coursewareRenderError
	if err == nil || strings.Contains(err.Error(), "第1页") || !errors.As(err, &issue) || issue.Page != 3 {
		t.Fatalf("single-page renderer index leaked: %v", err)
	}
}
