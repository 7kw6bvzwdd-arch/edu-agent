package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

func TestMatplotlibPlotSaveHTTP(t *testing.T) {
	s := worksServer(t)
	if endpoint := os.Getenv("TEST_PLOT_RENDER_URL"); endpoint != "" {
		s.cfg.RenderURL = endpoint
	}
	owner, other, admin := worksLogin(t, s, "teacher"), worksLogin(t, s, "other"), worksLogin(t, s, "admin")
	id := worksFixture(t, s, owner, "courseware", "succeeded", "实验读图", TaskInput{})
	route := "/api/tasks/" + id
	d := sampleDeck(TaskInput{Topic: "实验读图", Pages: 8, Theme: "violet"})
	d.Slides[0].Graph = nil
	d.Slides[0].Scientific = nil
	d.Slides[0].Table = nil
	d.Slides[0].Figure = nil
	d.Slides[0].Diagram = ""
	d.Slides[0].Picture = nil
	chart := map[string]any{"title": "电流与电压", "style": "errorbar", "labels": []string{"甲", "乙", "丙"}, "values": []float64{.1, .2, .5}, "x_values": []float64{1, 2}, "y_errors": []float64{.01, .02, .03}, "x_label": "电压 U / V", "y_label": "电流 I / A", "source_label": "示意数据，非实测"}
	d.Slides[0].Chart = chart
	payload := func(rev int) map[string]any { return map[string]any{"result": d, "revision": rev} }
	worksCall(t, s, "POST", route+"/edit", owner, payload(0), 400)
	chart["x_values"] = []float64{1, 2, 5}
	chart["y_errors"] = []float64{-.1, .1, .1}
	worksCall(t, s, "POST", route+"/edit", owner, payload(0), 400)
	chart["y_errors"] = []float64{.01, .02, .03}
	worksCall(t, s, "POST", route+"/edit", "", payload(0), 401)
	worksCall(t, s, "POST", route+"/edit", other, payload(0), 404)
	worksCall(t, s, "POST", route+"/edit", admin, payload(0), 404)
	before := worksObject(t, worksCall(t, s, "GET", "/api/billing", owner, nil, 200))
	worksCall(t, s, "POST", route+"/edit", owner, payload(0), 200)
	saved := worksObject(t, worksCall(t, s, "GET", route, owner, nil, 200))
	if saved["status"] != "succeeded" {
		t.Fatal("Plot edit changed delivery status")
	}
	var result Deck
	if err := json.Unmarshal(asJSON(saved["result"]), &result); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(asJSON(result.Slides[0].Chart), []byte(`"x_values":[1,2,5]`)) {
		t.Fatal("Numeric x spacing lost")
	}
	worksCall(t, s, "POST", route+"/edit", owner, payload(0), 409)
	data := worksCall(t, s, "GET", route+"/download/pptx", owner, nil, 200).Body.Bytes()
	worksCall(t, s, "GET", route+"/download/pptx", other, nil, 404)
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range archive.File {
		if file.Name == "ppt/slides/slide1.xml" {
			reader, _ := file.Open()
			content, _ := io.ReadAll(reader)
			reader.Close()
			found = strings.Contains(string(content), "matplotlib-data-plot")
		}
	}
	if !found {
		t.Fatal("Downloaded PPTX omitted Matplotlib plot")
	}
	after := worksObject(t, worksCall(t, s, "GET", "/api/billing", owner, nil, 200))
	for _, key := range []string{"balance", "reserved", "available"} {
		if before[key] != after[key] {
			t.Fatal("Manual plot edit charged points", key)
		}
	}
}
