package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Public HTTP seam; only external search, webpage and model replies are fixtures.
func TestCoursewareWebImagesHTTP(t *testing.T) {
	s := worksServer(t)
	teacher, other := worksLogin(t, s, "teacher"), worksLogin(t, s, "other")
	id := worksFixture(t, s, teacher, "courseware", "succeeded", "长城", TaskInput{})
	task := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, teacher, nil, 200))
	slide := task["result"].(map[string]any)["slides"].([]any)[0].(map[string]any)
	var pngBytes bytes.Buffer
	png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 320, 240)))
	var variant, commonsCalls atomic.Int32
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			var q map[string]any
			json.NewDecoder(r.Body).Decode(&q)
			if q["api_key"] != "fixture" || !strings.Contains(q["query"].(string), "长城") {
				t.Error("existing configured search contract not used")
			}
			respond(w, 200, map[string]any{"results": []any{map[string]string{"title": "长城照片", "url": "https://museum.example/collection/wall"}}})
		case "/collection/wall":
			if variant.Load() == 2 {
				http.Redirect(w, r, "http://127.0.0.1/private", 302)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			license := `<a rel="license" href="https://creativecommons.org/licenses/by/4.0/">CC BY 4.0</a>`
			if variant.Load() == 1 || variant.Load() == 4 {
				license = ""
			}
			src := "/images/wall.png"
			if variant.Load() == 3 {
				src = "http://127.0.0.1/private"
			}
			io.WriteString(w, `<html><head><title>长城建筑照片</title><meta name="author" content="Web Photographer"></head><body><figure><img src="`+src+`" alt="八达岭长城城墙与敌台"><figcaption>长城建筑观察。`+license+`</figcaption></figure></body></html>`)
		case "/images/wall.png":
			w.Header().Set("Content-Type", "image/png")
			w.Write(pngBytes.Bytes())
		case "/w/api.php":
			commonsCalls.Add(1)
			if variant.Load() == 4 {
				respond(w, 200, map[string]any{"query": map[string]any{"pages": []any{map[string]any{"title": "File:Fallback.png", "imageinfo": []any{map[string]any{"url": "https://upload.wikimedia.org/wikipedia/commons/f/f0/Fallback.png", "descriptionurl": "https://commons.wikimedia.org/wiki/File:Fallback.png", "mime": "image/png", "width": 320, "height": 240, "extmetadata": map[string]any{"Artist": map[string]string{"value": "Fallback Photographer"}, "LicenseShortName": map[string]string{"value": "CC BY 4.0"}, "LicenseUrl": map[string]string{"value": "https://creativecommons.org/licenses/by/4.0/"}}}}}}}})
				return
			}
			w.WriteHeader(503)
		case "/wikipedia/commons/f/f0/Fallback.png":
			w.Write(pngBytes.Bytes())
		case "/chat/completions":
			respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": `{"relevant":true,"reason":"固定视觉审核：长城建筑照片支持观察。"}`}}}})
		default:
			t.Errorf("unexpected external request %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer external.Close()
	u, _ := url.Parse(external.URL)
	s.mediaHTTP = &http.Client{Transport: commonsFixtureTransport{u}}
	s.cfg.SearchURL, s.cfg.SearchKey = external.URL+"/search", "fixture"
	s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = external.URL, "fixture", "fixture"
	route := "/api/tasks/" + id + "/images/search"
	body := map[string]any{"revision": task["revision"], "slide_id": slide["id"], "need": ImageNeed{Purpose: "观察长城防御设施", Queries: []string{"长城 建筑 照片"}}}
	bill := worksCall(t, s, "GET", "/api/billing", teacher, nil, 200).Body.String()
	worksCall(t, s, "POST", route, "", body, 401)
	worksCall(t, s, "POST", route, other, body, 404)
	got := worksObject(t, worksCall(t, s, "POST", route, teacher, body, 200))
	items := got["candidates"].([]any)
	if got["status"] != "found" || len(items) != 1 {
		t.Fatalf("web image must be primary: %v", got)
	}
	p := items[0].(map[string]any)
	prov := p["provenance"].(map[string]any)
	if prov["provider"] != "web_page" || prov["page_url"] != "https://museum.example/collection/wall" || prov["license"] != "CC BY 4.0" || commonsCalls.Load() != 0 {
		t.Fatal("web source or priority missing", p)
	}
	asset := p["picture"].(map[string]any)["asset_id"].(string)
	worksCall(t, s, "GET", "/api/media/"+asset, other, nil, 404)
	if render := os.Getenv("COMMONS_TEST_RENDER_URL"); render != "" {
		s.cfg.RenderURL = render
		deck := task["result"].(map[string]any)
		slide["picture"] = p["picture"]
		worksCall(t, s, "POST", "/api/tasks/"+id+"/edit", teacher, map[string]any{"revision": task["revision"], "result": deck}, 200)
		raw := worksCall(t, s, "GET", "/api/tasks/"+id+"/download/pptx", teacher, nil, 200).Body.Bytes()
		z, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			t.Fatal(err)
		}
		notes := ""
		for _, f := range z.File {
			if strings.HasPrefix(f.Name, "ppt/notesSlides/notesSlide") {
				r, _ := f.Open()
				b, _ := io.ReadAll(r)
				r.Close()
				notes += string(b)
			}
		}
		if !strings.Contains(notes, "Web Photographer") || !strings.Contains(notes, "https://museum.example/collection/wall") {
			t.Fatal("PPTX lost webpage attribution")
		}
		os.WriteFile("../work/web-images/web-http.pptx", raw, 0600)
		task = worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, teacher, nil, 200))
		body["revision"] = task["revision"]
	}
	variant.Store(1)
	got = worksObject(t, worksCall(t, s, "POST", route, teacher, body, 200))
	items = got["candidates"].([]any)
	if got["status"] != "needs_confirmation" || len(items) != 1 || commonsCalls.Load() == 0 {
		t.Fatalf("unknown rights require confirmation after fallback: %v", got)
	}
	pending := items[0].(map[string]any)
	pendingID := pending["picture"].(map[string]any)["asset_id"].(string)
	if os.Getenv("COMMONS_TEST_RENDER_URL") != "" {
		deck := task["result"].(map[string]any)
		first := deck["slides"].([]any)[0].(map[string]any)
		original := first["picture"]
		first["picture"] = pending["picture"]
		worksCall(t, s, "POST", "/api/tasks/"+id+"/edit", teacher, map[string]any{"revision": task["revision"], "result": deck}, 400)
		first["picture"] = original
	}
	confirmation := map[string]any{"revision": task["revision"], "slide_id": slide["id"], "asset_id": pendingID, "rights_note": "已取得作者用于教学课件及下载分发的许可"}
	confirmRoute := "/api/tasks/" + id + "/images/confirm"
	worksCall(t, s, "POST", confirmRoute, "", confirmation, 401)
	worksCall(t, s, "POST", confirmRoute, other, confirmation, 404)
	stale := map[string]any{"revision": -1, "slide_id": slide["id"], "asset_id": pendingID, "rights_note": confirmation["rights_note"]}
	worksCall(t, s, "POST", confirmRoute, teacher, stale, 409)
	confirmed := worksObject(t, worksCall(t, s, "POST", confirmRoute, teacher, confirmation, 200))
	if confirmed["provenance"].(map[string]any)["rights_status"] != "teacher_confirmed" {
		t.Fatal("confirmation not recorded")
	}
	if os.Getenv("COMMONS_TEST_RENDER_URL") != "" {
		deck := task["result"].(map[string]any)
		deck["slides"].([]any)[0].(map[string]any)["picture"] = confirmed["picture"]
		worksCall(t, s, "POST", "/api/tasks/"+id+"/edit", teacher, map[string]any{"revision": task["revision"], "result": deck}, 200)
		raw := worksCall(t, s, "GET", "/api/tasks/"+id+"/download/pptx", teacher, nil, 200).Body.Bytes()
		z, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			t.Fatal(err)
		}
		notes := ""
		for _, f := range z.File {
			if strings.HasPrefix(f.Name, "ppt/notesSlides/notesSlide") {
				r, _ := f.Open()
				b, _ := io.ReadAll(r)
				r.Close()
				notes += string(b)
			}
		}
		if !strings.Contains(notes, confirmation["rights_note"].(string)) || !strings.Contains(notes, "teacher_confirmed") {
			t.Fatal("PPTX lost teacher rights declaration")
		}
		os.WriteFile("../work/web-images/web-confirmed-http.pptx", raw, 0600)
		task = worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, teacher, nil, 200))
		body["revision"] = task["revision"]
	}
	variant.Store(4)
	fallback := worksObject(t, worksCall(t, s, "POST", route, teacher, body, 200))
	if fallback["status"] != "found" || fallback["candidates"].([]any)[0].(map[string]any)["provenance"].(map[string]any)["provider"] != "wikimedia_commons" {
		t.Fatal("Commons backup did not supply usable image", fallback)
	}
	for _, v := range []int32{2, 3} {
		variant.Store(v)
		result := worksObject(t, worksCall(t, s, "POST", route, teacher, body, 200))
		if len(result["candidates"].([]any)) != 0 {
			t.Fatal("private URL or redirect allowed")
		}
	}
	t.Run("paused-draft", func(t *testing.T) {
		previousMode := s.cfg.Mode
		s.cfg.Mode = "live"
		defer func() { s.cfg.Mode = previousMode }()
		variant.Store(1)
		pausedID := worksFixture(t, s, teacher, "courseware", "failed", "长城配图暂停草稿", TaskInput{PreviewFirst: true})
		pausedRoute := "/api/tasks/" + pausedID
		paused := worksObject(t, worksCall(t, s, "GET", pausedRoute, teacher, nil, 200))
		deck := paused["result"].(map[string]any)
		first := deck["slides"].([]any)[0].(map[string]any)
		first["bullets"] = []string{"观察城墙与敌台如何连接。"}
		first["notes"] = "固定验收样例：观察城墙与敌台的连接，具体年代与用途需要教师核对。"
		deck["slides"] = []any{first}
		checkpoint := map[string]any{"version": coursewarePageRepairVersion, "phase": "preview", "repair_page": 1, "issue": "尚未通过内容审核", "page_attempts": map[string]int{"1": 3}}
		deck["generation"] = checkpoint
		var actor Actor
		json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", teacher, nil, 200).Body.Bytes(), &actor)
		tx, err := s.tx(context.Background(), actor.TenantID)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(context.Background(), "UPDATE tasks SET result=$1,charge_state='released',mode='live' WHERE id=$2", asJSON(deck), pausedID)
		if err == nil {
			err = tx.Commit(context.Background())
		} else {
			tx.Rollback(context.Background())
		}
		if err != nil {
			t.Fatal(err)
		}
		q := map[string]any{"revision": paused["revision"], "slide_id": first["id"], "need": ImageNeed{Purpose: "观察长城建筑", Queries: []string{"长城"}}}
		worksCall(t, s, "POST", pausedRoute+"/images/search", other, q, 404)
		result := worksObject(t, worksCall(t, s, "POST", pausedRoute+"/images/search", teacher, q, 200))
		items, ok := result["candidates"].([]any)
		if !ok || len(items) != 1 || result["status"] != "needs_confirmation" {
			t.Fatalf("paused live draft lost candidate: %v", result)
		}
		candidate := items[0].(map[string]any)
		first["image_search"] = map[string]any{"status": "needs_confirmation", "message": result["message"], "candidates": items}
		retained := map[string]any{"revision": paused["revision"], "result": deck}
		worksCall(t, s, "POST", pausedRoute+"/draft", teacher, retained, 200)
		paused = worksObject(t, worksCall(t, s, "GET", pausedRoute, teacher, nil, 200))
		deck = paused["result"].(map[string]any)
		first = deck["slides"].([]any)[0].(map[string]any)
		state := first["image_search"].(map[string]any)
		if first["picture"] != nil || len(state["candidates"].([]any)) != 1 || paused["status"] != "failed" || paused["charge_state"] != "released" {
			t.Fatal("pending candidate was discarded, inserted or restarted task")
		}
		first["picture"] = candidate["picture"]
		save := map[string]any{"revision": paused["revision"], "result": deck}
		worksCall(t, s, "POST", pausedRoute+"/draft", "", save, 401)
		worksCall(t, s, "POST", pausedRoute+"/draft", other, save, 404)
		worksCall(t, s, "POST", pausedRoute+"/draft", teacher, save, 400)
		confirm := map[string]any{"revision": paused["revision"], "slide_id": first["id"], "asset_id": candidate["picture"].(map[string]any)["asset_id"], "rights_note": "固定验收：已获得作者教学使用许可"}
		approved := worksObject(t, worksCall(t, s, "POST", pausedRoute+"/images/confirm", teacher, confirm, 200))
		first["picture"] = approved["picture"]
		deck["generation"] = nil // Client cannot erase the recovery checkpoint.
		worksCall(t, s, "POST", pausedRoute+"/draft", teacher, save, 200)
		worksCall(t, s, "POST", pausedRoute+"/draft", teacher, save, 409)
		after := worksObject(t, worksCall(t, s, "GET", pausedRoute, teacher, nil, 200))
		if after["status"] != "failed" || after["charge_state"] != "released" {
			t.Fatal("image edit restarted or charged paused task")
		}
		current := after["result"].(map[string]any)
		if string(asJSON(current["generation"])) != string(asJSON(checkpoint)) {
			t.Fatal("recovery checkpoint changed")
		}
		if len(current["slides"].([]any)) != 1 {
			t.Fatal("partial draft expanded")
		}
		preview := worksCall(t, s, "GET", pausedRoute+"/preview-pptx", teacher, nil, 200).Body.Bytes()
		z, e := zip.NewReader(bytes.NewReader(preview), int64(len(preview)))
		if e != nil {
			t.Fatal(e)
		}
		found := false
		notes := ""
		for _, f := range z.File {
			r, _ := f.Open()
			b, _ := io.ReadAll(r)
			r.Close()
			if strings.HasPrefix(f.Name, "ppt/media/") && bytes.Equal(b, pngBytes.Bytes()) {
				found = true
			}
			if strings.HasPrefix(f.Name, "ppt/notesSlides/") {
				notes += string(b)
			}
		}
		if !found || !strings.Contains(notes, confirm["rights_note"].(string)) {
			t.Fatal("paused draft PPTX lost image or rights declaration")
		}
		worksCall(t, s, "POST", pausedRoute+"/confirm", teacher, map[string]any{"revision": after["revision"], "result": current, "accept_review": true}, 400)
	})
	if worksCall(t, s, "GET", "/api/billing", teacher, nil, 200).Body.String() != bill {
		t.Fatal("web image selection charged generation credits")
	}
}

