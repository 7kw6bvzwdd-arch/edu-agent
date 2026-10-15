package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// Fixtures are constructed examples, not publisher textbook excerpts.
func TestTeachingClaimEvidenceRejectsForgedSupport(t *testing.T) {
	var sources []Source
	json.Unmarshal([]byte(`[{"id":"S1","kind":"teacher_selected","document_id":"book-a","hash":"abc","content":"在 y=kx+b 中，k≠0。","evidence_kind":"extracted_page"},{"id":"S2","kind":"teacher_selected","hash":"toc","content":"目录\n第二十六章 二次函数 29","evidence_kind":"catalogue"}]`), &sources)
	for _, test := range []struct {
		name, evidence string
		wantError      bool
	}{
		{"exact quote", `[{"claim":"k≠0","source_id":"S1","source_hash":"abc","quote":"k≠0"}]`, false},
		{"fabricated quote", `[{"claim":"k≠0","source_id":"S1","source_hash":"abc","quote":"k=0"}]`, true},
		{"unknown source", `[{"claim":"k≠0","source_id":"S999","source_hash":"abc","quote":"k≠0"}]`, true},
		{"stale hash", `[{"claim":"k≠0","source_id":"S1","source_hash":"old","quote":"k≠0"}]`, true},
		{"absent claim", `[{"claim":"未在课件中出现的结论","source_id":"S1","source_hash":"abc","quote":"k≠0"}]`, true},
		{"catalogue is not body", `[{"claim":"k≠0","source_id":"S2","source_hash":"toc","quote":"第二十六章"}]`, true},
		{"missing association", `[]`, true},
		{"field anchored claim", `[{"field":"bullets","index":0,"source_id":"S1","source_hash":"abc","quote":"k≠0"}]`, false},
		{"unknown field", `[{"field":"evidence","index":0,"source_id":"S1","source_hash":"abc","quote":"k≠0"}]`, true},
		{"missing bullet index", `[{"field":"bullets","source_id":"S1","source_hash":"abc","quote":"k≠0"}]`, true},
		{"out of range field", `[{"field":"bullets","index":5,"source_id":"S1","source_hash":"abc","quote":"k≠0"}]`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var d Deck
			json.Unmarshal([]byte(`{"slides":[{"title":"一次函数","bullets":["k≠0"],"notes":"依据 [S1]","evidence":`+test.evidence+`}]}`), &d)
			if err := coursewareSourceError(d, sources); (err != nil) != test.wantError {
				t.Fatalf("evidence validation: %v", err)
			}
		})
	}
	// Old works do not gain a new claim of verified support or become inaccessible.
	sources[0] = Source{ID: "S1", Content: "在 y=kx+b 中，k≠0。"}
	if err := coursewareSourceError(Deck{Slides: []Slide{{Notes: "依据 [S1]"}}}, sources); err != nil {
		t.Fatal(err)
	}
}

func TestTextbookClaimEvidenceWorkflowHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "other")
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	const original = "浮力方向竖直向上，来自液体对物体压力的合力。"
	pages := []Page{{1, "目录\n第十章 浮力"}, {2, original}, {3, "阿基米德原理的适用条件需要核对。"}}
	doc, err := s.saveDocument(context.Background(), a, "构造教学证据样例.txt", "", pages)
	if err != nil {
		t.Fatal(err)
	}
	worksCall(t, s, "PATCH", "/api/documents/"+doc+"/curriculum", token, CurriculumTag{"物理", "八年级", "人教版 样例版次A（非教材）"}, 200)
	in := TaskInput{Type: "courseware", Topic: "浮力", Subject: "物理", Grade: "八年级", Textbook: "人教版", Chapter: "样例章节", MaterialID: doc, MaterialStart: 2, MaterialEnd: 3, Pages: 8, Theme: "ocean", PreviewFirst: true, UseKB: true, IdempotencyKey: newID()}
	in.MaterialSnapshot = &TextbookSnapshot{Edition: "伪造版本", DocumentHash: "forged"}
	preview := worksObject(t, worksCall(t, s, "POST", "/api/courseware/materials/preview", token, in, 200))
	if preview["material_snapshot"].(map[string]any)["document_hash"] != textbookDocumentHash(pages) {
		t.Fatal("client can forge version snapshot")
	}
	worksCall(t, s, "POST", "/api/courseware/materials/preview", other, in, 404)
	in.IdempotencyKey = newID()
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	route := "/api/tasks/" + id
	if err = s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	v := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if v["status"] != "awaiting_preview" {
		t.Fatal(v["status"], v["error"])
	}
	var d Deck
	var sources []Source
	json.Unmarshal(asJSON(v["result"]), &d)
	json.Unmarshal(asJSON(v["sources"]), &sources)
	var src Source
	for _, x := range sources {
		if x.DocumentID == doc && x.FilePage == 2 {
			src = x
		}
	}
	if src.ID == "" {
		t.Fatal("selected source missing")
	}
	d.Slides[0].Bullets = []string{original, "观察浮力的方向与来源。"}
	d.Slides[0].Notes += "\n依据 [" + src.ID + "]"
	d.Slides[0].Evidence = []TeachingClaimEvidence{{Claim: original, SourceID: src.ID, SourceHash: src.Hash, Quote: original}}
	d.Slides[0].Evidence[0].Origin = &TeachingEvidenceOrigin{Title: "伪造出处", Location: "印刷第999页"}
	worksCall(t, s, "POST", route+"/draft", other, map[string]any{"revision": v["revision"], "result": d}, 404)
	worksCall(t, s, "POST", route+"/draft", token, map[string]any{"revision": v["revision"], "result": d}, 200)
	v = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	json.Unmarshal(asJSON(v["result"]), &d)
	if d.Slides[0].Evidence[0].Origin.Title == "伪造出处" {
		t.Fatal("client origin survived")
	}
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	bad := d
	bad.Slides = append([]Slide(nil), d.Slides...)
	bad.Slides[0].Evidence = append([]TeachingClaimEvidence(nil), d.Slides[0].Evidence...)
	bad.Slides[0].Evidence[0].Quote = "不存在的原文"
	worksCall(t, s, "POST", route+"/draft", token, map[string]any{"revision": v["revision"], "result": bad}, 400)
	if got := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200)); string(asJSON(got["result"])) != string(asJSON(v["result"])) {
		t.Fatal("failed save changed work")
	}
	if before != worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() {
		t.Fatal("evidence editing charged credits")
	}
	// Later teacher relabeling does not rewrite the saved work's original version.
	worksCall(t, s, "PATCH", "/api/documents/"+doc+"/curriculum", token, CurriculumTag{"物理", "八年级", "人教版 样例版次B（非教材）"}, 200)
	for _, entry := range v["slide_reviews"].([]any) {
		m := entry.(map[string]any)
		worksCall(t, s, "POST", route+"/slides/"+m["id"].(string)+"/review", token, map[string]any{"revision": v["revision"], "hash": m["hash"], "approved": true}, 200)
	}
	worksCall(t, s, "POST", route+"/expand", token, map[string]any{"revision": v["revision"]}, 202)
	if err = s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	v = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	if v["status"] != "awaiting_confirmation" {
		t.Fatal(v["status"], v["error"])
	}
	json.Unmarshal(asJSON(v["result"]), &d)
	if d.Slides[0].Evidence[0].Origin.Textbook.Edition != "人教版 样例版次A（非教材）" {
		t.Fatal("resume switched edition")
	}
	worksCall(t, s, "GET", route, other, nil, 404)
	for _, entry := range v["slide_reviews"].([]any) {
		m := entry.(map[string]any)
		worksCall(t, s, "POST", route+"/slides/"+m["id"].(string)+"/review", token, map[string]any{"revision": v["revision"], "hash": m["hash"], "approved": true}, 200)
	}
	worksCall(t, s, "POST", route+"/confirm", token, map[string]any{"revision": v["revision"], "result": d, "accept_review": true}, 202)
	if err = s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	ppt := worksCall(t, s, "GET", route+"/download/pptx", token, nil, 200).Body.Bytes()
	z, err := zip.NewReader(bytes.NewReader(ppt), int64(len(ppt)))
	if err != nil {
		t.Fatal(err)
	}
	var notes strings.Builder
	for _, f := range z.File {
		if strings.HasPrefix(f.Name, "ppt/notesSlides/notesSlide") && strings.HasSuffix(f.Name, ".xml") {
			r, _ := f.Open()
			b, _ := io.ReadAll(r)
			r.Close()
			notes.Write(b)
		}
	}
	for _, want := range []string{"原文关联（不等于事实认证）", "样例版次A", textbookDocumentHash(pages), src.Hash} {
		if !strings.Contains(notes.String(), want) {
			t.Fatal("export loses evidence", want)
		}
	}
	json.Unmarshal(asJSON(worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))["result"]), &d)
	bad = d
	bad.Slides = append([]Slide(nil), d.Slides...)
	bad.Slides[0].Evidence = append([]TeachingClaimEvidence(nil), d.Slides[0].Evidence...)
	bad.Slides[0].Evidence[0].SourceHash = "old"
	v = worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	worksCall(t, s, "POST", route+"/edit", token, map[string]any{"revision": v["revision"], "result": bad}, 400)
}

