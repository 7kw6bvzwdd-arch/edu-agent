package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestCoursewareRendererAutomaticRecovery(t *testing.T) {
	for _, status := range []int{503, 429, 401} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			renderer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls <= 2 {
					w.WriteHeader(status)
					return
				}
				w.Write([]byte("PK-test-artifact"))
			}))
			defer renderer.Close()
			s := &Server{cfg: Config{RenderURL: renderer.URL}, http: renderer.Client()}
			d := Deck{Title: "只重试渲染", Slides: []Slide{{Title: "保留原页", Bullets: []string{"原数据"}}}}
			before := string(asJSON(d))
			b, err := s.renderCoursewareAvailable(context.Background(), d)
			if status == 401 {
				if calls != 1 || err == nil {
					t.Fatal("permanent error retried", calls, err)
				}
			} else if calls != 3 || err != nil || !strings.HasPrefix(string(b), "PK") {
				t.Fatal("transient render error not recovered", calls, err)
			}
			if string(asJSON(d)) != before {
				t.Fatal("retry changed original deck")
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			prior := calls
			if _, err := s.renderCoursewareAvailable(ctx, d); err != context.Canceled || calls != prior {
				t.Fatal("cancellation continued rendering")
			}
		})
	}
}

func TestCoursewareGraphRequiresRendererCapability(t *testing.T) {
	renders := 0
	renderer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			respond(w, 200, map[string]string{"courseware_media_version": "media-v1"})
			return
		}
		renders++
	}))
	defer renderer.Close()
	s := &Server{cfg: Config{RenderURL: renderer.URL}, http: renderer.Client()}
	if _, err := s.renderPPTX(context.Background(), Deck{Slides: []Slide{{Graph: &FunctionGraph{}}}}); err == nil || renders != 0 {
		t.Fatal("old renderer could silently omit graph")
	}
}

func TestCoursewareAutomaticCompleteRepairHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	in := TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "ocean", UseKB: true, PreviewFirst: true, IdempotencyKey: newID()}
	body := map[string]any{}
	json.Unmarshal(asJSON(in), &body)
	previewPageCalls := map[int]int{}
	previewModel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Messages []struct{ Content string } }
		json.NewDecoder(r.Body).Decode(&req)
		var page, end int
		fmt.Sscanf(regexp.MustCompile(`本批只返回第(\d+)至第(\d+)页`).FindString(req.Messages[len(req.Messages)-1].Content), "本批只返回第%d至第%d页", &page, &end)
		if page < 1 || page > 3 || page != end {
			t.Error("preview exceeded three pages")
			w.WriteHeader(400)
			return
		}
		previewPageCalls[page]++
		if page == 2 && previewPageCalls[page] <= 2 {
			w.WriteHeader(503)
			return
		}
		d := sampleDeck(in)
		d.Slides = d.Slides[page-1 : page]
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(asJSON(d))}}}})
	}))
	defer previewModel.Close()
	s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", previewModel.URL, "test", "test"
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, body, 202))["id"].(string)
	tx, err := s.tx(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(context.Background(), "UPDATE tasks SET mode='live' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	route := "/api/tasks/" + id
	preview := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if preview["status"] != "awaiting_preview" {
		t.Fatalf("preview repair requires manual retry: %v %v calls=%v", preview["status"], preview["error"], previewPageCalls)
	}
	if previewPageCalls[1] != 1 || previewPageCalls[2] != 3 || previewPageCalls[3] != 1 {
		t.Fatal("preview regenerated unrelated pages", previewPageCalls)
	}
	worksCall(t, s, "POST", route+"/expand", token, map[string]any{"revision": preview["revision"]}, 409)
	for _, entry := range preview["slide_reviews"].([]any) {
		m := entry.(map[string]any)
		worksCall(t, s, "POST", route+"/slides/"+m["id"].(string)+"/review", token, map[string]any{"revision": preview["revision"], "hash": m["hash"], "approved": true}, 200)
	}
	worksCall(t, s, "POST", route+"/expand", token, map[string]any{"revision": preview["revision"]}, 202)
	calls := map[int]int{}
	original := map[int]Slide{}
	ranges := regexp.MustCompile(`本批只返回第(\d+)至第(\d+)页`)
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Messages []struct{ Content string } }
		json.NewDecoder(r.Body).Decode(&req)
		prompt := req.Messages[len(req.Messages)-1].Content
		var page, end int
		fmt.Sscanf(ranges.FindString(prompt), "本批只返回第%d至第%d页", &page, &end)
		if page < 1 || page > 8 || page != end {
			t.Error("nonlocal request")
			w.WriteHeader(400)
			return
		}
		calls[page]++
		if page == 4 && calls[page] <= 2 {
			w.WriteHeader(503)
			return
		}
		d := sampleDeck(in)
		sl := d.Slides[page-1]
		original[page] = sl
		if page == 5 && calls[page] == 1 {
			sl.Formula = `\invalidSyntax{`
		}
		d.Slides = []Slide{sl}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(asJSON(d))}}}})
	}))
	defer model.Close()
	s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", model.URL, "test", "test"
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	done := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if done["status"] != "awaiting_confirmation" {
		t.Fatalf("requires manual continuation: %v %v calls=%v", done["status"], done["error"], calls)
	}
	var d Deck
	json.Unmarshal(asJSON(done["result"]), &d)
	if len(d.Slides) != 8 {
		t.Fatalf("incomplete: %d", len(d.Slides))
	}
	for p, sl := range d.Slides {
		if p < 3 {
			continue
		}
		expected := original[p+1]
		expected.ID = sl.ID
		if string(asJSON(sl)) != string(asJSON(expected)) {
			t.Errorf("unexpected change on page %d: got %s expected %s", p+1, asJSON(sl), asJSON(expected))
		}
		want := 1
		if p+1 == 4 {
			want = 3
		}
		if p+1 == 5 {
			want = 2
		}
		if calls[p+1] != want {
			t.Errorf("page %d calls=%d want=%d", p+1, calls[p+1], want)
		}
	}
	if !reflect.DeepEqual(preview["result"].(map[string]any)["slides"], done["result"].(map[string]any)["slides"].([]any)[:3]) {
		t.Fatal("approved preview changed")
	}
	if done["charge_state"] != "reserved" {
		t.Fatal("automatically settled before final review")
	}
	billing := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))
	if billing["reserved"] != float64(20) {
		t.Fatal("repair reserved points twice", billing["reserved"])
	}
	artifact := worksCall(t, s, "GET", route+"/preview-pptx", token, nil, 200).Body.Bytes()
	if len(artifact) < 1000 || string(artifact[:2]) != "PK" {
		t.Fatal("missing complete real PPTX")
	}
	worksCall(t, s, "GET", route, worksLogin(t, s, "other"), nil, 404)
	worksCall(t, s, "POST", route+"/cancel", token, nil, 200)
}

