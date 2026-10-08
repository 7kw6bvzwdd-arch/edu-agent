package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCoursewareTableRecoveryMatrix(t *testing.T) {
	s := worksServer(t)
	for _, kind := range []string{"rows", "columns", "title", "header", "cell", "missing", "single-column", "empty", "crowded", "teaching-case"} {
		t.Run(kind, func(t *testing.T) {
			d := sampleDeck(TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "classroom"})
			table := &TeachingTable{Title: "记录", Headers: []string{"条件", "观察"}, Rows: [][]string{{"数据A", "数据B"}}}
			switch kind {
			case "rows":
				for i := 0; i < 9; i++ {
					table.Rows = append(table.Rows, []string{fmt.Sprintf("原始行%d", i), "保留数值"})
				}
			case "columns":
				table.Headers = []string{"条件", "一", "二", "三", "四", "五", "六", "七"}
				table.Rows = [][]string{{"记录", "1", "2", "3", "4", "5", "6", "7"}}
			case "title":
				table.Title = strings.Repeat("完整表格标题", 13)
			case "header":
				table.Headers[1] = strings.Repeat("完整表头", 7)
			case "cell":
				table.Rows[0][1] = strings.Repeat("完整说明必须保留。", 14)
			case "missing":
				table.Headers = []string{"条件", ""}
				table.Rows = [][]string{{"记录"}, {"", "第二行"}}
			case "single-column":
				table.Headers = []string{"观察"}
				table.Rows = [][]string{{"原始观察"}}
			case "empty":
				table = &TeachingTable{}
			case "crowded":
				for i := 0; i < 4; i++ {
					table.Rows = append(table.Rows, []string{strings.Repeat("实验条件", 10), strings.Repeat("观察内容", 10)})
				}
			case "teaching-case":
				d.Slides[6].Title = "同一物体，浮力为什么改变？"
				d.Slides[6].Section = "实验分析"
				d.Slides[6].Bullets = []string{"比较A、B、C组：浸入体积变化时，浮力如何变化？", "比较C、D、E组：完全浸没后，哪些条件改变了浮力？"}
				table = &TeachingTable{Title: "控制变量分析（教学假设数据，g取10 N/kg）", Headers: []string{"组别", "液体", "密度kg/m³", "排液体积cm³", "重力N", "浮力N", "拉力N"}, Rows: [][]string{
					{"A", "水", "1000", "20", "3", "0.2", "2.8"},
					{"B", "水", "1000", "50", "3", "0.5", "2.5"},
					{"C", "水", "1000", "100", "3", "1.0", "2.0"},
					{"D", "水（更深）", "1000", "100", "3", "1.0", "2.0"},
					{"E", "盐水", "1200", "100", "3", "1.2", "1.8"},
					{"F", "酒精", "800", "100", "3", "0.8", "2.2"},
				}}
				d.Slides[6].Notes = "这是演示用假设数据，不是学生实测。物体体积100 cm³，各组静止悬挂且不触底；A、B为部分浸入，C至F完全浸没。浮力由液体密度和排开液体体积确定，拉力等于重力减浮力。实验用低风险液体，不使用明火。"
			}
			d.Slides[6].Table = table
			original := string(asJSON(d))
			got := arrangeCourseware(d)
			if original != string(asJSON(d)) || !reflect.DeepEqual(arrangeCourseware(got), got) {
				t.Fatal("normalization mutates input or isn't idempotent")
			}
			if err := validateDeck(got); err != nil {
				t.Fatal(err)
			}
			b, err := s.renderCourseware(context.Background(), &got)
			if err != nil || len(b) < 1000 {
				t.Fatal("automatic repair did not export", err)
			}
			if out := os.Getenv("TEST_COURSEWARE_ARTIFACT_DIR"); out != "" {
				if err := os.MkdirAll(out, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(out, kind+".pptx"), b, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(out, kind+".json"), asJSON(got), 0600); err != nil {
					t.Fatal(err)
				}
			}
			visible := ""
			all := ""
			for _, sl := range got.Slides {
				visible += strings.Join(sl.Bullets, "")
				if sl.Table != nil {
					visible += sl.Table.Title + strings.Join(sl.Table.Headers, "")
					for _, row := range sl.Table.Rows {
						visible += strings.Join(row, "")
					}
				}
				all += sl.Notes
			}
			all += visible
			for _, row := range table.Rows {
				for _, cell := range row {
					if cell != "" && !strings.Contains(visible, cell) {
						t.Fatal("cell was lost from visible pages")
					}
				}
			}
			if table.Title != "" && !strings.Contains(all, table.Title) {
				t.Fatal("title lost")
			}
			for _, h := range table.Headers {
				if h != "" && !strings.Contains(all, h) {
					t.Fatal("header lost")
				}
			}
		})
	}
}