func TestTeachingEvidenceDoesNotLeakHiddenAnswersOrMutateNotes(t *testing.T) {
	d := Deck{Slides: []Slide{{Bullets: []string{"求数值"}, Notes: "原始备注", Evidence: []TeachingClaimEvidence{{Claim: "隐藏答案是7", Quote: "隐藏来源答案7"}}}}}
	if strings.Contains(string(asJSON(visualReviewInput(d))), "隐藏") {
		t.Fatal("source claims leak teacher answers")
	}
	export := coursewareEvidenceForExport(d)
	if d.Slides[0].Notes != "原始备注" || !strings.Contains(export.Slides[0].Notes, "隐藏来源答案7") {
		t.Fatal("export mutated saved notes or lost quote")
	}
}

func TestTeachingEvidenceAnchorsResolveOnceAndInvalidateEdits(t *testing.T) {
	index := 0
	sources := []Source{{ID: "S1", Hash: "hash", Content: "教材条件：k≠0。", EvidenceKind: "extracted_text"}}
	d := Deck{Slides: []Slide{{Bullets: []string{"k≠0"}, Evidence: []TeachingClaimEvidence{{Field: "bullets", Index: &index, SourceID: "S1", SourceHash: "hash", Quote: "k≠0"}}}}}
	if err := coursewareSourceError(d, sources); err != nil {
		t.Fatal(err)
	}
	e := d.Slides[0].Evidence[0]
	if e.Claim != "k≠0" || e.Field != "" || e.Index != nil {
		t.Fatal("anchor was not bound to actual saved claim", e)
	}
	d.Slides[0].Bullets[0] = "k=0"
	if err := coursewareSourceError(d, sources); err == nil {
		t.Fatal("editing silently moved old support to a new claim")
	}
	// Recalled chunks of the same selected book cannot evade corrupt-page guards.
	in := TaskInput{MaterialID: "book", Curriculum: []Page{{2, "/G21/G22/G23"}}}
	merged := appendTeachingSources(in, []Source{{ID: "S1", DocumentID: "book", Content: "狔＝犽狓，犽≠０", Location: "第2页 · 段1"}})
	if merged[0].EvidenceKind != "unreliable_extraction" || merged[0].FilePage != 2 {
		t.Fatal("recalled textbook chunk bypassed page guard")
	}
}

func TestSelectedTextbookEvidenceRetainsVersionAndPage(t *testing.T) {
	var in TaskInput
	json.Unmarshal([]byte(`{"material_id":"book-a","textbook":"人教版 八下 2025教师标注","chapter":"教师核对的章节","curriculum":[{"page":7,"text":"实际抽取文字"}],"material_snapshot":{"document_id":"book-a","name":"教材.pdf","edition":"人教版 八下 2025教师标注","document_hash":"full-hash","chapter":"教师核对的章节"}}`), &in)
	sources := appendTeachingSources(in, nil)
	var raw []map[string]any
	json.Unmarshal(asJSON(sources), &raw)
	if len(raw) != 1 || raw[0]["file_page"] != float64(7) || raw[0]["evidence_kind"] != "extracted_page" || raw[0]["textbook"] == nil {
		t.Fatal("version/page snapshot missing", raw)
	}
	meta := raw[0]["textbook"].(map[string]any)
	if meta["document_hash"] != "full-hash" || meta["edition"] != in.Textbook {
		t.Fatal("version snapshot changed", meta)
	}
	in.Curriculum = []Page{{1, "目录\n第一章 函数 … 1\n第二章 方程 … 20"}}
	json.Unmarshal(asJSON(appendTeachingSources(in, nil)), &raw)
	if raw[0]["evidence_kind"] != "catalogue" {
		t.Fatal("directory passed as body", raw)
	}
	if textbookPageKind("/G21/G22/G23\ny=6x") != "unreliable_extraction" || textbookPageKind("狔＝犽狓，犽≠０") != "unreliable_extraction" {
		t.Fatal("corrupt formulas accepted as original evidence")
	}
	if textbookPageKind("一次函数 y=kx+b，k≠0。") != "extracted_page" {
		t.Fatal("ordinary original formula wrongly marked corrupt")
	}
}

func TestUnresolvedPDFGlyphWarningBlocksEvidence(t *testing.T) {
	text := "[PDF文字待核对：部分字形未识别，不能作为已核验的公式依据]\ny=6x"
	if textbookPageKind(text) != "unreliable_extraction" {
		t.Fatal("parser warning lost at source classification")
	}
	if textbookPageKind("[页边文字未提取]\ny=6/x") != "extracted_page" {
		t.Fatal("a skipped unreadable footer blocked the readable body")
	}
}

