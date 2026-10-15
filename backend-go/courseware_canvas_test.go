package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCoursewareCanvasValidationAndReviewInvalidation(t *testing.T) {
	d := sampleDeck(TaskInput{Topic: "浮力", Pages: 8, Theme: "forest"})
	assignSlideIDs(&d)
	d.Slides[0].Order = 1
	d.PageChecks = map[string]PageQualityCheck{d.Slides[0].ID: {Hash: slideContentHash(d, d.Slides[0]), Content: "checked", Visual: "checked"}}
	var changed Deck
	json.Unmarshal(asJSON(d), &changed)
	changed.Slides[0].CanvasPositions = map[string]CanvasOffset{"title": {DX: .01, DY: .01}}
	preserveCoursewareChecks(&changed, d)
	if len(changed.PageChecks) != 0 || slideContentHash(changed, changed.Slides[0]) == slideContentHash(d, d.Slides[0]) {
		t.Fatal("position change retained old approval")
	}
	if err := validateDeck(changed); err != nil {
		t.Fatal(err)
	}
	changed.Slides[0].CanvasPositions["invalid"] = CanvasOffset{}
	if validateDeck(changed) == nil {
		t.Fatal("unknown object accepted")
	}
}

func TestCoursewareCanvasSavePreviewDownloadIsolationHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "other")
	admin := worksLogin(t, s, "admin")
	id := worksFixture(t, s, token, "courseware", "succeeded", "对象移动", TaskInput{})
	path := "/api/tasks/" + id
	var initial struct{ Revision int }
	json.Unmarshal(worksCall(t, s, "GET", path, token, nil, 200).Body.Bytes(), &initial)
	base := initial.Revision
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	d := sampleDeck(TaskInput{Topic: "浮力", Pages: 8, Theme: "forest"})
	assignSlideIDs(&d)
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	worksCall(t, s, "POST", path+"/edit", token, map[string]any{"result": d, "revision": base}, 200)
	var saved struct {
		Result   Deck
		Revision int
	}
	json.Unmarshal(worksCall(t, s, "GET", path, token, nil, 200).Body.Bytes(), &saved)
	slide := saved.Result.Slides[0].ID
	objects := path + "/slides/" + slide + "/objects?revision=" + strconv.Itoa(base+1)
	mapResult := worksObject(t, worksCall(t, s, "GET", objects, token, nil, 200))
	if len(mapResult["objects"].([]any)) == 0 {
		t.Fatal("empty actual object map")
	}
	for _, foreign := range []string{other, admin} {
		worksCall(t, s, "GET", objects, foreign, nil, 404)
	}
	worksCall(t, s, "GET", path+"/slides/"+slide+"/objects?revision="+strconv.Itoa(base), token, nil, 409)
	worksCall(t, s, "GET", path+"/slides/missing/objects?revision="+strconv.Itoa(base+1), token, nil, 404)
	worksCall(t, s, "GET", objects, "", nil, 401)
	for _, sl := range saved.Result.Slides {
		worksCall(t, s, "POST", path+"/slides/"+sl.ID+"/review", token, map[string]any{"revision": base + 1, "hash": slideContentHash(saved.Result, sl), "approved": true}, 200)
	}
	scale := .9
	saved.Result.Slides[0].CanvasPositions = map[string]CanvasOffset{"title": {DX: .01, DY: .01, Scale: &scale}}
	worksCall(t, s, "POST", path+"/edit", token, map[string]any{"result": saved.Result, "revision": base + 1}, 200)
	var moved struct {
		Result       Deck
		Revision     int
		SlideReviews []slideReview `json:"slide_reviews"`
	}
	json.Unmarshal(worksCall(t, s, "GET", path, token, nil, 200).Body.Bytes(), &moved)
	if moved.Result.Slides[0].CanvasPositions["title"].DX != .01 || moved.Result.Slides[0].CanvasPositions["title"].Scale == nil || *moved.Result.Slides[0].CanvasPositions["title"].Scale != .9 || moved.SlideReviews[0].Approved || !moved.SlideReviews[1].Approved {
		t.Logf("moved=%s reviews=%+v", asJSON(moved.Result.Slides[0].CanvasPositions), moved.SlideReviews)
		t.Fatal("positions lost or wrong pages invalidated")
	}
	worksCall(t, s, "POST", path+"/edit", token, map[string]any{"result": saved.Result, "revision": base + 1}, 409)
	download := worksCall(t, s, "GET", path+"/download/pptx", token, nil, 200).Body.Bytes()
	if dir := os.Getenv("TEST_CANVAS_ARTIFACTS"); dir != "" {
		os.MkdirAll(dir, 0700)
		os.WriteFile(filepath.Join(dir, "moved.pptx"), download, 0600)
	}
	result, err := s.postBytes(context.Background(), s.cfg.RenderURL+"/render/pptx", moved.Result, "")
	if err != nil || len(result) < 1000 || len(download) < 1000 {
		t.Fatal("missing actual PPTX", err)
	}
	worksCall(t, s, "GET", path+"/slides/"+slide+"/preview?revision="+strconv.Itoa(base+2), token, nil, 200)
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before {
		t.Fatal("free positioning changed accounting")
	}
	for _, foreign := range []string{other, admin} {
		worksCall(t, s, "POST", path+"/edit", foreign, map[string]any{"result": moved.Result, "revision": base + 2}, 404)
	}
}

