package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
)

// Migrate old generation-only HTTP fixtures to the current plan/preview/review
// workflow. Generation still goes to the supplied provider, with its original
// failures and call counters. Review is explicitly synthetic and disabled for
// content; visual checks still receive the real PPTX screenshot.
func configureDirectHTTPFixture(t *testing.T, s *Server, in TaskInput, provider string) {
	t.Helper()
	if in.Pages == 0 {
		in.Pages = 8
	}
	d := sampleDeck(in)
	plan := TeachingPlan{Summary: "固定接口验收规划，非真实模型结果。", Conventions: "保留本测试的页数、顺序和教学条件。", LearningGoals: []LearningGoal{{ID: "g1", Text: "理解本课概念并应用", Core: true}}}
	for i, sl := range d.Slides {
		row := TeachingPage{Position: i + 1, Goal: sl.Title, Task: sl.Layout, Visual: "固定接口验收画面", Minutes: 3, Preview: i < 3, GoalRefs: []string{"g1"}}
		if sl.Layout == "exercise" {
			row.ChecksGoals = []string{"g1"}
		}
		plan.Pages = append(plan.Pages, row)
	}
	u, err := url.Parse(provider)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		var req struct{ Messages []struct{ Content string } }
		_ = json.Unmarshal(raw, &req)
		if len(req.Messages) > 0 && strings.Contains(req.Messages[len(req.Messages)-1].Content, "规划符合实际学科") {
			respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(asJSON(plan))}}}})
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(gateway.Close)
	vision := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": `{"issues":[]}`}}}})
	}))
	t.Cleanup(vision.Close)
	s.cfg.LLMURL = gateway.URL
	s.cfg.CoursewareContentReviewDisabled = true
	s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = vision.URL, "fixture", "vision-fixture"
}

// Explicit synthetic HTTP fixture, never used by real generation or samples.
func directHTTPFixtureDeck(d Deck) Deck {
	d.Slides = append([]Slide(nil), d.Slides...)
	for i := range d.Slides {
		sl := &d.Slides[i]
		sl.HTML = `<style>body{margin:0;background:white;font-family:Arial}h1{position:absolute;left:60px;top:30px;font-size:38px}p{position:absolute;left:60px;top:160px;width:1100px;font-size:26px;line-height:1.5}</style><h1 data-editable="title">` + html.EscapeString(sl.Title) + `</h1><p data-editable="body">` + html.EscapeString(strings.Join(sl.Bullets, " ")+" "+sl.Formula) + `</p>`
		sl.Graph = nil
		sl.Chart = nil
		sl.Scientific = nil
		sl.Figure = nil
		sl.Table = nil
		sl.Diagram = ""
		sl.Formula = ""
	}
	return d
}

func TestDirectPagePurposeDoesNotImposeVisualLayout(t *testing.T) {
	d := Deck{Plan: &TeachingPlan{}}
	for i := 0; i < 8; i++ {
		d.Slides = append(d.Slides, Slide{Title: fmt.Sprintf("Concept %d", i), Layout: "concept", HTML: fmt.Sprintf(`<p data-editable="text">Different drawing %d</p>`, i), Notes: strings.Repeat("Teacher explanation with evidence. ", 3)})
	}
	for _, issue := range coursewareQualityIssues(d, 8) {
		if strings.Contains(issue, "版式") {
			t.Fatal("purpose was mistaken for visual layout", issue)
		}
	}
}

func TestDirectDeckSaveAcceptsBoundedAuthoredSource(t *testing.T) {
	raw := `{"slides":[{"html":"` + strings.Repeat("x", (2<<20)+1) + `"}]}`
	var d Deck
	if err := decodeCoursewareSave(httptest.NewRequest("POST", "/api/tasks/id/draft", strings.NewReader(raw)), &d); err != nil {
		t.Fatal("authored deck cannot be saved", err)
	}
	if err := decodeCoursewareSave(httptest.NewRequest("POST", "/api/tasks/id/draft", strings.NewReader(`{"html":"`+strings.Repeat("x", 32<<20)+`"}`)), &d); err == nil {
		t.Fatal("oversized save accepted")
	}
	var other map[string]any
	if err := decode(httptest.NewRequest("POST", "/api/tasks", strings.NewReader(raw)), &other); err == nil {
		t.Fatal("unrelated input limit changed")
	}
}

