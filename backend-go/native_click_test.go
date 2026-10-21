package main

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestNativeClickCoursewareEditHTTP(t *testing.T) {
	if os.Getenv("NATIVE_CLICK_RENDER_URL") == "" {
		t.Skip("requires candidate renderer")
	}
	s := worksServer(t)
	s.cfg.RenderURL = os.Getenv("NATIVE_CLICK_RENDER_URL")
	s.cfg.CoursewareReviewPolicy = "none"
	s.cfg.CoursewareContentReviewDisabled = true
	teacher, other, admin := worksLogin(t, s, "teacher"), worksLogin(t, s, "other"), worksLogin(t, s, "admin")
	id := worksFixture(t, s, teacher, "courseware", "succeeded", "比较二次函数", TaskInput{})
	bill := worksCall(t, s, "GET", "/api/billing", teacher, nil, 200).Body.String()
	task := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, teacher, nil, 200))
	deck := task["result"].(map[string]any)
	slide := deck["slides"].([]any)[0].(map[string]any)
	for _, field := range []string{"chart", "scientific", "figure", "picture", "diagram"} {
		delete(slide, field)
	}
	graph := map[string]any{"title": "比较开口方向", "x_min": -3, "x_max": 3, "y_min": -4, "y_max": 4, "reveal_on_click": true, "series": []any{map[string]any{"kind": "quadratic", "a": 1}, map[string]any{"kind": "quadratic", "a": -1}}}
	slide["graph"] = graph
	body := map[string]any{"revision": task["revision"], "result": deck}
	route := "/api/tasks/" + id + "/edit"
	worksCall(t, s, "POST", route, "", body, 401)
	for _, foreign := range []string{other, admin} {
		worksCall(t, s, "POST", route, foreign, body, 404)
	}
	graph["renderer"] = "matplotlib"
	worksCall(t, s, "POST", route, teacher, body, 400)
	delete(graph, "renderer")
	worksCall(t, s, "POST", route, teacher, body, 200)
	result := worksCall(t, s, "GET", "/api/tasks/"+id, teacher, nil, 200)
	if !bytes.Contains(result.Body.Bytes(), []byte(`"reveal_on_click":true`)) {
		t.Fatal("click setting lost from persisted deck")
	}
	ppt := worksCall(t, s, "GET", "/api/tasks/"+id+"/download/pptx", teacher, nil, 200).Body.Bytes()
	archive, err := zip.NewReader(bytes.NewReader(ppt), int64(len(ppt)))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range archive.File {
		if f.Name == "ppt/slides/slide1.xml" {
			r, _ := f.Open()
			data, _ := io.ReadAll(r)
			r.Close()
			found = bytes.Count(data, []byte(`nodeType="clickEffect"`)) == 2 && bytes.Contains(data, []byte("function-series-1"))
		}
	}
	if !found {
		t.Fatal("editable curve groups or entrance timing missing from delivered PPTX")
	}
	if worksCall(t, s, "GET", "/api/billing", teacher, nil, 200).Body.String() != bill {
		t.Fatal("native animation edit changed credits")
	}
}

func TestNativeClickRejectsOldRenderer(t *testing.T) {
	renderer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Error("old renderer must not receive export")
		}
		respond(w, 200, map[string]string{"courseware_media_version": "media-v1"})
	}))
	defer renderer.Close()
	s := &Server{cfg: Config{RenderURL: renderer.URL}, http: renderer.Client()}
	_, err := s.renderPPTX(context.Background(), Deck{Slides: []Slide{{Graph: &FunctionGraph{RevealOnClick: true}}}})
	if err == nil || !strings.Contains(err.Error(), "原生点击") {
		t.Fatal("old renderer silently omitted animation", err)
	}
}