func TestCoursewareCanvasPreviewRechecksOnlyChangedPageHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	id := worksFixture(t, s, token, "courseware", "succeeded", "三页对象移动", TaskInput{PreviewFirst: true})
	var actor Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &actor)
	d := sampleDeck(TaskInput{Topic: "浮力", Pages: 8, Theme: "forest"})
	assignSlideIDs(&d)
	d.Plan = &TeachingPlan{Summary: "样例规划", Conventions: "统一单位"}
	for i := 0; i < 8; i++ {
		d.Plan.Pages = append(d.Plan.Pages, TeachingPage{Position: i + 1, Goal: "理解浮力", Task: "concept", Minutes: 5, Preview: i < 3})
	}
	d.Slides = d.Slides[:3]
	d.PageChecks = map[string]PageQualityCheck{}
	for i := range d.Slides {
		d.Slides[i].Order = i + 1
		d.PageChecks[d.Slides[i].ID] = PageQualityCheck{Hash: slideContentHash(d, d.Slides[i]), Content: "sample", Visual: "sample"}
	}
	path, err := s.savePPTX(context.Background(), actor.TenantID, id+"-fixture", &d)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := s.tx(context.Background(), actor.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), "UPDATE tasks SET result=$1,pptx_path=$2,status='awaiting_preview' WHERE id=$3", asJSON(d), path, id)
	if err == nil {
		err = tx.Commit(context.Background())
	} else {
		tx.Rollback(context.Background())
	}
	if err != nil {
		t.Fatal(err)
	}
	route := "/api/tasks/" + id
	var current struct {
		Revision int
		Result   Deck
	}
	json.Unmarshal(worksCall(t, s, "GET", route, token, nil, 200).Body.Bytes(), &current)
	for _, sl := range current.Result.Slides {
		worksCall(t, s, "POST", route+"/slides/"+sl.ID+"/review", token, map[string]any{"revision": current.Revision, "hash": slideContentHash(current.Result, sl), "approved": true}, 200)
	}
	scale := .9
	current.Result.Slides[0].CanvasPositions = map[string]CanvasOffset{"title": {DX: .01, DY: .01, Scale: &scale}}
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	worksCall(t, s, "POST", route+"/draft", token, map[string]any{"revision": current.Revision, "result": current.Result}, 200)
	var moved struct {
		Result       Deck
		SlideReviews []slideReview `json:"slide_reviews"`
	}
	json.Unmarshal(worksCall(t, s, "GET", route, token, nil, 200).Body.Bytes(), &moved)
	if moved.SlideReviews[0].Approved || !moved.SlideReviews[1].Approved || !moved.SlideReviews[2].Approved {
		t.Fatal("wrong preview pages invalidated")
	}
	if !pageCheckPassed(moved.Result.PageChecks[moved.Result.Slides[0].ID], true) {
		t.Fatal("edited page was not checked")
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before {
		t.Fatal("free preview movement changed billing")
	}
}