func TestDirectHTMLCannotSilentlyUseOldRenderer(t *testing.T) {
	renders := 0
	renderer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.Write([]byte(`{"status":"ok","courseware_media_version":"media-v1"}`))
			return
		}
		renders++
		w.Write([]byte("old-template"))
	}))
	defer renderer.Close()
	s := &Server{cfg: Config{RenderURL: renderer.URL}, http: renderer.Client()}
	_, err := s.renderCoursewareAvailable(context.Background(), Deck{Slides: []Slide{{Title: "Direct", HTML: `<p data-editable="x">Source</p>`}}})
	if err == nil || renders != 0 {
		t.Fatal("old renderer silently discarded authored source", err, renders)
	}
}

func TestDirectAuthorGenerationContract(t *testing.T) {
	p := coursewarePrompt(TaskInput{Pages: 3, Topic: "海陆风"}, nil)
	if !strings.Contains(p, "data-editable") || !strings.Contains(p, "html") {
		t.Fatal("courseware still uses a template contract")
	}
	if !strings.Contains(animAuthorInstructions(TaskInput{}, ""), "renderFrame") {
		t.Fatal("animation still uses component authoring")
	}
}

func TestDirectSourceValidationAndHashes(t *testing.T) {
	raw := []byte(`{"version":"anim-v2","title":"Direct","archetype":"html.direct@1","representative":"s1","scenes":[{"id":"s1","title":"Observe","intent":"Observe motion","html":"<svg><circle id='c' r='40'/></svg><script>window.renderFrame=f=>document.getElementById('c').setAttribute('cx',100+f.progress*500)</script>","narration":[{"id":"n1","display":"Observe","speech":"Observe"}],"holds":[]}]}`)
	sc, problems := decodeAnimScript(raw)
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	bytes := string(buildAnimPackage(animPackageInput{Script: sc, Voice: animVoiceEstimated}))
	if !strings.Contains(bytes, "sandbox") {
		t.Fatal("missing isolated source frame")
	}
	d := Deck{Title: "Direct", Theme: "classroom", Lesson: "Teacher notes", Slides: []Slide{{Title: "One", HTML: "<p data-editable='x'>One</p>"}}}
	if err := validateDeckMinimum(d, 1); err != nil {
		t.Fatal(err)
	}
	hash := slideContentHash(d, d.Slides[0])
	d.Slides[0].HTML += "<hr>"
	if hash == slideContentHash(d, d.Slides[0]) {
		t.Fatal("source changes do not invalidate approval")
	}
}

func TestModelCoursewareRequiresDirectSource(t *testing.T) {
	var d Deck
	if err := decodeAuthoredCourseware(`{"slides":[{"title":"Missing","bullets":["old template"]}]}`, &d); err == nil {
		t.Fatal("new model output silently used a fixed template")
	}
}

func TestDirectCoursewareDoesNotSilentlyDropOldVisuals(t *testing.T) {
	var d Deck
	raw := `{"slides":[{"title":"Mixed","html":"<p data-editable='x'>Diagram</p>","diagram":"buoyancy"}]}`
	if err := decodeAuthoredCourseware(raw, &d); err == nil {
		t.Fatal("old diagram would be silently omitted from direct page")
	}
}

func directModelFixture(t *testing.T) AnimScript {
	sc := loadLinearKFixture(t)
	sc.Archetype = "html.direct@1"
	for i := range sc.Scenes {
		sc.Scenes[i].Objects = nil
		sc.Scenes[i].Actions = nil
		sc.Scenes[i].HTML = `<svg width="1280" height="608"><circle id="point" cy="250" r="50" fill="#34a7a0"/></svg><script>window.renderFrame=f=>document.getElementById('point').setAttribute('cx',100+f.progress*800)</script>`
	}
	return sc
}

