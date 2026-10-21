package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// Uses the real extraction service and real PPTX output; relevance alone is a labelled fixture.
func TestKnowledgeFiguresOfflineHTTP(t *testing.T) {
	dir := os.Getenv("KNOWLEDGE_IMAGE_FIXTURES")
	if dir == "" {
		t.Skip("requires generated PDF/PPTX and candidate renderer")
	}
	s := worksServer(t)
	if renderer := os.Getenv("KNOWLEDGE_IMAGE_RENDER_URL"); renderer != "" {
		s.cfg.RenderURL = renderer
	}
	teacher, admin, foreign := worksLogin(t, s, "teacher"), worksLogin(t, s, "admin"), worksLogin(t, s, "other")
	bill := worksCall(t, s, "GET", "/api/billing", teacher, nil, 200).Body.String()
	for _, suffix := range []string{"pdf", "pptx"} {
		s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = "", "", ""
		doc := uploadKnowledgeImageFixture(t, s, admin, filepath.Join(dir, "cycle."+suffix))
		if err := s.ingestOne(context.Background()); err != nil {
			t.Fatal(err)
		}
		route := "/api/documents/" + doc + "/images"
		worksCall(t, s, "GET", route, "", nil, 401)
		worksCall(t, s, "GET", route, foreign, nil, 404)
		figures := worksObject(t, worksCall(t, s, "GET", route, teacher, nil, 200))
		items, ok := figures["images"].([]any)
		if !ok || len(items) != 1 {
			t.Fatalf("extracted figures missing: %v", figures)
		}
		figure := items[0].(map[string]any)
		if figure["page"] != float64(1) {
			t.Fatal(figure)
		}
		asset := figure["asset_id"].(string)
		worksCall(t, s, "GET", "/api/media/"+asset, teacher, nil, 200)
		worksCall(t, s, "GET", "/api/media/"+asset, admin, nil, 404)
		id := worksFixture(t, s, teacher, "courseware", "succeeded", "水循环", TaskInput{UseKB: true, MaterialID: doc})
		task := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, teacher, nil, 200))
		deck := task["result"].(map[string]any)
		slide := deck["slides"].([]any)[0].(map[string]any)
		body := map[string]any{"revision": task["revision"], "slide_id": slide["id"], "need": map[string]any{"purpose": "观察水循环", "queries": []string{"Water cycle"}}}
		// Without an image reviewer, local candidate selection must not silently pass.
		result := worksObject(t, worksCall(t, s, "POST", "/api/tasks/"+id+"/images/search", teacher, body, 200))
		if result["status"] == "found" {
			t.Fatal("unreviewed image adopted")
		}
		reviewer := httptest.NewServer(httpHandlerImageRelevant(t))
		s.cfg.VisionURL, s.cfg.VisionKey, s.cfg.VisionModel = reviewer.URL, "fixture", "fixture"
		result = worksObject(t, worksCall(t, s, "POST", "/api/tasks/"+id+"/images/search", teacher, body, 200))
		reviewer.Close()
		if result["status"] != "found" {
			t.Fatal("offline local image was not used", result)
		}
		selected := result["candidates"].([]any)[0].(map[string]any)
		provenance := selected["provenance"].(map[string]any)
		if provenance["provider"] != "knowledge_document" || provenance["document_id"] != doc || provenance["page"] != float64(1) {
			t.Fatal(provenance)
		}
		slide["picture"] = selected["picture"]
		delete(slide, "graph")
		delete(slide, "scientific")
		delete(slide, "chart")
		delete(slide, "diagram")
		delete(slide, "figure")
		s.cfg.CoursewareReviewPolicy = "none"
		s.cfg.CoursewareContentReviewDisabled = true
		worksCall(t, s, "POST", "/api/tasks/"+id+"/edit", teacher, map[string]any{"revision": task["revision"], "result": deck}, 200)
		output := worksCall(t, s, "GET", "/api/tasks/"+id+"/download/pptx", teacher, nil, 200).Body.Bytes()
		archive, err := zip.NewReader(bytes.NewReader(output), int64(len(output)))
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, f := range archive.File {
			if len(f.Name) > 15 && f.Name[:15] == "ppt/notesSlides" {
				r, _ := f.Open()
				b, _ := io.ReadAll(r)
				r.Close()
				found = found || bytes.Contains(b, []byte(doc)) && bytes.Contains(b, []byte("第1页"))
			}
		}
		if !found {
			t.Fatal("source document/page missing from actual PPTX notes")
		}
		if suffix == "pdf" {
			worksCall(t, s, "POST", "/api/documents/"+doc+"/reparse", "", nil, 401)
			worksCall(t, s, "POST", "/api/documents/"+doc+"/reparse", foreign, nil, 409)
			worksCall(t, s, "POST", "/api/documents/"+doc+"/reparse", admin, nil, 202)
			worksCall(t, s, "POST", "/api/documents/"+doc+"/reparse", admin, nil, 409)
			if err := s.ingestOne(context.Background()); err != nil {
				t.Fatal(err)
			}
			reused := worksObject(t, worksCall(t, s, "GET", route, teacher, nil, 200))["images"].([]any)
			if len(reused) != 1 || reused[0].(map[string]any)["asset_id"] != asset {
				t.Fatal("reparse duplicated or lost immutable asset")
			}
		}
		worksCall(t, s, "DELETE", "/api/documents/"+doc, admin, nil, 200)
		worksCall(t, s, "GET", route, teacher, nil, 404)
		worksCall(t, s, "GET", "/api/media/"+asset, teacher, nil, 200) // immutable copies preserve old deliveries
	}
	if worksCall(t, s, "GET", "/api/billing", teacher, nil, 200).Body.String() != bill {
		t.Fatal("figure reuse changed member credits")
	}
}

func httpHandlerImageRelevant(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("invalid review request")
		}
		if !bytes.Contains(asJSON(body), []byte("data:image/png;base64,")) {
			t.Error("reviewer did not receive actual extracted image")
		}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": `{"relevant":true,"reason":"固定验收：图示符合水循环观察用途"}`}}}})
	})
}

func uploadKnowledgeImageFixture(t *testing.T, s *Server, token, file string) string {
	t.Helper()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", filepath.Base(file))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(raw); err != nil {
		t.Fatal(err)
	}
	mw.Close()
	request := httptest.NewRequest("POST", "/api/documents", &body)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", mw.FormDataContentType())
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != 202 {
		t.Fatal(response.Code, response.Body.String())
	}
	return worksObject(t, response)["id"].(string)
}
