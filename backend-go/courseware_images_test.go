package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
)

// External Commons requests keep their real URLs; only transport is replaced.
type commonsFixtureTransport struct{ base *url.URL }

func (f commonsFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	copy := r.Clone(r.Context())
	u := *r.URL
	u.Scheme, u.Host = f.base.Scheme, f.base.Host
	copy.URL = &u
	response, err := http.DefaultTransport.RoundTrip(copy)
	if response != nil {
		response.Request = r
	}
	return response, err
}

func TestCoursewareImagesSearchHTTP(t *testing.T) {
	var variant atomic.Int32
	var downloads atomic.Int32
	var reviews atomic.Int32
	s := worksServer(t)
	teacher, other, admin := worksLogin(t, s, "teacher"), worksLogin(t, s, "other"), worksLogin(t, s, "admin")
	id := worksFixture(t, s, teacher, "courseware", "succeeded", "长城", TaskInput{})
	task := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, teacher, nil, 200))
	sl := task["result"].(map[string]any)["slides"].([]any)[0].(map[string]any)
	var picture bytes.Buffer
	png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 320, 240)))
	commons := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/w/api.php" {
			downloads.Add(1)
			if variant.Load() == 3 {
				http.Redirect(w, r, "http://127.0.0.1/private", 302)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			w.Write(picture.Bytes())
			return
		}
		if r.URL.Query().Get("action") != "query" || r.URL.Query().Get("gsrnamespace") != "6" {
			t.Error("search must query Commons file namespace")
		}
		if variant.Load() == 4 {
			w.WriteHeader(503)
			return
		}
		licenseURL := "https://creativecommons.org/licenses/by-sa/4.0/"
		licenseName := "CC BY-SA 4.0"
		copyrighted := "True"
		if variant.Load() == 7 {
			licenseURL = "javascript:alert(1)"
			licenseName = "Public domain"
			copyrighted = "False"
		}
		imageURL := "https://upload.wikimedia.org/wikipedia/commons/a/ab/Great_Wall.png"
		thumbURL := "https://upload.wikimedia.org/wikipedia/commons/thumb/a/ab/Great_Wall.png/960px-Great_Wall.png"
		if variant.Load() == 1 {
			licenseURL = "https://example.org/unknown"
		}
		if variant.Load() == 2 {
			imageURL = "http://127.0.0.1/private"
			thumbURL = imageURL
		}
		respond(w, 200, map[string]any{"query": map[string]any{"pages": []any{map[string]any{
			"pageid": 123, "title": "File:Great Wall.jpg", "imageinfo": []any{map[string]any{
				"url": imageURL, "thumburl": thumbURL, "descriptionurl": "https://commons.wikimedia.org/wiki/File:Great_Wall.jpg", "mime": "image/png", "width": 320, "height": 240,
				"extmetadata": map[string]any{"Artist": map[string]string{"value": "<a href='https://example.org'>Test Photographer</a>"}, "Copyrighted": map[string]string{"value": copyrighted}, "LicenseShortName": map[string]string{"value": licenseName}, "LicenseUrl": map[string]string{"value": licenseURL}, "ImageDescription": map[string]string{"value": "Great Wall at Badaling"}},
			}},
		}}}})
	}))
	defer commons.Close()
	u, _ := url.Parse(commons.URL)
	s.mediaHTTP = &http.Client{Transport: commonsFixtureTransport{u}}
	reviewer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reviews.Add(1)
		if variant.Load() == 6 {
			w.WriteHeader(503)
			return
		}
		if variant.Load() == 5 {
			respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": `{"relevant":false,"reason":"固定审核：与用途不相关。"}`}}}})
			return
		}
		var body struct {
			Messages []struct{ Content []map[string]any }
		}
		json.NewDecoder(r.Body).Decode(&body)
		if len(body.Messages) == 0 || len(body.Messages[0].Content) < 2 || body.Messages[0].Content[1]["type"] != "image_url" {
			t.Error("image relevance review must receive actual image bytes")
		}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": `{"relevant":true,"reason":"画面为长城建筑，可支持观察防御设施。"}`}}}})
	}))
	defer reviewer.Close()
	s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = reviewer.URL, "fixture", "vision-fixture"
	route := "/api/tasks/" + id + "/images/search"
	body := map[string]any{"revision": task["revision"], "slide_id": sl["id"], "need": map[string]any{"purpose": "观察长城的防御设施", "queries": []string{"Great Wall Badaling"}, "constraints": "实际建筑照片"}}
	worksCall(t, s, "POST", route, "", body, 401)
	for _, foreign := range []string{other, admin} {
		worksCall(t, s, "POST", route, foreign, body, 404)
	}
	bill := worksCall(t, s, "GET", "/api/billing", teacher, nil, 200).Body.String()
	response := worksObject(t, worksCall(t, s, "POST", route, teacher, body, 200))
	items, ok := response["candidates"].([]any)
	if !ok || len(items) != 1 || response["status"] != "found" {
		t.Fatalf("no relevant attributed image: %v", response)
	}
	selected := items[0].(map[string]any)
	p := selected["picture"].(map[string]any)
	asset := p["asset_id"].(string)
	if p["data_url"] != nil || !strings.Contains(p["source"].(string), "commons.wikimedia.org/wiki/File:") {
		t.Fatal("image must be a reference with a real source", p)
	}
	got := worksCall(t, s, "GET", "/api/media/"+asset, teacher, nil, 200)
	if !bytes.Equal(got.Body.Bytes(), picture.Bytes()) {
		t.Fatal("stored image changed")
	}
	metaRoute := "/api/media/" + asset + "/metadata"
	metadata := worksObject(t, worksCall(t, s, "GET", metaRoute, teacher, nil, 200))
	provenance := metadata["provenance"].(map[string]any)
	if provenance["author"] != "Test Photographer" || provenance["license"] != "CC BY-SA 4.0" || provenance["sha256"] == "" || provenance["retrieved_at"] == "" || provenance["provider"] != "wikimedia_commons" {
		t.Fatal("source snapshot missing", provenance)
	}
	worksCall(t, s, "GET", metaRoute, "", nil, 401)
	for _, foreign := range []string{other, admin} {
		worksCall(t, s, "GET", "/api/media/"+asset, foreign, nil, 404)
		worksCall(t, s, "GET", metaRoute, foreign, nil, 404)
	}
	if worksCall(t, s, "GET", "/api/billing", teacher, nil, 200).Body.String() != bill {
		t.Fatal("image selection charged generation credits")
	}
	for _, tc := range []struct {
		name   string
		kind   int32
		status string
	}{
		{"unknown license", 1, "not_found"}, {"private address", 2, "not_found"}, {"private redirect", 3, "not_found"}, {"gallery unavailable", 4, "unavailable"}, {"irrelevant", 5, "not_found"}, {"review unavailable", 6, "unavailable"}, {"unsafe public-domain link", 7, "not_found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			variant.Store(tc.kind)
			before := downloads.Load()
			reviewBefore := reviews.Load()
			result := worksObject(t, worksCall(t, s, "POST", route, teacher, body, 200))
			if result["status"] != tc.status || len(result["candidates"].([]any)) != 0 {
				t.Fatal("unsafe image adopted", result)
			}
			if (tc.kind <= 2 || tc.kind == 7) && (before != downloads.Load() || reviewBefore != reviews.Load()) {
				t.Fatal("invalid metadata downloaded/reviewed")
			}
		})
	}
	variant.Store(0)
	visionKey := s.cfg.VisionKey
	s.cfg.VisionKey = ""
	noVision := worksObject(t, worksCall(t, s, "POST", route, teacher, body, 200))
	if noVision["status"] != "unavailable" {
		t.Fatal("unconfigured visual review must reject candidates")
	}
	s.cfg.VisionKey = visionKey
	invalid := map[string]any{"revision": task["revision"], "slide_id": sl["id"], "need": map[string]any{"purpose": "", "queries": []string{"test"}}}
	worksCall(t, s, "POST", route, teacher, invalid, 400)
	stale := map[string]any{"revision": -1, "slide_id": sl["id"], "need": body["need"]}
	worksCall(t, s, "POST", route, teacher, stale, 409)
	// Candidate references must also enter the existing free edit/export flow.
	if render := os.Getenv("COMMONS_TEST_RENDER_URL"); render != "" {
		s.cfg.RenderURL = render
		s.cfg.CoursewareReviewPolicy = "none"
		s.cfg.CoursewareContentReviewDisabled = true
		var deck Deck
		json.Unmarshal(asJSON(task["result"]), &deck)
		deck.Slides[0].Chart, deck.Slides[0].Graph, deck.Slides[0].Scientific = nil, nil, nil
		deck.Slides[0].Diagram, deck.Slides[0].Figure, deck.Slides[0].Table = "", nil, nil
		json.Unmarshal(asJSON(p), &deck.Slides[0].Picture)
		deck.Slides[1].CanvasObjects = []CanvasObject{{ID: "object-commons", Kind: "image", X: .3, Y: .3, Width: .3, Height: .3, AssetID: asset, Provenance: &ImageProvenance{Author: "forged canvas"}}}
		// Client-provided attribution cannot replace the immutable asset record.
		deck.Slides[0].Picture.Provenance = &ImageProvenance{Author: "forged attribution"}
		worksCall(t, s, "POST", "/api/tasks/"+id+"/edit", teacher, map[string]any{"revision": task["revision"], "result": deck}, 200)
		pptx := worksCall(t, s, "GET", "/api/tasks/"+id+"/download/pptx", teacher, nil, 200).Body.Bytes()
		z, err := zip.NewReader(bytes.NewReader(pptx), int64(len(pptx)))
		if err != nil {
			t.Fatal(err)
		}
		var notes string
		imageFound := false
		for _, f := range z.File {
			r, _ := f.Open()
			b, _ := io.ReadAll(r)
			r.Close()
			if strings.HasPrefix(f.Name, "ppt/notesSlides/notesSlide") {
				notes += string(b)
			}
			if strings.HasPrefix(f.Name, "ppt/media/") && bytes.Equal(b, picture.Bytes()) {
				imageFound = true
			}
		}
		if !imageFound || !strings.Contains(notes, "Test Photographer") || !strings.Contains(notes, "creativecommons.org/licenses/by-sa/4.0/") || strings.Contains(notes, "forged attribution") || strings.Contains(notes, "forged canvas") || strings.Count(notes, "Test Photographer") < 2 {
			t.Fatal("real PPTX missing image or trusted source snapshot")
		}
		if dir := os.Getenv("COMMONS_TEST_ARTIFACTS"); dir != "" {
			os.MkdirAll(dir, 0700)
			os.WriteFile(filepath.Join(dir, "commons-http.pptx"), pptx, 0600)
		}
	}
}

