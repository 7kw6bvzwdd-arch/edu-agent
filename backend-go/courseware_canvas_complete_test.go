package main

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func canvasTestImage(t *testing.T) string {
	t.Helper()
	var b bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 96, 96))
	for y := 0; y < 96; y++ {
		for x := 0; x < 96; x++ {
			img.Set(x, y, color.RGBA{20, 120, 180, 255})
		}
	}
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}
func TestCoursewareCanvasCompleteValidationAndTextIsolation(t *testing.T) {
	good := CanvasObject{ID: "object-test", Kind: "text", X: .2, Y: .4, Width: .3, Height: .1, Text: "教学补充", FontSize: 24, Color: "182820"}
	sl := Slide{Title: "教学页面", CanvasObjects: []CanvasObject{good}}
	if err := validateCanvasPositions(sl); err != nil {
		t.Fatal(err)
	}
	changes := []func(*CanvasObject){func(o *CanvasObject) { o.ID = "title" }, func(o *CanvasObject) { o.X = math.NaN() }, func(o *CanvasObject) { o.Width = -.1 }, func(o *CanvasObject) { o.Y = .99 }, func(o *CanvasObject) { o.Rotation = 181 }, func(o *CanvasObject) { o.Color = "red" }, func(o *CanvasObject) { o.FontSize = 9 }, func(o *CanvasObject) { o.Align = "justify" }, func(o *CanvasObject) { o.Kind = "script" }, func(o *CanvasObject) { o.Text = "" }, func(o *CanvasObject) { o.Kind = "image"; o.DataURL = "https://example.com/image.png" }}
	for i, modify := range changes {
		bad := good
		modify(&bad)
		if validateCanvasPositions(Slide{CanvasObjects: []CanvasObject{bad}}) == nil {
			t.Fatalf("invalid case %d accepted", i)
		}
	}
	sl.CanvasObjects = append(sl.CanvasObjects, good)
	if validateCanvasPositions(sl) == nil {
		t.Fatal("duplicate ID accepted")
	}
	sl.CanvasObjects = []CanvasObject{good}
	sl.CanvasOrder = []string{"object-test", "missing"}
	if validateCanvasPositions(sl) == nil {
		t.Fatal("unknown layer accepted")
	}
	sl.CanvasOrder = nil
	sl.CanvasObjects = append(sl.CanvasObjects, CanvasObject{ID: "object-image", Kind: "image", X: .5, Y: .4, Width: .2, Height: .2, DataURL: canvasTestImage(t)})
	before := string(asJSON(sl))
	text := slideForTextModel(sl)
	if text.CanvasObjects[1].DataURL != "" || string(asJSON(sl)) != before {
		t.Fatal("text model sanitization destroyed original image")
	}
	deck := Deck{Slides: []Slide{sl}}
	deckForTextModel(deck)
	if string(asJSON(sl)) != before {
		t.Fatal("deck sanitization mutated original")
	}
}

