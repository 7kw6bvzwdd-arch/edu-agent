package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// Acceptance stays at authenticated HTTP boundaries; provider output is a fixture.
func TestCoursewareSubjectsWithoutPresetsHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "admin")
	other := worksLogin(t, s, "other")
	before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	cases := []struct{ subject, grade, topic string }{
		{"语文", "五年级", "古诗中的意象"}, {"英语", "高一", "Ordering food"}, {"历史", "七年级", "丝绸之路"},
		{"地理", "高二", "水循环"}, {"化学", "九年级", "物质的变化"}, {"生物", "七年级", "植物细胞"},
		{"道德与法治", "六年级", "规则与责任"}, {"信息科技", "高一", "信息编码"}, {"音乐", "三年级", "节奏"},
		{"美术", "四年级", "冷暖色"}, {"体育与健康", "大学", "运动安全"}, {"劳动", "二年级", "整理书包"},
		{"科学", "幼儿园大班", "观察树叶"}, {"数学", "成人学习", "百分数"}, {"物理", "高三", "波的传播"},
		{"跨学科项目", "职业教育", "校园节水"}, {"", "", "阅读课"},
	}
	for _, c := range cases {
		body := TaskInput{Type: "courseware", Topic: c.topic, Subject: c.subject, Grade: c.grade, UseKB: true, PreviewFirst: true, IdempotencyKey: newID()}
		worksCall(t, s, "POST", "/api/tasks", "", body, 401)
		v := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, body, 202))
		id := v["id"].(string)
		record := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
		in := record["input"].(map[string]any)
		if in["subject"] != c.subject || in["grade"] != c.grade || in["textbook"] != "" || in["detail"] != "auto" {
			t.Fatal("courseware scope or unspecified fields overwritten", in)
		}
		if minutes, ok := in["lesson_minutes"]; ok && minutes != float64(0) {
			t.Fatal("unspecified duration became a preset", in)
		}
		worksCall(t, s, "GET", "/api/tasks/"+id, other, nil, 404)
		worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
	}
	after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	for _, key := range []string{"balance", "reserved", "available"} {
		if before[key] != after[key] {
			t.Fatal("cancelled subject probes changed member balance", key, before[key], after[key])
		}
	}
}

func TestCoursewareSubjectClarificationHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "admin")
	content := `{"reply":"这是一节阅读课，您希望聚焦意象还是朗读？","tool":"courseware","action":"discuss","settings":{"subject":"语文","grade":"小学五年级"},"requirements":"聚焦古诗意象","questions":[],"assumptions":[]}`
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
	}))
	defer provider.Close()
	s.cfg.Mode = "live"
	s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = provider.URL, "fixture", "fixture"
	body := map[string]any{"input": TaskInput{Topic: "古诗意象"}, "messages": []map[string]string{{"role": "user", "content": "小学五年级语文古诗意象课件"}}}
	v := worksObject(t, worksCall(t, s, "POST", "/api/workspace/brief", token, body, 200))
	blocks := string(asJSON(v["blocks"]))
	for _, preset := range []string{"物理（默认", "八年级（默认", "人教版（默认", "45（默认", "典型例题"} {
		if strings.Contains(blocks, preset) {
			t.Fatal("unconfirmed preset leaked into requirements", blocks)
		}
	}
	if !strings.Contains(blocks, "未指定") {
		t.Fatal("missing fields must remain unspecified", blocks)
	}
}