func TestCoursewareDenseTextPreservesOriginalWording(t *testing.T) {
	s := worksServer(t)
	d := sampleDeck(TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "classroom"})
	text := strings.Repeat("在同种液体中，浮力随排开液体的体积增加而增大；完全浸没后继续加深，排开液体的体积不再增加。", 5)
	d.Slides[1].Bullets = []string{text, "解释上述结论成立的条件。"}
	original := string(asJSON(d))
	got := arrangeCourseware(d)
	if original != string(asJSON(d)) || !reflect.DeepEqual(arrangeCourseware(got), got) {
		t.Fatal("text normalization is not stable")
	}
	if err := validateDeck(got); err != nil {
		t.Fatal(err)
	}
	if _, err := s.renderCourseware(context.Background(), &got); err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, sl := range got.Slides {
		joined += strings.Join(sl.Bullets, "")
	}
	if !strings.Contains(joined, text) {
		t.Fatal("body text lost during pagination")
	}
}

func TestCoursewareFieldNormalization(t *testing.T) {
	raw := `{"title":"课件","lesson":["教学目标","教学过程"],"needs_review":"true","slides":[{"title":"实验","bullets":"观察现象","notes":["完整讲解","检查答案"],"emphasis":"浮力","table":"","chart":[],"picture":{},"figure":{"kind":"interval","items":["开始","结束"],"values":["0","30"],"unit":"s"}}]}`
	var d Deck
	if err := decodeCourseware(raw, &d); err != nil {
		t.Fatal(err)
	}
	sl := d.Slides[0]
	if d.Lesson != "教学目标\n教学过程" || sl.Notes != "完整讲解\n检查答案" || !d.NeedsReview ||
		sl.Table != nil || sl.Picture != nil || sl.Chart != nil || sl.Figure.Items[1].Label != "结束" || sl.Figure.Values[1] != 30 {
		t.Fatal("lossless normalization failed")
	}
	for _, entry := range []struct{ raw, field string }{
		{`{"slides":[{"notes":{"private":"do-not-expose"}}]}`, "notes"},
		{`{"slides":[{"figure":"do-not-expose"}]}`, "figure"},
		{`{"slides":[{"figure":{"values":["do-not-expose"]}}]}`, "figure.values"},
	} {
		err := decodeCourseware(entry.raw, &d)
		if err == nil || !strings.Contains(err.Error(), "第1页") || !strings.Contains(err.Error(), entry.field) || strings.Contains(err.Error(), "do-not-expose") {
			t.Fatalf("missing safe field diagnostic: %v", err)
		}
	}
}

