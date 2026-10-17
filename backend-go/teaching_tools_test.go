package main

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScientificDiagramsRejectImpossibleConditions(t *testing.T) {
	for _, diagram := range []ScientificDiagram{
		{Kind: "triangle", Title: "三角形", Sides: []float64{1, 2, 3}},
		{Kind: "forces", Title: "受力", Object: "物体", Unit: "N", Equilibrium: true, Forces: []ForceVector{{Label: "重力", X: 0, Y: -10}}},
		{Kind: "circuit", Title: "电路", Voltage: 6, Connection: "series", Resistances: []float64{0}},
	} {
		if validateTeachingMedia(Slide{Scientific: &diagram}) == nil {
			t.Fatalf("invalid diagram accepted: %s", diagram.Kind)
		}
	}
	for _, diagram := range []ScientificDiagram{
		{Kind: "triangle", Title: "三角形", Sides: []float64{3, 4, 5}},
		{Kind: "forces", Title: "平衡", Object: "静止木块", Unit: "N", Equilibrium: true, Forces: []ForceVector{{Label: "重力", Y: -10}, {Label: "支持力", Y: 10}}},
		{Kind: "circuit", Title: "并联", Voltage: 6, Connection: "parallel", Resistances: []float64{10, 20}},
	} {
		if err := validateTeachingMedia(Slide{Scientific: &diagram}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNonlinearFunctionsUseComputedValues(t *testing.T) {
	graph := &FunctionGraph{Title: "抛物线", XMin: -3, XMax: 3, YMin: -2, YMax: 10, Series: []FunctionSeries{{Kind: "quadratic", A: 1, MarkX: []float64{2}}}}
	if err := validateTeachingMedia(Slide{Graph: graph}); err != nil {
		t.Fatal(err)
	}
	graph.Series[0] = FunctionSeries{Kind: "inverse", K: 2, MarkX: []float64{0}}
	if validateTeachingMedia(Slide{Graph: graph}) == nil {
		t.Fatal("undefined point accepted")
	}
	graph.Series[0] = FunctionSeries{Kind: "unsupported"}
	if validateTeachingMedia(Slide{Graph: graph}) == nil {
		t.Fatal("unknown expression silently drawn")
	}
}

func TestMaterialSelectionUsesOriginalPagesAndStyleScope(t *testing.T) {
	pages := []Page{{1, "第十九章 一次函数\n函数的定义"}, {2, "19.1 函数\ny=kx+b"}, {3, "例题原文"}}
	chapters := materialChapters(pages)
	if len(chapters) != 2 || chapters[1].Start != 2 || chapters[1].End != 3 {
		t.Fatal(chapters)
	}
	selected, err := selectedCurriculum(pages, 2, 3)
	if err != nil || len(selected) != 2 || selected[0].Text != pages[1].Text {
		t.Fatal(selected, err)
	}
	for _, span := range [][2]int{{0, 2}, {3, 2}, {1, 21}, {2, 4}} {
		if _, err = selectedCurriculum(pages, span[0], span[1]); err == nil {
			t.Fatal("invalid page range accepted", span)
		}
	}
	in := TaskInput{ReferenceMode: "style", Reference: &ReferenceProfile{Layout: "blue-white split layout", Pages: []Page{{1, "UNRELATED ORIGINAL CONTENT"}}}}
	if strings.Contains(coursewarePrompt(in, nil), "UNRELATED ORIGINAL CONTENT") {
		t.Fatal("style reference leaks content")
	}
	in.ReferenceMode = "content"
	if !strings.Contains(coursewarePrompt(in, nil), "UNRELATED ORIGINAL CONTENT") {
		t.Fatal("content reference lost")
	}
}

func TestTeachingMaterialsAndReferencesIsolationHTTP(t *testing.T) {
	s := worksServer(t)
	teacher := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "other")
	admin := worksLogin(t, s, "admin")
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", teacher, nil, 200).Body.Bytes(), &a)
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	doc, ref := newID(), newID()
	_, err = tx.Exec(context.Background(), "INSERT INTO documents(id,tenant_id,member_id,name,status,pages,curriculum) VALUES($1,$2,$3,'人教版数学','ready',$4,$5)", doc, a.TenantID, a.MemberID, asJSON([]Page{{1, "第十九章 一次函数"}, {2, "y=kx+b，k不为零"}}), asJSON(CurriculumTag{"数学", "八年级", "人教版"}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), "INSERT INTO courseware_references(id,tenant_id,member_id,name,profile) VALUES($1,$2,$3,'本人参考',$4)", ref, a.TenantID, a.MemberID, asJSON(ReferenceProfile{Name: "本人参考", Theme: "ocean", Layout: "左右分栏", Typography: "大标题", ObservedPages: []int{1}}))
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	in := TaskInput{Subject: "数学", Grade: "八年级", Textbook: "人教版", MaterialID: doc, MaterialStart: 1, MaterialEnd: 2, ReferenceID: ref}
	worksCall(t, s, "POST", "/api/courseware/materials/preview", teacher, in, 200)
	worksCall(t, s, "POST", "/api/courseware/materials/preview", other, in, 404)
	private := in
	private.MaterialID = ""
	worksCall(t, s, "POST", "/api/courseware/materials/preview", admin, private, 404)
	bad := in
	bad.Grade = "九年级"
	worksCall(t, s, "POST", "/api/courseware/materials/preview", teacher, bad, 400)
	worksCall(t, s, "DELETE", "/api/courseware/references/"+ref, other, nil, 404)
	worksCall(t, s, "DELETE", "/api/courseware/references/"+ref, teacher, nil, 200)
	worksCall(t, s, "PATCH", "/api/documents/"+doc+"/curriculum", other, CurriculumTag{"数学", "八年级", "人教版"}, 404)
	snapshot := TaskInput{Curriculum: []Page{{1, "FORGED CONTENT"}}, Reference: &ReferenceProfile{Layout: "FORGED"}}
	if err = s.resolveTeachingMaterials(context.Background(), a, &snapshot); err != nil || snapshot.Curriculum != nil || snapshot.Reference != nil {
		t.Fatal("client can forge verified material")
	}
}

func TestReferenceUploadUsesActualImagesAndRetainsNoBinaryHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	s.cfg.Mode = "live"
	s.cfg.VisionModel = "fixture-vision"
	s.cfg.VisionKey = "fixture-only"
	imageURL := "data:image/jpeg;base64,fixture"
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if !strings.Contains(string(asJSON(body)), imageURL) {
			t.Error("reference image not sent")
		}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(asJSON(ReferenceProfile{Theme: "ocean", Typography: "large titles", Layout: "split columns", Rhythm: "reason then example", DiagramStyle: "numerical plot"}))}}}})
	}))
	defer provider.Close()
	s.cfg.VisionURL = provider.URL
	renderer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"pages": []Page{{1, "original source text"}}, "images": []any{map[string]any{"page": 1, "data_url": imageURL}}})
	}))
	defer renderer.Close()
	s.cfg.RenderURL = renderer.URL
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	p, _ := mw.CreateFormFile("file", "reference.pdf")
	p.Write([]byte("fixture uploaded bytes"))
	mw.Close()
	request := httptest.NewRequest("POST", "/api/courseware/references", &buf)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", mw.FormDataContentType())
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != 201 {
		t.Fatal(response.Code, response.Body.String())
	}
	result := worksObject(t, response)
	if calls != 1 {
		t.Fatal("duplicate model analysis", calls)
	}
	listed := worksCall(t, s, "GET", "/api/courseware/references", token, nil, 200)
	if strings.Contains(listed.Body.String(), imageURL) || strings.Contains(listed.Body.String(), "fixture uploaded bytes") {
		t.Fatal("binary retained")
	}
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	in := TaskInput{ReferenceID: result["id"].(string)}
	if err := s.resolveTeachingMaterials(context.Background(), a, &in); err != nil || in.Reference == nil || len(in.Reference.Pages) != 1 || in.Reference.Pages[0].Text != "original source text" {
		t.Fatal("reference snapshot unavailable", err)
	}
	worksCall(t, s, "DELETE", "/api/courseware/references/"+in.ReferenceID, token, nil, 200)
	if in.Reference.Layout != "split columns" {
		t.Fatal("deleting reference mutated work snapshot")
	}
}