// Model responses are fixtures; PPTX and screenshots use the real renderer.
func TestCoursewareAllSubjectsPreviewToPPTXHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "other")
	for _, c := range []struct{ subject, grade, topic string }{{"语文", "小学五年级", "静夜思的意象"}, {"英语", "高一", "Ordering food"}, {"历史", "七年级", "丝绸之路"}} {
		t.Run(c.subject, func(t *testing.T) {
			lesson := Deck{Title: c.topic, Theme: "ocean", Lesson: "接口验收固定内容，非真实模型生成；背景、理解与应用，答案留在教师备注。", NeedsReview: true, Slides: []Slide{
				{Title: c.topic + "：学习情境", Layout: "concept", Bullets: []string{"观察本课材料，描述你已经理解的内容。"}, Notes: "教师先提供本课真实材料，引导学生说明观察到的线索，并区分材料中的事实与个人理解。"},
				{Title: "比较与理解", Layout: "comparison", Bullets: []string{"比较两种表达，说明它们在当前情境中的作用。"}, Notes: "教师让学生结合具体上下文给出判断依据；接受有材料支持的不同解释，不将开放问题压成唯一答案。"},
				{Title: "表达与应用", Layout: "steps", Bullets: []string{"选择一处材料，用自己的语言解释，并说明依据。"}, Notes: "学生独立表达后交换意见，教师按内容准确、证据清楚和表达完整反馈；本页采用开放任务而非计算题。"},
				{Title: "独立交流", Layout: "concept", Bullets: []string{"与同伴交流理解，依据材料回应不同意见。"}, Notes: "教师提醒学生回到材料，尊重有依据的不同理解，并记录还需进一步核实的问题。"},
				{Title: "课堂回顾", Layout: "summary", Bullets: []string{"用自己的话回顾今天的收获，并提出一个新问题。"}, Notes: "学生回顾本课的理解过程，教师根据学生反馈决定下次学习需要补充的材料。"},
			}}
			plan := TeachingPlan{Summary: "按实际学科安排；课时未指定，分钟仅为规划估计。", Conventions: "使用同一材料与术语。"}
			plan.LearningGoals = []LearningGoal{{ID: "g1", Text: "依据材料表达理解", Core: true}}
			for i, sl := range lesson.Slides {
				plan.Pages = append(plan.Pages, TeachingPage{Position: i + 1, Goal: sl.Title, Task: "材料理解与表达", Visual: "不需要数值图", Minutes: 3, Preview: i < 3})
				plan.Pages[i].GoalRefs = []string{"g1"}
				if i == 2 {
					plan.Pages[i].ChecksGoals = []string{"g1"}
				}
			}
			var callsMu sync.Mutex
			generated := map[int]int{}
			visualCalls := 0
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Model    string
					Messages []struct{ Content json.RawMessage }
				}
				if json.NewDecoder(r.Body).Decode(&req) != nil {
					w.WriteHeader(400)
					return
				}
				raw := `{"issues":[]}`
				if req.Model == "subject-fixture" {
					var prompt string
					json.Unmarshal(req.Messages[len(req.Messages)-1].Content, &prompt)
					if strings.Contains(prompt, "规划符合实际学科") {
						raw = string(asJSON(plan))
					} else if strings.Contains(prompt, "本批只返回第") {
						var page, end int
						fmt.Sscanf(regexp.MustCompile(`本批只返回第\d+至第\d+页`).FindString(prompt), "本批只返回第%d至第%d页", &page, &end)
						if page < 1 || page > len(lesson.Slides) || page != end {
							t.Error("invalid page request", page, end)
							w.WriteHeader(400)
							return
						}
						callsMu.Lock()
						generated[page]++
						callsMu.Unlock()
						one := lesson
						one.Slides = lesson.Slides[page-1 : page]
						raw = string(asJSON(one))
					} else {
						raw = `{"summary":"固定验收反馈"}`
					}
				} else if req.Model == "vision-fixture" {
					if !strings.Contains(string(req.Messages[0].Content), "data:image/png;base64,") {
						t.Error("visual review did not receive the actual screenshot")
					}
					callsMu.Lock()
					visualCalls++
					callsMu.Unlock()
				}
				respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": raw}}}})
			}))
			defer provider.Close()
			s.cfg.Mode = "live"
			s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = provider.URL, "fixture", "subject-fixture"
			s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = provider.URL, "fixture", "vision-fixture"
			s.cfg.CoursewareContentReviewDisabled = true
			before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
			in := TaskInput{Type: "courseware", Topic: c.topic, Subject: c.subject, Grade: c.grade, UseKB: true, Theme: "ocean", PreviewFirst: true, IdempotencyKey: newID()}
			id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
			route := "/api/tasks/" + id
			if err := s.workOne(context.Background()); err != nil {
				t.Fatal(err)
			}
			record := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
			if record["status"] != "awaiting_preview" {
				t.Fatal("preview failed", record["status"], record["error"])
			}
			var d Deck
			json.Unmarshal(asJSON(record["result"]), &d)
			if len(d.Slides) != 3 || d.Plan == nil {
				t.Fatal("three-page plan missing")
			}
			previewHashes := map[string]string{}
			for _, sl := range d.Slides {
				previewHashes[sl.ID] = slideContentHash(d, sl)
			}
			worksCall(t, s, "POST", route+"/expand", token, map[string]any{"revision": record["revision"]}, 409)
			for _, sl := range d.Slides {
				worksCall(t, s, "POST", route+"/slides/"+sl.ID+"/review", token, map[string]any{"revision": record["revision"], "hash": slideContentHash(d, sl), "approved": true}, 200)
			}
			worksCall(t, s, "POST", route+"/expand", token, map[string]any{"revision": record["revision"]}, 202)
			if err := s.workOne(context.Background()); err != nil {
				t.Fatal(err)
			}
			record = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
			if record["status"] != "awaiting_confirmation" {
				t.Fatal("expansion failed", record["status"], record["error"])
			}
			json.Unmarshal(asJSON(record["result"]), &d)
			if len(d.Slides) != 5 {
				t.Fatal("remaining pages were not generated")
			}
			for i, sl := range d.Slides {
				if sl.Order != i+1 || (previewHashes[sl.ID] != "" && previewHashes[sl.ID] != slideContentHash(d, sl)) {
					t.Fatal("expansion overwrote an approved page or teaching order")
				}
				worksCall(t, s, "POST", route+"/slides/"+sl.ID+"/review", token, map[string]any{"revision": record["revision"], "hash": slideContentHash(d, sl), "approved": true}, 200)
			}
			callsMu.Lock()
			for i := 1; i <= 5; i++ {
				if generated[i] != 1 {
					t.Error("page regenerated or missing", i, generated[i])
				}
			}
			if visualCalls != 5 {
				t.Error("all generated pages must receive screenshot review", visualCalls)
			}
			callsMu.Unlock()
			if c.subject == "语文" {
				oldHash := slideContentHash(d, d.Slides[0])
				d.Slides[0].Title = c.topic + "：教师补充观察"
				worksCall(t, s, "POST", route+"/draft", other, map[string]any{"revision": record["revision"], "result": d}, 404)
				worksCall(t, s, "POST", route+"/draft", token, map[string]any{"revision": record["revision"], "result": d}, 200)
				record = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
				json.Unmarshal(asJSON(record["result"]), &d)
				worksCall(t, s, "POST", route+"/slides/"+d.Slides[0].ID+"/review", token, map[string]any{"revision": record["revision"], "hash": oldHash, "approved": true}, 409)
				worksCall(t, s, "POST", route+"/confirm", token, map[string]any{"revision": record["revision"], "result": d, "accept_review": true}, 409)
				worksCall(t, s, "POST", route+"/slides/"+d.Slides[0].ID+"/review", token, map[string]any{"revision": record["revision"], "hash": slideContentHash(d, d.Slides[0]), "approved": true}, 200)
			}
			worksCall(t, s, "POST", route+"/confirm", token, map[string]any{"revision": record["revision"], "result": d, "accept_review": true}, 202)
			if err := s.workOne(context.Background()); err != nil {
				t.Fatal(err)
			}
			record = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
			if record["status"] != "succeeded" {
				t.Fatal("delivery failed", record["status"], record["error"])
			}
			worksCall(t, s, "GET", route+"/download/pptx", other, nil, 404)
			worksCall(t, s, "GET", route+"/download/pptx", "", nil, 401)
			file := worksCall(t, s, "GET", route+"/download/pptx", token, nil, 200).Body.Bytes()
			z, err := zip.NewReader(bytes.NewReader(file), int64(len(file)))
			if err != nil {
				t.Fatal(err)
			}
			xml := ""
			for _, f := range z.File {
				if f.Name == "ppt/slides/slide1.xml" {
					reader, _ := f.Open()
					b, _ := io.ReadAll(reader)
					reader.Close()
					xml = string(b)
				}
			}
			if !strings.Contains(xml, c.topic) {
				t.Fatal("real PPTX lost subject text")
			}
			if c.subject == "语文" && !strings.Contains(xml, "教师补充观察") {
				t.Fatal("manual edit did not reach the delivered PPTX")
			}
			after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
			if after["balance"].(float64) != before["balance"].(float64)-20 || after["reserved"] != before["reserved"] {
				t.Fatal("delivery accounting wrong")
			}
		})
	}
}