func TestActualTextbookExtractionIngestHTTP(t *testing.T) {
	path := os.Getenv("EDU_TEXTBOOK_PDF")
	if path == "" {
		t.Skip("requires original uploaded PDF and isolated database")
	}
	s := worksServer(t)
	ctx := context.Background()
	pages, err := s.extractFile(ctx, path, "实际人教九下.pdf")
	if err != nil || len(pages) != 122 {
		t.Fatalf("actual extraction: %v, %d pages", err, len(pages))
	}
	if !strings.Contains(pages[10].Text, "y=ax^2+bx+c") || !strings.Contains(pages[10].Text, "y=6/x") {
		t.Fatal("actual formulas not preserved through renderer HTTP")
	}
	chapters := materialChapters(pages)
	found := false
	for _, chapter := range chapters {
		if chapter.Start < 8 {
			t.Fatal("catalogue became a lesson range", chapter)
		}
		if chapter.Title == "26.1.2 反比例函数的图象和性质" {
			found = chapter.Start == 11 && chapter.End == 18
		}
	}
	if !found {
		t.Fatal("actual three-level lesson range missing", chapters)
	}
	token := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "other")
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	doc, err := s.saveDocument(ctx, a, "实际PDF隔离解析验收.pdf", "", pages)
	if err != nil {
		t.Fatal(err)
	}
	in := TaskInput{MaterialID: doc, MaterialStart: 11, MaterialEnd: 11}
	preview := worksObject(t, worksCall(t, s, "POST", "/api/courseware/materials/preview", token, in, 200))
	var saved []Page
	json.Unmarshal(asJSON(preview["pages"]), &saved)
	if len(saved) != 1 || saved[0] != pages[10] {
		t.Fatal("persisted PDF page differs from extracted original")
	}
	worksCall(t, s, "POST", "/api/courseware/materials/preview", other, in, 404)
	in.MaterialStart, in.MaterialEnd = 9, 9
	preview = worksObject(t, worksCall(t, s, "POST", "/api/courseware/materials/preview", token, in, 200))
	if len(preview["page_warnings"].([]any)) == 0 {
		t.Fatal("unresolved PDF warning disappeared at preview")
	}
	json.Unmarshal(asJSON(preview["pages"]), &saved)
	in.Curriculum = saved
	sources := appendTeachingSources(in, nil)
	if sources[0].EvidenceKind != "unreliable_extraction" {
		t.Fatal("bad extraction became verified evidence")
	}
	deck := Deck{Slides: []Slide{{Title: "不能凭损坏文字确认公式", Evidence: []TeachingClaimEvidence{{Claim: "不能凭损坏文字确认公式", SourceID: sources[0].ID, SourceHash: sources[0].Hash, Quote: saved[0].Text[:40]}}}}}
	if err := coursewareSourceError(deck, sources); err == nil {
		t.Fatal("unresolved original formula was accepted as evidence")
	}
	// Saved document points at a real isolated copy, not the production record.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	stored := s.cfg.Storage + "/original.pdf"
	if err = os.WriteFile(stored, data, 0600); err != nil {
		t.Fatal(err)
	}
	tx, err := s.tx(ctx, a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, "UPDATE documents SET upload_path=$1 WHERE id=$2", stored, doc)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	imageRoute := "/api/courseware/materials/" + doc + "/pages/10/image"
	image := worksObject(t, worksCall(t, s, "GET", imageRoute, token, nil, 200))
	if image["file_page"] != float64(10) || !strings.HasPrefix(image["data_url"].(string), "data:image/png;base64,") || image["original_hash"] == "" {
		t.Fatal("original page pixels or identity missing")
	}
	worksCall(t, s, "GET", imageRoute, other, nil, 404)
	worksCall(t, s, "GET", imageRoute, "", nil, 401)
	worksCall(t, s, "GET", "/api/courseware/materials/"+doc+"/pages/123/image", token, nil, 400)
	// Exercise the real background parser against this isolated original copy.
	worksCall(t, s, "POST", "/api/documents/"+doc+"/reparse", token, nil, 202)
	if err := s.ingestOne(ctx); err != nil {
		t.Fatal("real original-file reparse failed", err)
	}
	in.MaterialStart, in.MaterialEnd = 11, 11
	preview = worksObject(t, worksCall(t, s, "POST", "/api/courseware/materials/preview", token, in, 200))
	json.Unmarshal(asJSON(preview["pages"]), &saved)
	if len(saved) != 1 || saved[0] != pages[10] {
		t.Fatal("reparse changed identity or failed to replace original extraction")
	}
}
