package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScientificVisualsHTTP(t *testing.T) {
	s := worksServer(t)
	if url := os.Getenv("SCIENCE_RENDER_URL"); url != "" {
		s.cfg.RenderURL = url
	}
	teacher, other, admin := worksLogin(t, s, "teacher"), worksLogin(t, s, "other"), worksLogin(t, s, "admin")
	diagram := map[string]any{"kind": "thin_lens", "title": "凸透镜成像", "focal_length_cm": 10, "object_distance_cm": 30, "object_height_cm": 3}
	path := "/api/courseware/scientific"
	worksCall(t, s, "POST", path, "", map[string]any{"diagram": diagram}, 401)
	bill := worksCall(t, s, "GET", "/api/billing", teacher, nil, 200).Body.String()
	preview := worksObject(t, worksCall(t, s, "POST", path, teacher, map[string]any{"diagram": diagram}, 200))
	if !strings.HasPrefix(preview["image"].(string), "data:image/png;base64,") {
		t.Fatal("scientific preview missing")
	}
	unsafe := map[string]any{"kind": "scene", "title": "输入校验", "scene": map[string]any{
		"duration_ms": 1000, "script": "alert(1)", "objects": []any{map[string]any{"id": "dot", "type": "ellipse", "x": .5, "y": .5}}, "actions": []any{},
	}}
	worksCall(t, s, "POST", path, teacher, map[string]any{"diagram": unsafe}, 400)
	staticScene := map[string]any{"kind": "scene", "title": "静态图示", "scene": map[string]any{
		"duration_ms": 1000, "objects": []any{map[string]any{"id": "box", "type": "rect", "x": .5, "y": .5}},
	}}
	staticPreview := worksObject(t, worksCall(t, s, "POST", path, teacher, map[string]any{"diagram": staticScene}, 200))
	if staticPreview["animation"] != nil {
		t.Fatal("static scene unexpectedly animated")
	}
	if worksCall(t, s, "GET", "/api/billing", teacher, nil, 200).Body.String() != bill {
		t.Fatal("preview charged credits")
	}
	s.cfg.CoursewareContentReviewDisabled = true
	s.cfg.CoursewareReviewPolicy = "none"
	id := worksFixture(t, s, teacher, "courseware", "succeeded", "科学图示", TaskInput{})
	var revision struct{ Revision int }
	json.Unmarshal(worksCall(t, s, "GET", "/api/tasks/"+id, teacher, nil, 200).Body.Bytes(), &revision)
	pages := []any{}
	for i := 0; i < 8; i++ {
		pages = append(pages, map[string]any{"title": "图示教学", "layout": "concept", "bullets": []string{"依据图示说明条件与结论。"}, "scientific": diagram})
	}
	pages[1] = map[string]any{"title": "分子官能团", "layout": "concept", "bullets": []string{"结合结构识别羟基。"}, "scientific": map[string]any{"kind": "molecule", "title": "乙醇羟基", "smiles": "CCO", "highlight_group": "hydroxyl", "animation": map[string]any{"param": "highlight", "to": 1, "duration_ms": 1500}}}
	pages[2] = map[string]any{"title": "细胞结构", "layout": "concept", "bullets": []string{"根据结构编号辨认叶绿体。"}, "scientific": map[string]any{"kind": "cell", "title": "植物细胞", "cell_type": "plant", "visible_parts": []string{"cell_membrane", "nucleus", "chloroplast"}, "highlight_parts": []string{"chloroplast"}}}
	pages[3] = map[string]any{"title": "通用图形动画", "layout": "concept", "bullets": []string{"观察圆点沿路径移动。"}, "scientific": map[string]any{"kind": "scene", "title": "路径与位置（示意）", "scene": map[string]any{"duration_ms": 3000, "objects": []any{map[string]any{"id": "route", "type": "path", "points": [][2]float64{{.2, .5}, {.5, .3}, {.8, .5}}}, map[string]any{"id": "dot", "type": "ellipse", "x": .2, "y": .5, "width": .04, "height": .06, "fill": "#bc582b"}}, "actions": []any{map[string]any{"object": "dot", "op": "follow_path", "path": "route", "start_ms": 0, "duration_ms": 2500}}}}}
	deck := map[string]any{"title": "科学图示", "theme": "ocean", "lesson": "科学图示教学", "slides": pages}
	// The same attached label survives Go validation, Python rendering and PPTX.
	composed := pages[3].(map[string]any)["scientific"].(map[string]any)["scene"].(map[string]any)
	composed["objects"] = append(composed["objects"].([]any), map[string]any{"id": "label", "type": "text", "x": .2, "y": .64, "text": "圆点", "attach_to": "dot"})
	for _, token := range []string{other, admin} {
		worksCall(t, s, "POST", "/api/tasks/"+id+"/edit", token, map[string]any{"revision": revision.Revision, "result": deck}, 404)
	}
	worksCall(t, s, "POST", "/api/tasks/"+id+"/edit", teacher, map[string]any{"revision": revision.Revision, "result": deck}, 200)
	file := worksCall(t, s, "GET", "/api/tasks/"+id+"/download/pptx", teacher, nil, 200).Body.Bytes()
	z, err := zip.NewReader(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		t.Fatal(err)
	}
	content := ""
	for _, f := range z.File {
		if strings.HasPrefix(f.Name, "ppt/slides/slide") && strings.HasSuffix(f.Name, ".xml") {
			r, _ := f.Open()
			raw, _ := io.ReadAll(r)
			r.Close()
			content += string(raw)
		}
	}
	for _, name := range []string{"scientific-thin_lens", "scientific-molecule", "scientific-cell", "scientific-scene"} {
		if !strings.Contains(content, name) {
			t.Fatal("PPTX missing " + name)
		}
	}
	if dir := os.Getenv("SCIENCE_ARTIFACTS"); dir != "" {
		os.MkdirAll(dir, 0700)
		os.WriteFile(filepath.Join(dir, "science-http.pptx"), file, 0600)
	}
	if worksCall(t, s, "GET", "/api/billing", teacher, nil, 200).Body.String() != bill {
		t.Fatal("scientific editing charged credits")
	}
}