func TestAuthoredSVGLegalNamespace(t *testing.T) {
	for _, source := range []string{`<svg xmlns='http://www.w3.org/2000/svg'><text data-editable='x'>Cell</text></svg>`, `<svg xmlns = "http://www.w3.org/2000/svg"><text data-editable='x'>Cell</text></svg>`, `<svg id='s'></svg><script>window.renderFrame=f=>document.getElementById('s').append(document.createElementNS('http://www.w3.org/2000/svg','circle'))</script><p data-editable='x'>Cell</p>`} {
		if err := validateAuthoredHTML(source, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateAuthoredHTML(`<svg xmlns='http://www.w3.org/2000/svg'><image href='http://localhost/private'/><text data-editable='x'>Cell</text></svg>`, false); err == nil {
		t.Fatal("external resource accepted")
	}
	if err := validateAuthoredHTML(`<svg><image href='http://www.w3.org/2000/svg/private'/><text data-editable='x'>Cell</text></svg>`, false); err == nil {
		t.Fatal("namespace URL prefix bypassed offline restrictions")
	}
}

func TestDirectTeachingPlanAllowsComparativeDiagrams(t *testing.T) {
	p := TeachingPlan{Summary: "比较细胞结构", Pages: []TeachingPage{
		{Position: 1, Goal: "观察结构", Task: "并排观察植物和动物细胞两幅图", Visual: "左右细胞示意", Minutes: 5, Preview: true},
		{Position: 2, Goal: "比较", Task: "解释区别", Minutes: 5, Preview: true},
		{Position: 3, Goal: "应用", Task: "辨认细胞", Minutes: 5, Preview: true},
	}}
	if err := validateTeachingPlan(p); err != nil {
		t.Fatal(err)
	}
}

func TestDirectRendererHealthAvailability(t *testing.T) {
	for _, status := range []int{503, 429, 401} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			healthCalls, renders := 0, 0
			renderer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/health" {
					healthCalls++
					if healthCalls <= 2 {
						w.WriteHeader(status)
						return
					}
					respond(w, 200, map[string]any{"courseware_authored_html_version": "authored-html-v2", "isolated_browser_author_code": true})
					return
				}
				renders++
				w.Write([]byte("PK-test-artifact"))
			}))
			defer renderer.Close()
			s := &Server{cfg: Config{RenderURL: renderer.URL}, http: renderer.Client()}
			d := Deck{Slides: []Slide{{Title: "保留源码", HTML: `<p data-editable="x">已生成内容</p>`}}}
			before := string(asJSON(d))
			b, err := s.renderCoursewareAvailable(context.Background(), d)
			if status == 401 {
				if err == nil || healthCalls != 1 || renders != 0 {
					t.Fatal("permanent health failure retried", healthCalls, renders, err)
				}
			} else if err != nil || healthCalls != 3 || renders != 1 || !strings.HasPrefix(string(b), "PK") {
				t.Fatal("health outage confused with incompatible renderer", healthCalls, renders, err)
			}
			if before != string(asJSON(d)) {
				t.Fatal("health recovery changed source")
			}
		})
	}
}

func TestAuthoredAnimationSafeValidationReason(t *testing.T) {
	renderer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, 422, map[string]any{"detail": map[string]string{"code": "authored_animation_invalid", "reason": "invalid_geometry", "message": "private-provider-key"}})
	}))
	defer renderer.Close()
	s := &Server{cfg: Config{RenderURL: renderer.URL}, http: renderer.Client()}
	_, err := s.postBytes(context.Background(), renderer.URL+"/render/authored-animation-check", map[string]any{}, "")
	if err == nil || !strings.Contains(err.Error(), "NaN") || strings.Contains(err.Error(), "private-provider-key") {
		t.Fatal("missing safe actionable animation diagnostic", err)
	}
}