func TestFunctionGraphValidationAndTeachingQuality(t *testing.T) {
	raw := map[string]any{"title": "两点法作图", "layout": "example", "bullets": []string{"取点(0,3)与(1,5)，连接并延长成直线。"}, "notes": strings.Repeat("说明取点理由与坐标单位。", 4), "graph": map[string]any{"title": "y=2x+3", "x_min": -2, "x_max": 2, "y_min": -2, "y_max": 6, "series": []any{map[string]any{"k": 2, "b": 3, "mark_x": []int{0, 1}}}}}
	var sl Slide
	json.Unmarshal(asJSON(raw), &sl)
	if !strings.Contains(string(asJSON(sl)), `"graph"`) {
		t.Fatal("numeric graph discarded")
	}
	if err := validateTeachingMedia(sl); err != nil {
		t.Fatal(err)
	}
	g := raw["graph"].(map[string]any)
	g["x_max"] = -3
	json.Unmarshal(asJSON(raw), &sl)
	if err := validateTeachingMedia(sl); err == nil {
		t.Fatal("reversed numeric axes accepted")
	}
	g["x_max"] = 2
	g["y_max"] = 4
	json.Unmarshal(asJSON(raw), &sl)
	if err := validateTeachingMedia(sl); err == nil {
		t.Fatal("key point outside axes accepted")
	}
	d := sampleDeck(TaskInput{Type: "courseware", Topic: "一次函数", Pages: 8})
	assignSlideIDs(&d)
	in := TaskInput{Topic: "一次函数"}
	if len(coursewareFunctionIssues(in, d)) != 2 {
		t.Fatal("missing function graphs not detected")
	}
	d.Slides[3].Graph = &FunctionGraph{Series: []FunctionSeries{{K: 2, B: 3}}}
	d.Slides[4].Graph = &FunctionGraph{Series: []FunctionSeries{{K: 1}, {K: -1}}}
	if len(coursewareFunctionIssues(in, d)) != 0 {
		t.Fatal("numeric graphs and comparison not counted")
	}
	approved := map[string]bool{d.Slides[3].ID: true}
	if page := coursewareRepairTarget(d, "缺少函数图像对比", approved); page <= 4 || approved[d.Slides[page-1].ID] {
		t.Fatal("graph repair targeted approved page", page)
	}
}
