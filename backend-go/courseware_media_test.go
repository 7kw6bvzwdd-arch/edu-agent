package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func teachingTestPicture(t *testing.T) *TeachingPicture {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 80, 40))); err != nil {
		t.Fatal(err)
	}
	return &TeachingPicture{DataURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes()), Caption: "教师自绘示意", Source: "测试用自绘图片"}
}

func TestTeachingPictureRewritePreservesImageHTTP(t *testing.T) {
	s := worksServer(t)
	owner := worksLogin(t, s, "teacher")
	id := worksFixture(t, s, owner, "courseware", "succeeded", "浮力", TaskInput{})
	var a Actor
	if err := json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", owner, nil, 200).Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	d := sampleDeck(TaskInput{Topic: "浮力", Pages: 8, Theme: "classroom"})
	d.Slides[0].Picture = teachingTestPicture(t)
	original := string(asJSON(d.Slides[0].Picture))
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), "UPDATE tasks SET result=$1,mode='live' WHERE id=$2", asJSON(d), id)
	if err != nil {
		tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		raw, e := io.ReadAll(r.Body)
		if e != nil {
			t.Error(e)
		}
		if bytes.Contains(raw, []byte(d.Slides[0].Picture.DataURL)) {
			t.Error("rewrite model received image binary")
		}
		rewritten := directHTTPFixtureDeck(d).Slides[0]
		rewritten.HTML = strings.Replace(rewritten.HTML, rewritten.Title, "观察浮力方向", 1)
		rewritten.Title = "观察浮力方向"
		rewritten.HTML += `<img data-teaching-picture alt="教师自绘示意" style="position:absolute;left:500px;top:330px;width:400px;height:200px;object-fit:contain">`
		rewritten.Picture = &TeachingPicture{DataURL: "https://example.com/fabricated.png", Caption: "模型伪造", Source: "伪造"}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(asJSON(rewritten))}}}})
	}))
	defer provider.Close()
	s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "test-key", "fixture-model"
	route := "/api/tasks/" + id
	child := worksObject(t, worksCall(t, s, "POST", route+"/rewrite", owner, map[string]any{"revision": 0, "slide": 0, "instruction": "改进标题，保留原图", "idempotency_key": newID()}, 202))["id"].(string)
	if err = s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	result := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+child, owner, nil, 200))
	if result["status"] != "succeeded" || calls != 1 {
		t.Fatalf("rewrite failed: status=%v error=%v calls=%d events=%v", result["status"], result["error"], calls, result["events"])
	}
	var saved Deck
	parent := worksObject(t, worksCall(t, s, "GET", route, owner, nil, 200))
	if err = json.Unmarshal(asJSON(parent["result"]), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Slides[0].Title != "观察浮力方向" || string(asJSON(saved.Slides[0].Picture)) != original {
		t.Fatal("rewrite lost or replaced original image")
	}
	file := worksCall(t, s, "GET", route+"/download/pptx", owner, nil, 200).Body.Bytes()
	if len(file) < 1000 || string(file[:2]) != "PK" {
		t.Fatal("rewrite did not export real PPTX")
	}
}

func TestTeachingPictureSanitizationAndBoundaries(t *testing.T) {
	d := sampleDeck(TaskInput{Topic: "浮力", Pages: 8, Theme: "classroom"})
	d.Slides[0].Picture = teachingTestPicture(t)
	original := d.Slides[0].Picture.DataURL
	safe := deckForTextModel(d)
	if strings.Contains(string(asJSON(safe)), original) || d.Slides[0].Picture.DataURL != original || safe.Slides[0].Picture == d.Slides[0].Picture {
		t.Fatal("text-model sanitization leaked or mutated picture")
	}
	if safe.Slides[0].Picture.Caption != d.Slides[0].Picture.Caption || safe.Slides[0].Picture.Source != d.Slides[0].Picture.Source {
		t.Fatal("image context lost")
	}
	if err := validateDeck(d); err != nil {
		t.Fatal(err)
	}
	for _, url := range []string{"https://example.com/x.png", "file:///private.png", "data:image/svg+xml;base64,PHN2Zz4=", "data:image/png;base64,invalid", strings.Repeat("x", 180001)} {
		d.Slides[0].Picture.DataURL = url
		if validateDeck(d) == nil {
			t.Fatal("invalid uploaded image accepted")
		}
	}
	d.Slides[0].Picture.DataURL = original
	d.Slides[0].Diagram = "triangle"
	if validateDeck(d) == nil {
		t.Fatal("conflicting picture and diagram accepted")
	}
}