func TestTeachingReferenceLive(t *testing.T) {
	if os.Getenv("TEST_TEACHING_REFERENCE_LIVE") != "1" {
		t.Skip("explicit live reference analysis opt-in")
	}
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	s.cfg.Mode = "live"
	s.cfg.VisionURL = os.Getenv("VISION_LLM_BASE_URL")
	s.cfg.VisionKey = os.Getenv("VISION_LLM_API_KEY")
	s.cfg.VisionModel = os.Getenv("VISION_LLM_MODEL")
	raw, err := os.ReadFile("../work/teaching-tools/scientific-sample.pptx")
	if err != nil {
		t.Fatal(err)
	}
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("file", "scientific-reference.pptx")
	part.Write(raw)
	mw.Close()
	r := httptest.NewRequest("POST", "/api/courseware/references", &buf)
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 201 {
		t.Fatal("live reference analysis failed", w.Code, w.Body.String())
	}
	var result struct {
		ID      string           `json:"id"`
		Profile ReferenceProfile `json:"profile"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Profile.ObservedPages) != 4 || result.Profile.TotalPages != 4 || result.Profile.Layout == "" {
		t.Fatal("incomplete reference profile")
	}
	after := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	if before != after {
		t.Fatal("reference analysis changed business credits")
	}
	files, err := filepath.Glob(filepath.Join(s.cfg.Storage, "*", "reference-*"))
	if err != nil || len(files) > 0 {
		t.Fatal("temporary reference files retained", files, err)
	}
	os.WriteFile("../work/teaching-tools/live-reference.json", asJSON(map[string]any{"model": s.cfg.VisionModel, "profile": result.Profile, "business_accounting_unchanged": true}), 0600)
	worksCall(t, s, "DELETE", "/api/courseware/references/"+result.ID, token, nil, 200)
}
