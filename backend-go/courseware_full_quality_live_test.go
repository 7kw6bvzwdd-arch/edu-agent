package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Entirely live generation and independent review. Teacher approvals below are
// test actions in an isolated database, never real teacher quality acceptance.
func TestCoursewareFullQualityLive(t *testing.T) {
	if os.Getenv("TEST_FULL_DECK_LIVE") != "1" {
		t.Skip("explicit paid live full-deck opt-in required")
	}
	for _, tc := range []struct{ key, topic, grade, subject, requirements string }{
		{"linear", "一次函数", "八年级", "数学", "学生已学正比例函数。用y=2x+1、y=-x+2解释斜率和截距；用同一坐标比例作图比较；给出有条件的例题及独立练习，答案放备注。"},
		{"buoyancy", "浮力", "八年级", "物理", "g取10 N/kg。区分浮力、重力及绳拉力；实验写明器材、安全、控制变量和排液体积。全部浸没后密度不变时浮力不随深度增大，数据标明计算或实测。"},
		{"quadratic", "二次函数", "九年级", "数学", "学生已学一次函数。用y=x²-2x-3解释顶点、对称轴、交点及增减区间；图像和代数相互核对，给完整条件的例题和独立练习。"},
		{"circuits", "串并联电路", "九年级", "物理", "围绕电流路径、通断和电流电压规律；理想电源、导线和电表假设明确。用低压学生电源，严禁短接电源；串联/并联图必须支持实际教学问题，答案放备注。"},
		{"lens", "凸透镜成像规律", "八年级", "物理", "完整课堂，学生已学光的直线传播与折射；用明确标识的光路示意解释物距、像距、焦距及像的性质，区分实像与虚像。f取10cm，用u=30cm、20cm、15cm、5cm说明条件，不把示意计算冒充实测；包括安全操作、观察任务、完整条件的独立练习与备注答案。"},
		{"history", "辛亥革命的背景与影响", "八年级", "历史", "完整45分钟课堂，区分1911年武昌起义和1912年中华民国临时政府成立；使用可核实的史实，不虚构史料原文。至少一页用可追溯的相关实景照片开展观察任务，规划image_need并自动配图，图注只陈述来源能支持的内容。包括背景、过程、影响、史料讨论、独立练习和回顾，开放问题给出合理答案与依据。"},
		{"chinese", "静夜思中的意象与情感", "五年级", "语文", "完整课堂，作者李白，准确引用原文并区分原文与解释；让学生依据诗句理解思乡情感，不虚构作者经历或唯一开放题答案。"},
		{"chemistry", "化学反应中的质量守恒", "九年级", "化学", "完整课堂，区分质量与原子数；用配平的简单分子方程说明，不安排危险实验，题目条件完整、答案与推导放备注。"},
		{"biology", "植物细胞和动物细胞的结构", "七年级", "生物", "通过结构图观察比较典型植物和动物细胞，区分共有结构和主要差异；示意图明确标识，不把模型类比当真实显微照片。包括观察任务、独立练习和回顾。"},
		{"geography", "水循环及其地理意义", "七年级", "地理", "区分蒸发、凝结、降水、径流和下渗，用图示解释过程及相互联系；不要把自然过程画成所有水必须经过的唯一固定路线。包括读图、解释及独立练习。"},
		{"english", "一般过去时的叙述与表达", "七年级", "英语", "用明确过去时间情境练习一般过去时，兼顾规则动词、不规则动词与did疑问否定；语法说明有语境和例句，包含独立表达任务及备注答案。"},
	} {
		if selected := os.Getenv("TEST_FULL_DECK_TOPIC"); selected != "" && selected != tc.key {
			continue
		}
		t.Run(tc.key, func(t *testing.T) {
			s := worksServer(t)
			resumeID := os.Getenv("TEST_FULL_DECK_RESUME_ID")
			token := worksLogin(t, s, "teacher")
			var a Actor
			json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
			dir, err := filepath.Abs(filepath.Join("../work/courseware-full-quality", tc.key, env("TEST_FULL_DECK_RUN", "baseline")))
			if err != nil {
				t.Fatal(err)
			}
			if err = os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			s.cfg.Storage = filepath.Join(dir, "artifacts")
			s.cfg.Mode = "live"
			s.cfg.CoursewareReviewPolicy = env("TEST_FULL_DECK_POLICY", "all")
			s.cfg.CoursewareContentReviewDisabled = false
			s.cfg.SearchURL, s.cfg.SearchKey = env("SEARCH_API_URL", "https://api.tavily.com/search"), os.Getenv("SEARCH_API_KEY")
			s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = os.Getenv("LLM_BASE_URL"), os.Getenv("LLM_API_KEY"), os.Getenv("LLM_MODEL")
			s.cfg.CoursewareLLMURL, s.cfg.CoursewareLLMKey, s.cfg.CoursewareLLMModel = os.Getenv("COURSEWARE_LLM_BASE_URL"), os.Getenv("COURSEWARE_LLM_API_KEY"), os.Getenv("COURSEWARE_LLM_MODEL")
			s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = os.Getenv("EVAL_LLM_BASE_URL"), os.Getenv("EVAL_LLM_API_KEY"), os.Getenv("EVAL_LLM_MODEL")
			s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = os.Getenv("VISION_LLM_BASE_URL"), os.Getenv("VISION_LLM_API_KEY"), os.Getenv("VISION_LLM_MODEL")
			if !s.agentConfigured("courseware") || !s.coursewareQualityConfigured() || s.cfg.RenderURL == "" {
				t.Fatal("actual generation, dual review and renderer required")
			}
			// Keep exact renderer inputs, including rejected/locally repaired versions.
			target, err := url.Parse(s.cfg.RenderURL)
			if err != nil {
				t.Fatal(err)
			}
			proxy := httputil.NewSingleHostReverseProxy(target)
			var renders atomic.Int32
			renderer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/render/pptx" {
					body, e := io.ReadAll(r.Body)
					if e != nil {
						t.Error(e)
						w.WriteHeader(500)
						return
					}
					r.Body = io.NopCloser(bytes.NewReader(body))
					if e = os.WriteFile(filepath.Join(dir, fmt.Sprintf("render-%03d.json", renders.Add(1))), body, 0600); e != nil {
						t.Error(e)
					}
				}
				proxy.ServeHTTP(w, r)
			}))
			defer renderer.Close()
			s.cfg.RenderURL = renderer.URL
			// Capture successful model envelopes without headers or credentials so
			// empty/invalid replies can be reproduced instead of guessed from logs.
			transport := s.http.Transport
			if transport == nil {
				transport = http.DefaultTransport
			}
			var replies atomic.Int32
			capture := func(transport http.RoundTripper) http.RoundTripper {
				return coursewareTransportFunc(func(r *http.Request) (*http.Response, error) {
					res, e := transport.RoundTrip(r)
					if e != nil || res.StatusCode < 200 || res.StatusCode >= 300 || !strings.HasSuffix(r.URL.Path, "/chat/completions") {
						return res, e
					}
					b, e := io.ReadAll(res.Body)
					res.Body.Close()
					res.Body = io.NopCloser(bytes.NewReader(b))
					if e != nil {
						return res, e
					}
					var envelope struct {
						Model   string          `json:"model"`
						Choices json.RawMessage `json:"choices"`
						Usage   json.RawMessage `json:"usage"`
					}
					if json.Unmarshal(b, &envelope) == nil {
						os.WriteFile(filepath.Join(dir, fmt.Sprintf("model-%03d.json", replies.Add(1))), asJSON(envelope), 0600)
					}
					return res, nil
				})
			}
			s.http.Transport = capture(transport)
			s.reviewHTTP.Transport = capture(s.reviewHTTP.Transport)
			// Optional verified, actually readable first-party passages. No fabricated
			// textbook excerpts; when absent this is explicitly an unsupported draft.
			if resumeID == "" {
				seedTx, err := s.tx(context.Background(), a.TenantID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = seedTx.Exec(context.Background(), "DELETE FROM documents"); err != nil {
					seedTx.Rollback(context.Background())
					t.Fatal(err)
				}
				if err = seedTx.Commit(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			var sources []struct{ Topic, Title, URL, Content string }
			if b, e := os.ReadFile("../work/courseware-full-quality/verified-sources.json"); e == nil {
				if e = json.Unmarshal(b, &sources); e != nil {
					t.Fatal(e)
				}
			}
			useKB := true
			for _, src := range sources {
				if resumeID != "" {
					break
				}
				if src.Topic != tc.topic {
					continue
				}
				if _, e := s.saveDocument(context.Background(), a, src.Title, src.URL, []Page{{Page: 1, Text: "核验主题标签（不是教材原文）：" + src.Topic + "\n以下为第一方英文原文摘录（不是人教教材）：\n" + src.Content}}); e != nil {
					t.Fatal(e)
				}
				useKB = true
			}
			before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
			in := TaskInput{Type: "courseware", Topic: tc.topic, Subject: tc.subject, Grade: tc.grade, Textbook: "", Detail: "default", LessonMinutes: 45, Theme: "ocean", PreviewFirst: true, UseKB: useKB, UseWeb: os.Getenv("TEST_FULL_DECK_WEB") == "1", IdempotencyKey: newID(), Requirements: "教师讲授课件；依据实际已提供资料，未提供教材原文时不得声称已核对教材章节。" + tc.requirements}
			if in.UseWeb && s.cfg.SearchKey == "" {
				t.Fatal("real web/image search credentials are required")
			}
			if material := os.Getenv("TEST_FULL_DECK_MATERIAL"); material != "" && resumeID == "" {
				in.MaterialID = uploadKnowledgeImageFixture(t, s, worksLogin(t, s, "admin"), material)
				if err := s.ingestOne(context.Background()); err != nil {
					t.Fatal(err)
				}
				worksCall(t, s, "GET", "/api/documents/"+in.MaterialID+"/images", token, nil, 200)
			}
			if os.Getenv("TEST_FULL_DECK_MICRO") == "1" {
				in.Pages, in.LessonMinutes, in.Detail = 5, 15, "concise"
				in.Requirements += "严格制作五页15分钟微课，包含情境、概念理解、解释、独立练习及回顾，避免无必要图表。"
			}
			id := resumeID
			if id == "" {
				id = worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
			} else {
				v := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
				if v["status"] != "failed" && v["status"] != "cancelled" {
					t.Fatal("resume test requires a saved terminal checkpoint; never hijack an active worker")
				}
				if v["status"] == "cancelled" {
					worksCall(t, s, "POST", "/api/tasks/"+id+"/restore", token, nil, 200)
				}
				if resumed := worksObject(t, worksCall(t, s, "POST", "/api/tasks/"+id+"/retry", token, nil, 202))["id"]; resumed != id {
					t.Fatal("resume created a duplicate work")
				}
			}
			route := "/api/tasks/" + id
			started := time.Now()
			defer func() {
				tx, e := s.tx(context.Background(), a.TenantID)
				if e != nil {
					t.Error(e)
					return
				}
				defer tx.Rollback(context.Background())
				rows, e := queryMaps(context.Background(), tx, "SELECT status,stage,input,result,sources,error,charge_state,automatic_failures FROM tasks WHERE id=$1", id)
				if e != nil {
					t.Error(e)
					return
				}
				events, e := queryMaps(context.Background(), tx, "SELECT stage,message,created_at FROM task_events WHERE task_id=$1 ORDER BY id", id)
				if e != nil {
					t.Error(e)
					return
				}
				usage, e := queryMaps(context.Background(), tx, "SELECT operation,model,input_tokens,output_tokens,latency_ms,success FROM usage_logs WHERE task_id=$1 ORDER BY id", id)
				if e != nil {
					t.Error(e)
					return
				}
				_, _, model := s.agentModel("courseware")
				var saved struct{ Result Deck }
				json.Unmarshal(worksCall(t, s, "GET", route, token, nil, 200).Body.Bytes(), &saved)
				imageSources := []map[string]any{}
				for _, sl := range saved.Result.Slides {
					if sl.Picture == nil || sl.Picture.AssetID == "" {
						continue
					}
					asset, err := s.ownedMedia(context.Background(), a, sl.Picture.AssetID)
					if err != nil {
						t.Error("adopted image asset unavailable", err)
						continue
					}
					imageSources = append(imageSources, map[string]any{"order": sl.Order, "asset_id": asset.ID, "provenance": asset.Provenance})
				}
				if e = os.WriteFile(filepath.Join(dir, "image-sources.json"), asJSON(imageSources), 0600); e != nil {
					t.Error(e)
				}
				if e = os.WriteFile(filepath.Join(dir, "report.json"), asJSON(map[string]any{"model": model, "review_model": s.cfg.ReviewModel, "vision_model": s.cfg.VisionModel, "mode": "live", "passed": !t.Failed(), "seconds": time.Since(started).Seconds(), "task": rows, "events": events, "usage": usage, "test_teacher_approval": true, "human_quality_accepted": false, "render_calls": renders.Load(), "fail_fast": os.Getenv("TEST_FULL_DECK_FAIL_FAST") == "1", "closure_required": os.Getenv("TEST_FULL_DECK_CLOSURE") == "1", "image_required": os.Getenv("TEST_FULL_DECK_REQUIRE_IMAGE") == "1", "content_review_enabled": !s.cfg.CoursewareContentReviewDisabled, "review_policy": s.cfg.CoursewareReviewPolicy}), 0600); e != nil {
					t.Error(e)
				}
			}()
			defer func() { worksCall(t, s, "POST", route+"/cancel", token, nil, 200) }()
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
			defer cancel()
			advance := func(want string) map[string]any {
				for round := 0; round < 4; round++ {
					if err := s.workOne(ctx); err != nil {
						t.Fatal(err)
					}
					v := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
					os.WriteFile(filepath.Join(dir, "checkpoint.json"), asJSON(v), 0600)
					var d Deck
					json.Unmarshal(asJSON(v["result"]), &d)
					t.Logf("stage=%v pages=%d elapsed=%.1fs", v["stage"], len(d.Slides), time.Since(started).Seconds())
					if v["status"] == want {
						return v
					}
					if v["stage"] != "auto_recovering" {
						t.Fatalf("expected %s, got %v: %v", want, v["status"], v["error"])
					}
					if os.Getenv("TEST_FULL_DECK_FAIL_FAST") == "1" {
						t.Fatal("first failed live attempt retained; no further paid automatic retries in this probe")
					}
					select {
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					case <-time.After(coursewareResumeDelay(round+1) + time.Second):
					}
				}
				t.Fatal("isolated run exhausted bounded automatic recovery; checkpoints retained")
				return nil
			}
			var preview map[string]any
			if resumeID == "" {
				preview = advance("awaiting_preview")
			} else {
				preview = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
			}
			var d Deck
			json.Unmarshal(asJSON(preview["result"]), &d)
			if (resumeID == "" && len(d.Slides) != 3) || len(d.Slides) < 3 || d.Plan == nil {
				t.Fatal("missing genuinely generated representative preview")
			}
			locked := map[string]string{}
			approve := func(v map[string]any, d Deck) {
				for _, sl := range d.Slides {
					if !s.pageCheckPassed(d.PageChecks[sl.ID], false, sl) {
						t.Fatal("page lacks actual dual-review success", sl.Order)
					}
					worksCall(t, s, "POST", route+"/slides/"+sl.ID+"/review", token, map[string]any{"revision": v["revision"], "hash": slideContentHash(d, sl), "approved": true}, 200)
				}
			}
			for _, sl := range d.Slides[:3] {
				locked[sl.ID] = slideContentHash(d, sl)
			}
			if resumeID != "" {
				for _, sl := range d.Slides {
					if s.pageCheckPassed(d.PageChecks[sl.ID], false, sl) {
						locked[sl.ID] = slideContentHash(d, sl)
					}
				}
			}
			if resumeID == "" {
				approve(preview, d)
				worksCall(t, s, "POST", route+"/expand", token, map[string]any{"revision": preview["revision"]}, 202)
			}
			full := advance("awaiting_confirmation")
			d = Deck{}
			json.Unmarshal(asJSON(full["result"]), &d)
			if len(d.Slides) != len(d.Plan.Pages) {
				t.Fatal("full deck not completed")
			}
			for i, sl := range d.Slides {
				if sl.Order != i+1 {
					t.Fatal("incorrect teaching order")
				}
				if hash, ok := locked[sl.ID]; ok {
					if hash != slideContentHash(d, sl) {
						t.Fatal("approved preview overwritten")
					}
					delete(locked, sl.ID)
				}
			}
			if len(locked) != 0 {
				t.Fatal("approved preview lost")
			}
			if issues := coursewareQualityIssues(d, len(d.Slides)); len(issues) > 0 {
				t.Fatal(issues)
			}
			if d.Plan == nil || len(d.Plan.LearningGoals) == 0 {
				t.Fatal("learning goals missing from real full deck")
			}
			if err := validateGoalMapping(*d.Plan); err != nil {
				t.Fatal(err)
			}
			if d.DeckCheck == nil || d.DeckCheck.Status != "checked" || d.DeckCheck.Hash != deckContentHash(d) {
				t.Fatalf("current full-deck real review did not pass: %+v", d.DeckCheck)
			}
			if os.Getenv("TEST_FULL_DECK_REQUIRE_IMAGE") == "1" {
				images := 0
				for _, sl := range d.Slides {
					if sl.Picture == nil || sl.Picture.AssetID == "" {
						continue
					}
					asset, err := s.ownedMedia(ctx, a, sl.Picture.AssetID)
					if err != nil || asset.Provenance == nil || asset.Provenance.SHA256 == "" || asset.Provenance.PageURL == "" {
						t.Fatal("adopted image lacks actual owned source snapshot", err)
					}
					images++
				}
				if images == 0 {
					t.Fatal("no sourced image automatically inserted; image fallback is not successful image acceptance")
				}
			}
			if os.Getenv("TEST_FULL_DECK_CLOSURE") == "1" {
				beforeHash := d.DeckCheck.Hash
				d.Slides[len(d.Slides)-1].Notes += "\n教师手工修改：课堂结束时留出一分钟，请学生用自己的话回顾本课目标。"
				worksCall(t, s, "POST", route+"/draft", token, map[string]any{"revision": full["revision"], "result": d}, 200)
				full = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
				d = Deck{}
				json.Unmarshal(asJSON(full["result"]), &d)
				if d.DeckCheck == nil || d.DeckCheck.Hash == beforeHash || d.DeckCheck.Hash != deckContentHash(d) || d.DeckCheck.Status != "checked" {
					t.Fatalf("manual edit not independently rechecked: %+v", d.DeckCheck)
				}
			}
			approve(full, d)
			worksCall(t, s, "POST", route+"/confirm", token, map[string]any{"result": d, "revision": full["revision"], "accept_review": true}, 202)
			advance("succeeded")
			file := worksCall(t, s, "GET", route+"/download/pptx", token, nil, 200).Body.Bytes()
			if len(file) < 1000 || !bytes.HasPrefix(file, []byte("PK")) {
				t.Fatal("missing real PPTX")
			}
			if err := os.WriteFile(filepath.Join(dir, "complete.pptx"), file, 0600); err != nil {
				t.Fatal(err)
			}
			after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
			if after["available"].(float64) != before["available"].(float64)-20 || after["reserved"].(float64) != 0 {
				t.Fatal("incorrect exactly-once settlement")
			}
			t.Logf("live %s complete: %d pages; test approvals only, screenshots require human quality assessment", strings.TrimSpace(tc.topic), len(d.Slides))
		})
	}
}