func TestCoursewareCanvasScaleValidation(t *testing.T) {
	d := sampleDeck(TaskInput{Topic: "Scale test", Pages: 8, Theme: "forest"})
	assignSlideIDs(&d)
	hash := slideContentHash(d, d.Slides[0])
	scaleOnly := .9
	d.Slides[0].CanvasPositions = map[string]CanvasOffset{"title": {Scale: &scaleOnly}}
	if slideContentHash(d, d.Slides[0]) == hash {
		t.Fatal("scale-only change retained approval hash")
	}

	for _, scale := range []float64{0, -1, .79, 1.51, math.Inf(1), math.NaN()} {
		sl := Slide{CanvasPositions: map[string]CanvasOffset{"title": {Scale: &scale}}}
		if validateCanvasPositions(sl) == nil {
			t.Fatalf("invalid scale accepted: %v", scale)
		}
	}
	for _, scale := range []float64{.8, 1, 1.5} {
		if err := validateCanvasPositions(Slide{CanvasPositions: map[string]CanvasOffset{"title": {Scale: &scale}}}); err != nil {
			t.Fatal(err)
		}
	}
	var old Slide
	if err := json.Unmarshal([]byte(`{"canvas_positions":{"title":{"dx":0.01,"dy":0}}}`), &old); err != nil {
		t.Fatal(err)
	}
	if old.CanvasPositions["title"].Scale != nil || validateCanvasPositions(old) != nil {
		t.Fatal("legacy offsets rejected")
	}
	var zero Slide
	json.Unmarshal([]byte(`{"canvas_positions":{"title":{"dx":0,"dy":0,"scale":0}}}`), &zero)
	if validateCanvasPositions(zero) == nil {
		t.Fatal("explicit zero scale accepted")
	}
}