func TestCoursewareFigureContinuations(t *testing.T) {
	s := worksServer(t)
	for _, kind := range []string{"flow", "timeline", "route", "interval"} {
		t.Run(kind, func(t *testing.T) {
			d := sampleDeck(TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "classroom"})
			f := &TeachingFigure{Kind: kind, Title: "步骤顺序", Unit: "min"}
			for i := 0; i < 8; i++ {
				f.Items = append(f.Items, FigureItem{Label: fmt.Sprintf("节点%d", i), Detail: "原始说明"})
				if kind == "interval" {
					f.Values = append(f.Values, float64(i*i))
				}
			}
			d.Slides[6].Figure = f
			original := string(asJSON(d))
			got := arrangeCourseware(d)
			if original != string(asJSON(d)) || !reflect.DeepEqual(arrangeCourseware(got), got) {
				t.Fatal("figure arrangement mutates or repeatedly expands input")
			}
			if err := validateDeck(got); err != nil {
				t.Fatal(err)
			}
			found := map[string]bool{}
			for _, sl := range got.Slides {
				if sl.Figure == nil || sl.Figure.Title != f.Title {
					continue
				}
				for i, node := range sl.Figure.Items {
					found[node.Label] = true
					if kind == "interval" {
						var n int
						fmt.Sscanf(node.Label, "节点%d", &n)
						if sl.Figure.Values[i] != float64(n*n) {
							t.Fatal("interval association lost")
						}
					}
				}
			}
			if len(found) != 8 {
				t.Fatal("nodes lost")
			}
			pptx, err := s.renderCourseware(context.Background(), &got)
			if err != nil {
				t.Fatal(err)
			}
			if out := os.Getenv("TEST_COURSEWARE_ARTIFACT_DIR"); out != "" {
				if err := os.MkdirAll(out, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(out, "figure-"+kind+".pptx"), pptx, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "interval" {
				f.Values[4] = -1
				if validateDeck(arrangeCourseware(d)) == nil {
					t.Fatal("invalid interval was hidden by splitting")
				}
			}
		})
	}
}

func TestCoursewareModelOutputRecovery(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	var actor Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &actor)
	in := TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "forest"}
	for _, mode := range []string{"number-cell", "string-bullets", "field-variants", "wrong-field", "malformed-json", "truncated", "temporary-http", "permanent-http", "persistent-json"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if mode == "wrong-field" && calls == 2 {
					var request struct{ Messages []struct{ Content string } }
					if json.NewDecoder(r.Body).Decode(&request) != nil || len(request.Messages) < 2 ||
						!strings.Contains(request.Messages[1].Content, "第7页字段 notes") {
						t.Error("repair did not receive actionable field diagnosis")
					}
				}
				if mode == "permanent-http" {
					w.WriteHeader(401)
					return
				}
				if mode == "temporary-http" && calls == 1 {
					w.WriteHeader(503)
					return
				}
				d := sampleDeck(in)
				raw := string(asJSON(d))
				finish := "stop"
				if calls == 1 || mode == "persistent-json" {
					switch mode {
					case "field-variants", "wrong-field":
						var obj map[string]any
						json.Unmarshal(asJSON(d), &obj)
						sl := obj["slides"].([]any)[6].(map[string]any)
						if mode == "wrong-field" {
							sl["notes"] = map[string]any{"text": "讲解"}
						} else {
							obj["lesson"] = []string{d.Lesson}
							obj["needs_review"] = "true"
							sl["notes"] = []string{d.Slides[6].Notes}
							sl["table"], sl["chart"], sl["picture"] = "", []any{}, ""
						}
						raw = string(asJSON(obj))
					case "number-cell":
						var obj map[string]any
						json.Unmarshal(asJSON(d), &obj)
						obj["slides"].([]any)[6].(map[string]any)["table"] = map[string]any{"headers": []string{"体积", "浮力"}, "rows": [][]any{{100, 1.5}, {200, nil}}}
						raw = string(asJSON(obj))
					case "string-bullets":
						var obj map[string]any
						json.Unmarshal(asJSON(d), &obj)
						obj["slides"].([]any)[6].(map[string]any)["bullets"] = "比较条件并说明浮力大小的变化。"
						raw = string(asJSON(obj))
					case "malformed-json", "persistent-json":
						raw = "{invalid json"
					case "truncated":
						finish = "length"
					}
				}
				respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": finish, "message": map[string]string{"content": raw}}}})
			}))
			defer provider.Close()
			s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "test", "test"
			d, err := s.generate(context.Background(), actor, "recovery-check", in, nil)
			want := 2
			if mode == "number-cell" || mode == "string-bullets" || mode == "field-variants" || mode == "permanent-http" {
				want = 1
			}
			if calls != want {
				t.Fatalf("unexpected model requests: %d want %d", calls, want)
			}
			if mode == "permanent-http" || mode == "persistent-json" {
				if err == nil {
					t.Fatal("invalid result marked successful")
				}
				return
			}
			if err != nil || len(d.Slides) < 8 {
				t.Fatal("recoverable model output rejected", err)
			}
		})
	}
}

func TestOversizedTableAutomaticallyCompletesHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		d := sampleDeck(TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "forest"})
		d.Slides[6].Table = &TeachingTable{Title: "原始记录", Headers: []string{"实验", "体积", "密度", "重力", "浮力", "拉力"}}
		for i := 0; i < 7; i++ {
			d.Slides[6].Table.Rows = append(d.Slides[6].Table.Rows, []string{"组别" + string(rune('A'+i)), "10", "1", "3", "1", "2"})
		}
		var obj map[string]any
		json.Unmarshal(asJSON(d), &obj)
		sl := obj["slides"].([]any)[1].(map[string]any)
		sl["notes"] = []string{d.Slides[1].Notes}
		sl["table"], sl["chart"], sl["picture"] = "", []any{}, map[string]any{}
		sl["figure"] = map[string]any{"kind": "flow", "title": "观察与比较", "items": []string{"准备材料", "控制条件", "进行实验", "记录结果", "比较解释"}}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(asJSON(obj))}}}})
	}))
	defer provider.Close()
	s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "test", "test"
	in := TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "forest", UseKB: true, IdempotencyKey: newID()}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	route := "/api/tasks/" + id
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	task := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if task["status"] != "awaiting_confirmation" {
		t.Fatalf("oversized table halted production: %v", task["error"])
	}
	var d Deck
	json.Unmarshal(asJSON(task["result"]), &d)
	if calls != 1 {
		t.Fatalf("layout repair unnecessarily called the model %d times", calls)
	}
	if !strings.Contains(string(asJSON(d)), "组别G") {
		t.Fatal("last row lost")
	}
	for _, sl := range d.Slides {
		if sl.Table != nil && (len(sl.Table.Headers) > 4 || len(sl.Table.Rows) > 5) {
			t.Fatal("oversized table remains")
		}
	}
	worksCall(t, s, "POST", route+"/confirm", token, map[string]any{"result": d, "revision": task["revision"], "accept_review": true}, 202)
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	task = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if task["status"] != "succeeded" || task["charge_state"] != "settled" {
		t.Fatal("file not delivered", task["error"])
	}
	pptx := worksCall(t, s, "GET", route+"/download/pptx", token, nil, 200).Body.Bytes()
	if len(pptx) < 1000 || string(pptx[:2]) != "PK" {
		t.Fatal("not a PPTX artifact")
	}
}