func TestCoursewareCanvasCompleteSaveAndTemporaryPreviewHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "other")
	admin := worksLogin(t, s, "admin")
	id := worksFixture(t, s, token, "courseware", "succeeded", "完整画布测试", TaskInput{})
	path := "/api/tasks/" + id
	var current struct {
		Revision int
		Result   Deck
	}
	json.Unmarshal(worksCall(t, s, "GET", path, token, nil, 200).Body.Bytes(), &current)
	d := sampleDeck(TaskInput{Topic: "画布样例", Pages: 8, Theme: "forest"})
	assignSlideIDs(&d)
	sl := &d.Slides[0]
	sl.Title = "教师补充的教学内容"
	sl.Layout = "concept"
	sl.Bullets = []string{}
	sl.Focus = ""
	sl.Graph = nil
	sl.Scientific = nil
	sl.Chart = nil
	sl.Diagram = ""
	sl.Formula = ""
	sl.Notes = "CANVAS_PRIVATE_NOTE"
	sl.CanvasObjects = []CanvasObject{
		{ID: "object-text", Kind: "text", X: .09, Y: .36, Width: .47, Height: .1, Text: "观察 → 推理 → 验证", FontSize: 24, Bold: true, Align: "left", Color: "182820"},
		{ID: "object-image", Kind: "image", X: .7, Y: .4, Width: .2, Height: .25, Rotation: 15, DataURL: canvasTestImage(t), Caption: "教师上传测试配图"},
		{ID: "object-shape", Kind: "shape", X: .1, Y: .55, Width: .4, Height: .12, Rotation: 5, Shape: "arrow", Color: "246650", Background: "DDEEE7", LineWidth: 2},
	}
	sl.CanvasOrder = []string{"title", "object-shape", "object-image", "object-text"}
	beforeBill := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	worksCall(t, s, "POST", path+"/edit", token, map[string]any{"revision": current.Revision, "result": d}, 200)
	json.Unmarshal(worksCall(t, s, "GET", path, token, nil, 200).Body.Bytes(), &current)
	baseline := string(asJSON(current.Result))
	revision := current.Revision
	route := path + "/slides/" + d.Slides[0].ID + "/canvas-preview"
	body := map[string]any{"revision": revision, "result": current.Result}
	worksCall(t, s, "POST", route, "", body, 401)
	for _, foreign := range []string{other, admin} {
		worksCall(t, s, "POST", route, foreign, body, 404)
	}
	worksCall(t, s, "POST", route, token, map[string]any{"revision": revision - 1, "result": d}, 409)
	worksCall(t, s, "POST", path+"/slides/missing/canvas-preview", token, body, 404)
	canvasPreviewSlot <- struct{}{}
	worksCall(t, s, "POST", route, token, body, 429)
	<-canvasPreviewSlot
	var preview struct {
		Image    string
		Revision int
		Objects  []map[string]any
	}
	json.Unmarshal(worksCall(t, s, "POST", route, token, body, 200).Body.Bytes(), &preview)
	if len(preview.Objects) < 4 {
		t.Fatal("actual preview omitted editable geometry")
	}
	pngBytes, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(preview.Image, "data:image/png;base64,"))
	if err != nil || len(pngBytes) < 1000 || !bytes.HasPrefix(pngBytes, []byte{137, 80, 78, 71}) || preview.Revision != revision {
		t.Fatal("not an Office PNG")
	}
	var after struct {
		Revision int
		Result   Deck
	}
	json.Unmarshal(worksCall(t, s, "GET", path, token, nil, 200).Body.Bytes(), &after)
	if after.Revision != revision || string(asJSON(after.Result)) != baseline || worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != beforeBill {
		t.Fatal("temporary preview wrote task or accounting")
	}
	download := worksCall(t, s, "GET", path+"/download/pptx", token, nil, 200).Body.Bytes()
	archive, err := zip.NewReader(bytes.NewReader(download), int64(len(download)))
	if err != nil {
		t.Fatal(err)
	}
	var xmlText string
	for _, f := range archive.File {
		if f.Name == "ppt/slides/slide1.xml" {
			r, e := f.Open()
			if e != nil {
				t.Fatal(e)
			}
			raw, e := io.ReadAll(r)
			r.Close()
			if e != nil {
				t.Fatal(e)
			}
			xmlText = string(raw)
		}
	}
	for _, text := range []string{"观察 → 推理 → 验证", "edu-region=object-image", "edu-region=object-shape", "rot=\"900000\""} {
		if !strings.Contains(xmlText, text) {
			t.Fatalf("PPTX missing %s", text)
		}
	}
	if dir := os.Getenv("TEST_CANVAS_ARTIFACTS"); dir != "" {
		os.MkdirAll(dir, 0700)
		os.WriteFile(filepath.Join(dir, "canvas-complete.pptx"), download, 0600)
		os.WriteFile(filepath.Join(dir, "canvas-complete-preview.png"), pngBytes, 0600)
	}
	invalid := current.Result
	invalid.Slides = append([]Slide(nil), current.Result.Slides...)
	invalid.Slides[0].CanvasObjects = append([]CanvasObject(nil), current.Result.Slides[0].CanvasObjects...)
	invalid.Slides[0].CanvasObjects[0].Text = strings.Repeat("内容", 100)
	invalid.Slides[0].CanvasObjects[0].Height = .01
	worksCall(t, s, "POST", route, token, map[string]any{"revision": revision, "result": invalid}, 422)
	worksCall(t, s, "POST", path+"/edit", token, map[string]any{"revision": revision, "result": invalid}, 422)
	json.Unmarshal(worksCall(t, s, "GET", path, token, nil, 200).Body.Bytes(), &after)
	if after.Revision != revision || string(asJSON(after.Result)) != baseline {
		t.Fatal("failed edit replaced original")
	}
	current.Result.Slides[0].CanvasObjects = current.Result.Slides[0].CanvasObjects[:1]
	current.Result.Slides[0].CanvasOrder = []string{"title", "object-text"}
	worksCall(t, s, "POST", path+"/edit", token, map[string]any{"revision": revision, "result": current.Result}, 200)
	json.Unmarshal(worksCall(t, s, "GET", path, token, nil, 200).Body.Bytes(), &after)
	if len(after.Result.Slides[0].CanvasObjects) != 1 || strings.Contains(string(asJSON(after.Result)), "data:image/") {
		t.Fatal("deleted image payload retained in current work")
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != beforeBill {
		t.Fatal("canvas edit charged credits")
	}
}