func TestCoursewareCanvasInlineTextSavePreviewDownloadIsolationHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "other")
	admin := worksLogin(t, s, "admin")
	id := worksFixture(t, s, token, "courseware", "succeeded", "对象移动", TaskInput{})
	path := "/api/tasks/" + id
	var initial struct{ Revision int }
	json.Unmarshal(worksCall(t, s, "GET", path, token, nil, 200).Body.Bytes(), &initial)
	base := initial.Revision
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	d := sampleDeck(TaskInput{Topic: "浮力", Pages: 8, Theme: "forest"})
	assignSlideIDs(&d)
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	worksCall(t, s, "POST", path+"/edit", token, map[string]any{"result": d, "revision": base}, 200)
	var saved struct {
		Result   Deck
		Revision int
	}
	json.Unmarshal(worksCall(t, s, "GET", path, token, nil, 200).Body.Bytes(), &saved)
	slide := saved.Result.Slides[0].ID
	objects := path + "/slides/" + slide + "/objects?revision=" + strconv.Itoa(base+1)
	mapResult := worksObject(t, worksCall(t, s, "GET", objects, token, nil, 200))
	if len(mapResult["objects"].([]any)) == 0 {
		t.Fatal("empty actual object map")
	}
	for _, foreign := range []string{other, admin} {
		worksCall(t, s, "GET", objects, foreign, nil, 404)
	}
	worksCall(t, s, "GET", path+"/slides/"+slide+"/objects?revision="+strconv.Itoa(base), token, nil, 409)
	worksCall(t, s, "GET", path+"/slides/missing/objects?revision="+strconv.Itoa(base+1), token, nil, 404)
	worksCall(t, s, "GET", objects, "", nil, 401)
	for _, sl := range saved.Result.Slides {
		worksCall(t, s, "POST", path+"/slides/"+sl.ID+"/review", token, map[string]any{"revision": base + 1, "hash": slideContentHash(saved.Result, sl), "approved": true}, 200)
	}
	saved.Result.Slides[0].Title = "观察排水实验"
	saved.Result.Slides[0].Bullets = []string{"根据排水体积计算浮力", "浮力方向竖直向上"}
	saved.Result.Slides[0].Focus = "排水体积改变时浮力如何变化？"
	scale := .9
	saved.Result.Slides[0].CanvasPositions = map[string]CanvasOffset{"title": {DX: .01, DY: .01, Scale: &scale}}
	worksCall(t, s, "POST", path+"/edit", token, map[string]any{"result": saved.Result, "revision": base + 1}, 200)
	var moved struct {
		Result       Deck
		Revision     int
		SlideReviews []slideReview `json:"slide_reviews"`
	}
	json.Unmarshal(worksCall(t, s, "GET", path, token, nil, 200).Body.Bytes(), &moved)
	if moved.Result.Slides[0].CanvasPositions["title"].DX != .01 || moved.Result.Slides[0].CanvasPositions["title"].Scale == nil || *moved.Result.Slides[0].CanvasPositions["title"].Scale != .9 || moved.SlideReviews[0].Approved || !moved.SlideReviews[1].Approved {
		t.Logf("moved=%s reviews=%+v", asJSON(moved.Result.Slides[0].CanvasPositions), moved.SlideReviews)
		t.Fatal("positions lost or wrong pages invalidated")
	}
	worksCall(t, s, "POST", path+"/edit", token, map[string]any{"result": saved.Result, "revision": base + 1}, 409)
	if moved.Result.Slides[0].Title != "观察排水实验" || moved.Result.Slides[0].Focus != "排水体积改变时浮力如何变化？" {
		t.Fatal("inline text lost")
	}
	// A rendered overflow and an invalid API input must leave the previous version intact.
	var overflowing Deck
	json.Unmarshal(asJSON(moved.Result), &overflowing)
	overflowing.Slides[0].Bullets = []string{strings.Repeat("教学说明", 38), strings.Repeat("教学说明", 38), strings.Repeat("教学说明", 38), strings.Repeat("教学说明", 38), strings.Repeat("教学说明", 38), strings.Repeat("教学说明", 38)}
	worksCall(t, s, "POST", path+"/edit", token, map[string]any{"result": overflowing, "revision": base + 2}, 422)
	var unchanged struct {
		Revision int
		Result   Deck
	}
	json.Unmarshal(worksCall(t, s, "GET", path, token, nil, 200).Body.Bytes(), &unchanged)
	if unchanged.Revision != base+2 || string(asJSON(unchanged.Result)) != string(asJSON(moved.Result)) {
		t.Fatal("failed render replaced saved text")
	}
	overflowing.Slides[0].Title = strings.Repeat("字", 71)
	worksCall(t, s, "POST", path+"/edit", token, map[string]any{"result": overflowing, "revision": base + 2}, 400)
	download := worksCall(t, s, "GET", path+"/download/pptx", token, nil, 200).Body.Bytes()
	archive, err := zip.NewReader(bytes.NewReader(download), int64(len(download)))
	if err != nil {
		t.Fatal(err)
	}
	var xmlText string
	for _, file := range archive.File {
		if file.Name == "ppt/slides/slide1.xml" {
			reader, e := file.Open()
			if e != nil {
				t.Fatal(e)
			}
			raw, e := io.ReadAll(reader)
			reader.Close()
			if e != nil {
				t.Fatal(e)
			}
			xmlText = string(raw)
		}
	}
	for _, text := range []string{"观察排水实验", "根据排水体积计算浮力", "浮力方向竖直向上", "排水体积改变时浮力如何变化？"} {
		if !strings.Contains(xmlText, text) {
			t.Fatalf("download missing edited text: %s", text)
		}
	}
	if dir := os.Getenv("TEST_CANVAS_ARTIFACTS"); dir != "" {
		os.MkdirAll(dir, 0700)
		os.WriteFile(filepath.Join(dir, "inline-edited.pptx"), download, 0600)
	}
	result, err := s.postBytes(context.Background(), s.cfg.RenderURL+"/render/pptx", moved.Result, "")
	if err != nil || len(result) < 1000 || len(download) < 1000 {
		t.Fatal("missing actual PPTX", err)
	}
	preview := worksCall(t, s, "GET", path+"/slides/"+slide+"/preview?revision="+strconv.Itoa(base+2), token, nil, 200).Body.Bytes()
	if len(preview) < 1000 || !bytes.HasPrefix(preview, []byte{137, 80, 78, 71}) {
		t.Fatal("missing Office preview")
	}
	if dir := os.Getenv("TEST_CANVAS_ARTIFACTS"); dir != "" {
		os.WriteFile(filepath.Join(dir, "inline-edited-preview.png"), preview, 0600)
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before {
		t.Fatal("free positioning changed accounting")
	}
	for _, foreign := range []string{other, admin} {
		worksCall(t, s, "POST", path+"/edit", foreign, map[string]any{"result": moved.Result, "revision": base + 2}, 404)
	}
}