// External model and Commons replies below are fixtures; PPTX and task flow are real.
func TestCoursewareImageNeedGenerationHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	if render := os.Getenv("COMMONS_TEST_RENDER_URL"); render != "" {
		s.cfg.RenderURL = render
	}
	var picture bytes.Buffer
	fixtureImage := image.NewRGBA(image.Rect(0, 0, 320, 240))
	fixtureImage.Set(0, 0, color.RGBA{R: 120, G: 80, B: 40, A: 255})
	png.Encode(&picture, fixtureImage)
	commons := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/web-page" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			io.WriteString(w, `<html><head><title>长城建筑</title><meta name="author" content="Test Photographer"></head><body><figure><img src="/web-photo.png" alt="长城城墙与敌台"><figcaption><a rel="license" href="https://creativecommons.org/licenses/by/4.0/">CC BY 4.0</a></figcaption></figure></body></html>`)
			return
		}
		if r.URL.Query().Get("gsrsearch") == "outage" {
			w.WriteHeader(503)
			return
		}
		if r.URL.Path != "/w/api.php" {
			w.Write(picture.Bytes())
			return
		}
		respond(w, 200, map[string]any{"query": map[string]any{"pages": []any{map[string]any{"title": "File:Great Wall.png", "imageinfo": []any{map[string]any{
			"url": "https://upload.wikimedia.org/wikipedia/commons/a/ab/Great_Wall.png", "descriptionurl": "https://commons.wikimedia.org/wiki/File:Great_Wall.png", "mime": "image/png", "width": 320, "height": 240,
			"extmetadata": map[string]any{"Artist": map[string]string{"value": "Test Photographer"}, "LicenseShortName": map[string]string{"value": "CC BY-SA 4.0"}, "LicenseUrl": map[string]string{"value": "https://creativecommons.org/licenses/by-sa/4.0/"}},
		}}}}}})
	}))
	defer commons.Close()
	u, _ := url.Parse(commons.URL)
	s.mediaHTTP = &http.Client{Transport: commonsFixtureTransport{u}}
	for _, tc := range []struct {
		name          string
		web           bool
		query, status string
	}{
		{"automatic", true, "Great Wall", "found"}, {"web-primary", true, "web-primary", "found"}, {"disabled", false, "Great Wall", "disabled"}, {"outage", true, "outage", "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			need := &ImageNeed{Purpose: "观察城墙的连接", Queries: []string{tc.query}, Constraints: "实际建筑照片"}
			lesson := Deck{Title: "长城的防御设施", Theme: "ocean", Lesson: "固定接口验收内容，非真实模型结果。观察城墙、比较设施、解释防御关系。"}
			plan := TeachingPlan{Summary: "课时未指定，分钟仅为估计。", Conventions: "图片只作为建筑观察材料，不能证明具体历史事件。"}
			for i, title := range []string{"观察城墙与敌台", "比较防御设施", "解释建筑连接", "说明观察依据", "讨论证据边界"} {
				lesson.Slides = append(lesson.Slides, Slide{Title: title, Layout: []string{"concept", "comparison", "steps", "concept", "summary"}[i], Bullets: []string{title + "，用画面中的具体线索支持判断。"}, Notes: "引导学生区分建筑的可见线索与推测，具体年代和用途需要结合可靠资料核对。固定验收内容。", ImageNeed: need})
				plan.Pages = append(plan.Pages, TeachingPage{Position: i + 1, Goal: title, Task: "观察、比较与解释", Visual: "真实建筑照片支持读图", Minutes: 3, Preview: i < 3, ImageNeed: need})
			}
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Model    string
					Messages []struct{ Content json.RawMessage }
				}
				json.NewDecoder(r.Body).Decode(&req)
				if len(req.Messages) == 0 {
					if tc.name == "web-primary" {
						respond(w, 200, map[string]any{"results": []any{map[string]string{"title": "长城建筑", "url": "https://museum.example/web-page"}}})
						return
					}
					respond(w, 200, map[string]any{"results": []any{}})
					return
				}
				raw := `{"issues":[]}`
				if req.Model == "text-fixture" {
					var prompt string
					json.Unmarshal(req.Messages[len(req.Messages)-1].Content, &prompt)
					if strings.Contains(prompt, "规划符合实际学科") {
						raw = string(asJSON(plan))
					} else if strings.Contains(prompt, "本批只返回第") {
						var page, end int
						fmt.Sscanf(regexp.MustCompile(`本批只返回第\d+至第\d+页`).FindString(prompt), "本批只返回第%d至第%d页", &page, &end)
						if page < 1 || page > 5 || page != end {
							t.Error("invalid page request")
							w.WriteHeader(400)
							return
						}
						one := lesson
						one.Slides = lesson.Slides[page-1 : page]
						raw = string(asJSON(one))
					}
				} else if strings.Contains(string(req.Messages[0].Content), "真实候选图片") {
					raw = `{"relevant":true,"reason":"固定相关性审核：建筑照片支持连接观察。"}`
				}
				respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": raw}}}})
			}))
			defer provider.Close()
			s.cfg.Mode = "live"
			s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = provider.URL, "fixture", "text-fixture"
			s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = provider.URL, "fixture", "vision-fixture"
			s.cfg.SearchURL, s.cfg.SearchKey = provider.URL, "fixture"
			s.cfg.CoursewareContentReviewDisabled = true
			in := TaskInput{Type: "courseware", Topic: "长城", Subject: "历史", Theme: "ocean", UseKB: true, UseWeb: tc.web, IdempotencyKey: newID()}
			id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
			if err := s.workOne(context.Background()); err != nil {
				t.Fatal(err)
			}
			got := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
			if got["status"] != "awaiting_preview" {
				t.Fatalf("preview not preserved: %v %v events=%s", got["status"], got["error"], asJSON(got["result"]))
			}
			var d Deck
			json.Unmarshal(asJSON(got["result"]), &d)
			if len(d.Slides) != 3 || d.Plan == nil {
				t.Fatal("three planned preview pages missing")
			}
			for _, sl := range d.Slides {
				if sl.ImageSearch == nil || sl.ImageSearch.Status != tc.status {
					t.Fatalf("image status: %+v", sl.ImageSearch)
				}
				if tc.status == "found" && (sl.Picture == nil || sl.Picture.AssetID == "" || sl.Picture.DataURL != "") {
					t.Fatal("automatic image reference missing")
				}
				if tc.name == "web-primary" && !strings.HasPrefix(sl.Picture.Source, "网页配图") {
					t.Fatal("automatic web primary did not use webpage image")
				}
				if tc.status != "found" && sl.Picture != nil {
					t.Fatal("failure must not invent a picture")
				}
			}
			if tc.status == "found" {
				route := "/api/tasks/" + id
				// Teacher reviews the actual preview, then expands and confirms the same task.
				for _, sl := range d.Slides {
					worksCall(t, s, "POST", route+"/slides/"+sl.ID+"/review", token, map[string]any{"revision": got["revision"], "hash": slideContentHash(d, sl), "approved": true}, 200)
				}
				worksCall(t, s, "POST", route+"/expand", token, map[string]any{"revision": got["revision"]}, 202)
				if err := s.workOne(context.Background()); err != nil {
					t.Fatal(err)
				}
				got = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
				json.Unmarshal(asJSON(got["result"]), &d)
				if got["status"] != "awaiting_confirmation" {
					t.Fatal("expansion failed", got["status"], got["error"])
				}
				for _, sl := range d.Slides {
					worksCall(t, s, "POST", route+"/slides/"+sl.ID+"/review", token, map[string]any{"revision": got["revision"], "hash": slideContentHash(d, sl), "approved": true}, 200)
				}
				worksCall(t, s, "POST", route+"/confirm", token, map[string]any{"revision": got["revision"], "result": d, "accept_review": true}, 202)
				if err := s.workOne(context.Background()); err != nil {
					t.Fatal(err)
				}
				got = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
				if got["status"] != "succeeded" || got["charge_state"] != "settled" {
					t.Fatal("delivery failed", got["status"], got["error"])
				}
				pptx := worksCall(t, s, "GET", route+"/download/pptx", token, nil, 200).Body.Bytes()
				z, err := zip.NewReader(bytes.NewReader(pptx), int64(len(pptx)))
				if err != nil {
					t.Fatal(err)
				}
				notes := ""
				found := false
				for _, f := range z.File {
					r, _ := f.Open()
					b, _ := io.ReadAll(r)
					r.Close()
					if strings.HasPrefix(f.Name, "ppt/notesSlides/") {
						notes += string(b)
					}
					if strings.HasPrefix(f.Name, "ppt/media/") && bytes.Equal(b, picture.Bytes()) {
						found = true
					}
				}
				if !found || strings.Count(notes, "Test Photographer") < 5 {
					t.Fatal("automatic images/source snapshots missing from delivered PPTX")
				}
				if dir := os.Getenv("COMMONS_TEST_ARTIFACTS"); dir != "" {
					os.MkdirAll(dir, 0700)
					os.WriteFile(filepath.Join(dir, "commons-automatic.pptx"), pptx, 0600)
				}
			} else {
				worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
			}
		})
	}
}

