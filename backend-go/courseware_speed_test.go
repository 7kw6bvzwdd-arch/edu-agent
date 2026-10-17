package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCoursewareRenderReuseAndInvalidation(t *testing.T) {
	var calls atomic.Int32
	renderer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var d Deck
		json.NewDecoder(r.Body).Decode(&d)
		w.Write([]byte("PPTX:" + d.Slides[0].Notes))
	}))
	defer renderer.Close()
	s := &Server{cfg: Config{RenderURL: renderer.URL}, http: renderer.Client()}
	ctx := withCoursewareArtifacts(context.Background())
	d := Deck{Title: "一次函数", Theme: "ocean", Slides: []Slide{{Title: "斜率", Bullets: []string{"k决定倾斜方向"}, Notes: "斜率为2"}}}
	first, err := s.renderCoursewareAvailable(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	d.Lesson = "重建教案"
	d.Generation = &CoursewareCheckpoint{Phase: "preview"}
	d.PageChecks = map[string]PageQualityCheck{"p": {Content: "checked"}}
	second, err := s.renderCoursewareAvailable(ctx, d)
	if err != nil || string(first) != string(second) || calls.Load() != 1 {
		t.Fatalf("same page was rendered again: calls=%d err=%v", calls.Load(), err)
	}
	d.Slides[0].Notes = "斜率为3"
	third, err := s.renderCoursewareAvailable(ctx, d)
	if err != nil || string(first) == string(third) || calls.Load() != 2 {
		t.Fatal("changed content reused stale render", err, calls.Load())
	}
	if _, err = s.renderCoursewareAvailable(withCoursewareArtifacts(context.Background()), d); err != nil || calls.Load() != 3 {
		t.Fatal("another request reused private cache")
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, err = s.renderCoursewareAvailable(canceled, d); err != context.Canceled || calls.Load() != 3 {
		t.Fatal("canceled request returned cached success", err)
	}
}

func TestCoursewarePromptRetainsEvidenceOnce(t *testing.T) {
	original := "教材明确给出的唯一原文：y=kx+b，k不为零。"
	in := TaskInput{MaterialID: "book", Textbook: "人教版", Chapter: "一次函数", Curriculum: []Page{{Page: 3, Text: original}}}
	sources := appendTeachingSources(in, nil)
	prompt := coursewarePrompt(in, sources)
	if strings.Count(prompt, original) != 1 || !strings.Contains(prompt, "S1") || !strings.Contains(prompt, "文件第3页") {
		t.Fatal("evidence lost or duplicated")
	}
	if in.Curriculum[0].Text != original {
		t.Fatal("prompt optimization mutated snapshot")
	}
	withoutSources := coursewarePrompt(in, nil)
	if !strings.Contains(withoutSources, original) {
		t.Fatal("material omitted without source copy")
	}
	d := Deck{Title: "函数", Theme: "ocean", Slides: []Slide{{ID: "private-id", Title: "斜率", Bullets: []string{"斜率"}, Answer: "答案为2", Notes: "条件"}}}
	first := coursewareRenderKey(d)
	d.NeedsReview = true
	if coursewareRenderKey(d) == first {
		t.Fatal("footer change did not invalidate render")
	}
	first = coursewareRenderKey(d)
	d.Slides[0].Answer = "答案为3"
	if coursewareRenderKey(d) == first {
		t.Fatal("answer change did not invalidate render")
	}
	first = coursewareRenderKey(d)
	d.Slides[0].ID = "assigned-later"
	d.Slides[0].Order = 5
	if coursewareRenderKey(d) != first {
		t.Fatal("non-rendered identity invalidated render")
	}
	ctx := withCoursewareArtifacts(context.Background())
	artifactCachePut(ctx, "oversized", make([]byte, (64<<20)+1))
	if artifactCacheGet(ctx, "oversized") != nil {
		t.Fatal("cache exceeded capacity")
	}
}

func TestGraphDetailPreservesActualPixelsAndCalculations(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 100, 100))
	source.Set(50, 50, color.RGBA{R: 17, G: 45, B: 90, A: 255})
	var buf bytes.Buffer
	png.Encode(&buf, source)
	d := Deck{Slides: []Slide{{Layout: "steps", Graph: &FunctionGraph{Series: []FunctionSeries{{K: 2, B: 1, MarkX: []float64{0, 1}}, {Kind: "quadratic", A: 1, B: 2, C: 3, MarkX: []float64{2}}, {Kind: "inverse", K: 6, MarkX: []float64{3}}}}}}}
	detail, err := png.Decode(bytes.NewReader(coursewareGraphDetail(d, buf.Bytes())))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := detail.At(6, 40).RGBA()
	if r != 17*257 || g != 45*257 || b != 90*257 {
		t.Fatal("crop changed actual pixels")
	}
	d.Slides[0].Layout = "concept"
	detail, err = png.Decode(bytes.NewReader(coursewareGraphDetail(d, buf.Bytes())))
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ = detail.At(90, 40).RGBA()
	if r != 17*257 || g != 45*257 || b != 90*257 {
		t.Fatal("evidence-first crop lost actual graph pixels")
	}
	facts := teachingRenderEvidence(d)
	for _, point := range []string{"(0,1)", "(1,3)", "(2,11)", "(3,2)"} {
		if !strings.Contains(facts, point) {
			t.Fatal("incorrect calculated evidence", facts)
		}
	}
	if coursewareGraphDetail(d, []byte("invalid")) != nil {
		t.Fatal("invalid image accepted")
	}
}