func TestCoursewareAllSubjectsMaterialSelectionHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "other")
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	id := newID()
	_, err = tx.Exec(context.Background(), "INSERT INTO documents(id,tenant_id,member_id,name,status,pages) VALUES($1,$2,$3,'全科接口验收材料','ready',$4)", id, a.TenantID, a.MemberID, asJSON([]Page{{1, "观察课文中的意象，结合原文解释自己的理解。"}}))
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	tag := CurriculumTag{Subject: "语文", Grade: "小学五年级", Edition: "教师提供版次"}
	worksCall(t, s, "PATCH", "/api/documents/"+id+"/curriculum", "", tag, 401)
	worksCall(t, s, "PATCH", "/api/documents/"+id+"/curriculum", other, tag, 404)
	worksCall(t, s, "PATCH", "/api/documents/"+id+"/curriculum", token, tag, 200)
	for _, fields := range []TaskInput{{}, {Subject: "语文"}, {Grade: "小学五年级"}, {Textbook: "教师提供版次"}} {
		fields.MaterialID, fields.MaterialStart, fields.MaterialEnd = id, 1, 1
		worksCall(t, s, "POST", "/api/courseware/materials/preview", token, fields, 200)
		worksCall(t, s, "POST", "/api/courseware/materials/preview", other, fields, 404)
	}
	for _, fields := range []TaskInput{{Subject: "数学"}, {Grade: "高一"}, {Textbook: "另一版次"}} {
		fields.MaterialID, fields.MaterialStart, fields.MaterialEnd = id, 1, 1
		worksCall(t, s, "POST", "/api/courseware/materials/preview", token, fields, 400)
	}
}