func TestTeachingMediaRejectsStaleRenderer(t *testing.T) {
	version := ""
	renders := 0
	renderer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			respond(w, 200, map[string]string{"courseware_media_version": version})
			return
		}
		renders++
		w.Write([]byte("pptx"))
	}))
	defer renderer.Close()
	s := &Server{cfg: Config{RenderURL: renderer.URL}, http: renderer.Client()}
	d := Deck{Slides: []Slide{{Title: "区间", Focus: "观察区间"}}}
	if _, err := s.renderPPTX(context.Background(), d); err == nil || !strings.Contains(err.Error(), "尚未更新") || renders != 0 {
		t.Fatal("stale renderer accepted", err, renders)
	}
	version = "media-v1"
	if _, err := s.renderPPTX(context.Background(), d); err != nil || renders != 1 {
		t.Fatal("new renderer rejected", err, renders)
	}
}

func TestTeachingIntervalBoundaries(t *testing.T) {
	f := &TeachingFigure{Kind: "interval", Unit: "min", Values: []float64{0, 30, 32, 60}, Items: []FigureItem{{Label: "出发"}, {Label: "到站"}, {Label: "离站"}, {Label: "到达"}}}
	if err := validateTeachingMedia(Slide{Figure: f}); err != nil {
		t.Fatal(err)
	}
	var copy TeachingFigure
	if err := json.Unmarshal(asJSON(f), &copy); err != nil || len(copy.Values) != 4 || copy.Values[2] != 32 || copy.Unit != "min" {
		t.Fatal("lost interval data", err)
	}
	for _, values := range [][]float64{{0, 0, 32, 60}, {0, 32, 30, 60}, {0, 30, 60}, {0, 30, 32, math.Inf(1)}, {0, 30, 32, math.NaN()}, {-math.MaxFloat64, 0, 1, math.MaxFloat64}} {
		f.Values = values
		if validateTeachingMedia(Slide{Figure: f}) == nil {
			t.Fatal("invalid interval accepted", values)
		}
	}
	f.Values = []float64{0, 30, 32, 60}
	f.Unit = " "
	if validateTeachingMedia(Slide{Figure: f}) == nil {
		t.Fatal("empty unit accepted")
	}
}

func TestTeachingMediaRoundTripAndBoundaries(t *testing.T) {
	d := sampleDeck(TaskInput{Topic: "一次函数", Pages: 12, Theme: "forest"})
	var decoded Deck
	if err := json.Unmarshal(asJSON(d), &decoded); err != nil {
		t.Fatal(err)
	}
	if err := validateDeck(decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Slides[6].Table == nil || decoded.Slides[11].Figure == nil {
		t.Fatal("lost teaching evidence")
	}
	for _, mutate := range []func(*Slide){
		func(sl *Slide) { sl.Focus = strings.Repeat("字", 61) },
		func(sl *Slide) { sl.Table.Headers = []string{" "} },
		func(sl *Slide) { sl.Table.Rows = [][]string{{"wrong column count"}} },
		func(sl *Slide) { sl.Diagram = "triangle" },
		func(sl *Slide) {
			sl.Figure = &TeachingFigure{Kind: "arbitrary-code", Items: []FigureItem{{"a", ""}, {"b", ""}}}
		},
	} {
		var copy Deck
		json.Unmarshal(asJSON(d), &copy)
		mutate(&copy.Slides[6])
		if validateDeck(copy) == nil {
			t.Fatal("accepted invalid evidence")
		}
	}
	for i := range decoded.Slides {
		decoded.Slides[i].Figure = nil
		decoded.Slides[i].Table = nil
	}
	if strings.Contains(strings.Join(coursewareQualityIssues(decoded, len(decoded.Slides)), "；"), "至少2页教学图表") {
		t.Fatal("media quantity should not substitute for teaching value")
	}
}