func TestCoursewareSnapshotReuseActualPPTX(t *testing.T) {
	if runtime.GOOS != "windows" || os.Getenv("TEST_RENDER_URL") == "" {
		t.Skip("requires actual Windows Office and renderer")
	}
	s := &Server{cfg: Config{Storage: t.TempDir(), RenderURL: os.Getenv("TEST_RENDER_URL")}, http: &http.Client{Timeout: 30 * time.Second}}
	ctx := withCoursewareArtifacts(context.Background())
	d := sampleDeck(TaskInput{Topic: "浮力", Theme: "ocean", Pages: 8})
	d.Slides = d.Slides[1:2]
	b, err := s.renderCoursewareAvailable(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(s.cfg.Storage, "first.pptx")
	os.WriteFile(first, b, 0600)
	start := time.Now()
	if !s.snapshotCourseware(ctx, first) {
		t.Fatal("actual Office export failed")
	}
	cold := time.Since(start)
	second := filepath.Join(s.cfg.Storage, "second.pptx")
	os.WriteFile(second, b, 0600)
	start = time.Now()
	if !s.snapshotCourseware(ctx, second) {
		t.Fatal("snapshot reuse failed")
	}
	warm := time.Since(start)
	a, _ := os.ReadFile(first + ".slides/slide-1.png")
	c, _ := os.ReadFile(second + ".slides/slide-1.png")
	if len(a) < 1000 || string(a) != string(c) {
		t.Fatal("reused screenshot differs")
	}
	if warm >= cold {
		t.Fatal("snapshot reuse did not reduce latency", cold, warm)
	}
	cancel, stop := context.WithCancel(ctx)
	stop()
	if s.snapshotCourseware(cancel, second) {
		t.Fatal("canceled cached screenshot returned success")
	}
	t.Logf("actual Office export cold=%s cached=%s", cold, warm)
}

// Synthetic providers and screenshot isolate scheduling from Office/model speed.
// A separate test below verifies reuse against a real PPTX/Office export.
func TestCoursewareLookaheadRepairAndCancellationHTTP(t *testing.T) {
	for _, mode := range []string{"overlap", "repair", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			s := worksServer(t)
			token := worksLogin(t, s, "teacher")
			in := TaskInput{Type: "courseware", Topic: "浮力", Subject: "物理", Detail: "concise", LessonMinutes: 45, Theme: "ocean", UseKB: true, PreviewFirst: true, IdempotencyKey: newID()}
			id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
			route := "/api/tasks/" + id
			defer worksCall(t, s, "POST", route+"/cancel", token, nil, 200)
			var a Actor
			json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
			plan, err := s.planCourseware(context.Background(), a, id, in, nil)
			if err != nil {
				t.Fatal(err)
			}
			in.Pages = len(plan.Pages)
			for i := range plan.Pages {
				plan.Pages[i].Preview = i >= 1 && i <= 3
			}
			tx, err := s.tx(context.Background(), a.TenantID)
			if err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(context.Background(), "UPDATE tasks SET status='researching',mode='live',sources='[]' WHERE id=$1", id)
			if err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(context.Background()); err != nil {
				t.Fatal(err)
			}
			before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
			firstPosition, secondPosition := plannedSchedule(plan)[0].Position, plannedSchedule(plan)[1].Position
			nextStarted := make(chan struct{}, 10)
			aborted := make(chan struct{}, 10)
			var firstCalls, nextCalls, visualCalls atomic.Int32
			var earlySaved atomic.Bool
			var stale atomic.Bool
			var timedOut atomic.Bool
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Model    string
					Messages []struct{ Content any }
				}
				json.NewDecoder(r.Body).Decode(&req)
				raw := `{"issues":[]}`
				if req.Model == "generation" {
					prompt := req.Messages[len(req.Messages)-1].Content.(string)
					var position, end int
					fmt.Sscanf(regexp.MustCompile(`本批只返回第\d+至第\d+页`).FindString(prompt), "本批只返回第%d至第%d页", &position, &end)
					if position < 1 || position > in.Pages || position != end {
						t.Error("nonlocal generation")
						w.WriteHeader(400)
						return
					}
					d := sampleDeck(in)
					d.Slides = d.Slides[position-1 : position]
					if position == firstPosition && firstCalls.Add(1) > 1 {
						d.Slides[0].Notes += " 修复后有效条件"
					}
					if position == secondPosition {
						n := nextCalls.Add(1)
						nextStarted <- struct{}{}
						if (mode == "repair" || mode == "cancel") && n == 1 {
							select {
							case <-r.Context().Done():
								aborted <- struct{}{}
							case <-time.After(20 * time.Second):
								timedOut.Store(true)
							}
							return
						}
						if mode == "repair" && n > 1 && !strings.Contains(prompt, "修复后有效条件") {
							stale.Store(true)
						}
					}
					raw = string(asJSON(d))
				} else if req.Model == "vision" {
					if visualCalls.Add(1) == 1 {
						select {
						case <-nextStarted:
						// Actual isolated remote PostgreSQL now also reserves the
						// supplier call. Assert overlap, not a 2s network SLA.
						case <-time.After(10 * time.Second):
							timedOut.Store(true)
						}
						tx, e := s.tx(context.Background(), a.TenantID)
						if e == nil {
							var current Deck
							tx.QueryRow(context.Background(), "SELECT result FROM tasks WHERE id=$1", id).Scan(&current)
							earlySaved.Store(len(current.Slides) > 0)
							tx.Rollback(context.Background())
						}
						if mode == "repair" {
							raw = `{"issues":[{"object":"图表条件","category":"render_mismatch","certainty":"confirmed","location":"右侧图下方条件文字","observed":"输入中的条件未显示","expected":"完整显示图表条件","input_path":"/slides/0/bullets/0","observed_value":"","evidence":"必要条件未显示","severity":"error","repair":"补充修复后有效条件"}]}`
						}
						if mode == "cancel" {
							rr := httptest.NewRequest("POST", route+"/cancel", nil)
							rr.Header.Set("Authorization", "Bearer "+token)
							ww := httptest.NewRecorder()
							s.Handler().ServeHTTP(ww, rr)
							if ww.Code != 200 {
								t.Error("cancel failed", ww.Code)
							}
						}
					}
				}
				respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": raw}}}})
			}))
			defer provider.Close()
			renderer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/health" {
					respond(w, 200, map[string]string{"courseware_media_version": "media-v1", "courseware_graph_version": "graph-v1"})
					return
				}
				w.Write([]byte("scheduling-fixture-pptx"))
			}))
			defer renderer.Close()
			s.cfg.Mode = "live"
			s.cfg.RenderURL = renderer.URL
			s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = provider.URL, "fixture", "generation"
			s.cfg.ReviewURL, s.cfg.ReviewKey, s.cfg.ReviewModel = provider.URL, "fixture", "review"
			s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = provider.URL, "fixture", "vision"
			ctx, stop := context.WithTimeout(withCoursewareArtifacts(context.Background()), 2*time.Minute)
			defer stop()
			artifactCachePut(ctx, fmt.Sprintf("office:%x:0", sha256.Sum256([]byte("scheduling-fixture-pptx"))), asJSON(map[string][]byte{"slide-1.png": []byte("synthetic-image-for-scheduling-test")}))
			d, err := s.stagedCoursewarePlan(ctx, a, id, in, nil, Deck{Plan: plan})
			if timedOut.Load() {
				t.Fatal("next page did not overlap current visual review or cancellation did not stop request")
			}
			if earlySaved.Load() {
				t.Fatal("unreviewed page persisted before visual review")
			}
			if mode == "cancel" {
				v := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
				var saved Deck
				json.Unmarshal(asJSON(v["result"]), &saved)
				if err == nil || len(saved.Slides) != 0 || v["status"] != "cancelled" {
					t.Fatal("cancel committed unreviewed page", err, len(saved.Slides), v["status"])
				}
				select {
				case <-aborted:
				default:
					t.Fatal("cancel did not abort speculative generation")
				}
				return
			}
			if err != nil || len(d.Slides) != 3 {
				t.Fatal("preview incomplete", err, len(d.Slides))
			}
			if stale.Load() {
				t.Fatal("repair reused stale adjacent page")
			}
			if mode == "repair" {
				select {
				case <-aborted:
				default:
					t.Fatal("speculative request not aborted on repair")
				}
				if firstCalls.Load() != 2 || nextCalls.Load() != 2 {
					t.Fatal("unexpected repair calls", firstCalls.Load(), nextCalls.Load())
				}
			}
			if mode == "overlap" && nextCalls.Load() != 1 {
				t.Fatal("prefetched page generated twice", nextCalls.Load())
			}
			if after := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String(); after != before {
				t.Fatal("pipeline changed business points")
			}
			for _, sl := range d.Slides {
				if !pageCheckPassed(d.PageChecks[sl.ID], false) {
					t.Fatal("page skipped review")
				}
			}
		})
	}
}