func TestCoursewareCanvasInlinePreviewRechecksOnlyChangedPageHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	id := worksFixture(t, s, token, "courseware", "succeeded", "三页对象移动", TaskInput{PreviewFirst: true})
	var actor Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &actor)
	d := sampleDeck(TaskInput{Topic: "浮力", Pages: 8, Theme: "forest"})
	assignSlideIDs(&d)
	d.Plan = &TeachingPlan{Summary: "样例规划", Conventions: "统一单位"}
	for i := 0; i < 8; i++ {
		d.Plan.Pages = append(d.Plan.Pages, TeachingPage{Position: i + 1, Goal: "理解浮力", Task: "concept", Minutes: 5, Preview: i < 3})
	}
	d.Slides = d.Slides[:3]
	d.PageChecks = map[string]PageQualityCheck{}
	for i := range d.Slides {
		d.Slides[i].Order = i + 1
		d.PageChecks[d.Slides[i].ID] = PageQualityCheck{Hash: slideContentHash(d, d.Slides[i]), Content: "sample", Visual: "sample"}
	}
	path, err := s.savePPTX(context.Background(), actor.TenantID, id+"-fixture", &d)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := s.tx(context.Background(), actor.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), "UPDATE tasks SET result=$1,pptx_path=$2,status='awaiting_preview' WHERE id=$3", asJSON(d), path, id)
	if err == nil {
		err = tx.Commit(context.Background())
	} else {
		tx.Rollback(context.Background())
	}
	if err != nil {
		t.Fatal(err)
	}
	route := "/api/tasks/" + id
	var current struct {
		Revision int
		Result   Deck
	}
	json.Unmarshal(worksCall(t, s, "GET", route, token, nil, 200).Body.Bytes(), &current)
	for _, sl := range current.Result.Slides {
		worksCall(t, s, "POST", route+"/slides/"+sl.ID+"/review", token, map[string]any{"revision": current.Revision, "hash": slideContentHash(current.Result, sl), "approved": true}, 200)
	}
	current.Result.Slides[0].Title = "三页预览直接改标题"
	current.Result.Slides[0].Bullets = []string{"根据排水体积计算浮力", "浮力方向竖直向上"}
	current.Result.Slides[0].Focus = "如何验证浮力方向？"
	scale := .9
	current.Result.Slides[0].CanvasPositions = map[string]CanvasOffset{"title": {DX: .01, DY: .01, Scale: &scale}}
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	worksCall(t, s, "POST", route+"/draft", token, map[string]any{"revision": current.Revision, "result": current.Result}, 200)
	var moved struct {
		Result       Deck
		SlideReviews []slideReview `json:"slide_reviews"`
	}
	json.Unmarshal(worksCall(t, s, "GET", route, token, nil, 200).Body.Bytes(), &moved)
	if moved.SlideReviews[0].Approved || !moved.SlideReviews[1].Approved || !moved.SlideReviews[2].Approved {
		t.Fatal("wrong preview pages invalidated")
	}
	if !pageCheckPassed(moved.Result.PageChecks[moved.Result.Slides[0].ID], true) {
		t.Fatal("edited page was not checked")
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before {
		t.Fatal("free preview movement changed billing")
	}
}