// Opt-in external search + webpage download + configured vision, no paid task.
type webImagesProbeTransport struct {
	base http.RoundTripper
	t    *testing.T
}

func (p webImagesProbeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	start := time.Now()
	response, err := p.base.RoundTrip(r)
	status := 0
	if response != nil {
		status = response.StatusCode
	}
	p.t.Logf("external host=%s status=%d elapsed=%s error_type=%T", r.URL.Hostname(), status, time.Since(start).Round(time.Millisecond), err)
	return response, err
}
func TestCoursewareWebImagesLiveHTTP(t *testing.T) {
	if os.Getenv("RUN_WEB_IMAGES_LIVE") != "1" {
		t.Skip("explicit real external probe only")
	}
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	s.cfg.SearchURL, s.cfg.SearchKey = os.Getenv("SEARCH_API_URL"), os.Getenv("SEARCH_API_KEY")
	s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = os.Getenv("VISION_LLM_BASE_URL"), os.Getenv("VISION_LLM_API_KEY"), os.Getenv("VISION_LLM_MODEL")
	s.cfg.Mode = "live"
	s.mediaHTTP.Transport = webImagesProbeTransport{s.mediaHTTP.Transport, t}
	id := worksFixture(t, s, token, "courseware", "succeeded", "观察长城建筑", TaskInput{})
	task := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
	slide := task["result"].(map[string]any)["slides"].([]any)[0].(map[string]any)
	got := worksObject(t, worksCall(t, s, "POST", "/api/tasks/"+id+"/images/search", token, map[string]any{"revision": task["revision"], "slide_id": slide["id"], "need": ImageNeed{Purpose: "观察长城城墙和敌台连接", Queries: []string{"长城 八达岭 建筑 照片"}, Constraints: "实际建筑照片，不用于证明具体历史事件"}}, 200))
	os.WriteFile("../work/web-images/live-http.json", asJSON(got), 0600)
	t.Logf("real external result: %v", got["status"])
	items, ok := got["candidates"].([]any)
	if !ok || len(items) == 0 {
		t.Fatal("real external image chain did not return usable candidates")
	}
	for _, item := range items {
		if item.(map[string]any)["provenance"].(map[string]any)["provider"] != "web_page" {
			t.Fatal("probe returned backup instead of webpage image")
		}
	}
}