// Opt-in real Commons + configured vision probe. No paid courseware task is created.
func TestCoursewareCommonsLiveHTTP(t *testing.T) {
	if os.Getenv("RUN_COMMONS_LIVE") != "1" {
		t.Skip("explicit live probe only")
	}
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	s.cfg.Mode = "live"
	s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = os.Getenv("VISION_LLM_BASE_URL"), os.Getenv("VISION_LLM_API_KEY"), os.Getenv("VISION_LLM_MODEL")
	id := worksFixture(t, s, token, "courseware", "succeeded", "长城建筑观察", TaskInput{})
	task := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
	slide := task["result"].(map[string]any)["slides"].([]any)[0].(map[string]any)
	got := worksObject(t, worksCall(t, s, "POST", "/api/tasks/"+id+"/images/search", token, map[string]any{"revision": task["revision"], "slide_id": slide["id"], "need": ImageNeed{Purpose: "观察真实长城建筑及城墙连接", Queries: []string{"Great Wall Badaling photograph"}, Constraints: "真实建筑照片；不用于证明具体历史事件"}}, 200))
	if dir := os.Getenv("COMMONS_TEST_ARTIFACTS"); dir != "" {
		os.MkdirAll(dir, 0700)
		os.WriteFile(filepath.Join(dir, "live-probe.json"), asJSON(got), 0600)
	}
	t.Logf("actual Commons result: %v — %v", got["status"], got["message"])
	if got["status"] != "found" {
		t.Fatal("actual external chain not verified")
	}
}
