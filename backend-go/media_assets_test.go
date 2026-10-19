package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/gif"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mediaUpload(t *testing.T, s *Server, token, name string, raw []byte, want int) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	m := multipart.NewWriter(&body)
	f, _ := m.CreateFormFile("file", name)
	f.Write(raw)
	m.Close()
	r := httptest.NewRequest("POST", "/api/media", &body)
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", m.FormDataContentType())
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("upload: %d, want %d: %s", w.Code, want, w.Body.String())
	}
	return w
}

func TestMediaAssetsIsolationHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "other")
	admin := worksLogin(t, s, "admin")
	frame := image.NewPaletted(image.Rect(0, 0, 64, 64), color.Palette{color.White, color.Black})
	var raw bytes.Buffer
	if e := gif.EncodeAll(&raw, &gif.GIF{Image: []*image.Paletted{frame, frame}, Delay: []int{10, 10}}); e != nil {
		t.Fatal(e)
	}
	mediaUpload(t, s, "", "lesson.gif", raw.Bytes(), 401)
	mediaUpload(t, s, token, "truncated.gif", raw.Bytes()[:20], 400)
	w := mediaUpload(t, s, token, "lesson.gif", raw.Bytes(), 201)
	asset := worksObject(t, w)["asset_id"].(string)
	path := "/api/media/" + asset
	worksCall(t, s, "GET", path, "", nil, 401)
	for _, foreign := range []string{other, admin} {
		worksCall(t, s, "GET", path, foreign, nil, 404)
	}
	got := worksCall(t, s, "GET", path, token, nil, 200)
	if !bytes.Equal(got.Body.Bytes(), raw.Bytes()) {
		t.Fatal("animated GIF changed on upload")
	}
	if got.Header().Get("Content-Type") != "image/gif" {
		t.Fatal(got.Header())
	}
	if renderURL := os.Getenv("MEDIA_TEST_RENDER_URL"); renderURL != "" {
		s.cfg.RenderURL = renderURL
		s.cfg.CoursewareContentReviewDisabled = true
		s.cfg.CoursewareReviewPolicy = "none"
		id := worksFixture(t, s, token, "courseware", "succeeded", "图片引用", TaskInput{})
		d := Deck{Title: "图片引用", Lesson: "教学说明", Theme: "forest"}
		for i := 0; i < 8; i++ {
			d.Slides = append(d.Slides, Slide{Title: "图片引用", Layout: "concept", Bullets: []string{"观察两帧颜色变化。"}})
		}
		d.Slides[0].Picture = &TeachingPicture{AssetID: asset, Caption: "两帧教学图", Source: "教师自绘测试素材"}
		d.Slides[1].CanvasObjects = []CanvasObject{{ID: "object-gif", Kind: "image", AssetID: asset, X: .5, Y: .4, Width: .2, Height: .2}}
		d.Slides[2].Graph = &FunctionGraph{Title: "参数a与开口", XMin: -3, XMax: 3, YMin: -4, YMax: 4, Series: []FunctionSeries{{Kind: "quadratic", A: 1}}, Animation: &FunctionAnimation{Series: 0, Param: "a", To: -1, DurationMS: 2000}}
		d.Slides[2].Table = &TeachingTable{Headers: []string{"参数", "开口"}, Rows: [][]string{{"a大于零", "向上"}, {"a小于零", "向下"}}}
		var current struct{ Revision int }
		json.Unmarshal(worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200).Body.Bytes(), &current)
		billing := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
		worksCall(t, s, "POST", "/api/tasks/"+id+"/edit", token, map[string]any{"revision": current.Revision, "result": d}, 200)
		var saved struct{ Result Deck }
		json.Unmarshal(worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200).Body.Bytes(), &saved)
		if saved.Result.Slides[0].Picture.DataURL != "" || saved.Result.Slides[0].Picture.AssetID != asset || saved.Result.Slides[1].CanvasObjects[0].DataURL != "" {
			t.Fatal("binary data leaked into persisted deck")
		}
		pptx := worksCall(t, s, "GET", "/api/tasks/"+id+"/download/pptx", token, nil, 200).Body.Bytes()
		z, e := zip.NewReader(bytes.NewReader(pptx), int64(len(pptx)))
		if e != nil {
			t.Fatal(e)
		}
		found := false
		gifCount := 0
		for _, f := range z.File {
			if strings.HasPrefix(f.Name, "ppt/media/") {
				if strings.HasSuffix(f.Name, ".gif") {
					gifCount++
				}
				r, _ := f.Open()
				b, _ := io.ReadAll(r)
				r.Close()
				found = found || bytes.Equal(b, raw.Bytes())
			}
		}
		if !found || gifCount < 2 {
			t.Fatal("original GIF or generated function GIF missing from delivered PPTX")
		}
		if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != billing {
			t.Fatal("asset edit charged credits")
		}
		foreignID := worksFixture(t, s, other, "courseware", "succeeded", "窃取图片引用", TaskInput{})
		json.Unmarshal(worksCall(t, s, "GET", "/api/tasks/"+foreignID, other, nil, 200).Body.Bytes(), &current)
		worksCall(t, s, "POST", "/api/tasks/"+foreignID+"/edit", other, map[string]any{"revision": current.Revision, "result": d}, 404)
		if dir := os.Getenv("MEDIA_TEST_ARTIFACTS"); dir != "" {
			os.MkdirAll(dir, 0700)
			os.WriteFile(filepath.Join(dir, "references.pptx"), pptx, 0600)
		}
	}
}
