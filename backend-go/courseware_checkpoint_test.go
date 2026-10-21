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

func TestCoursewareCheckpointRetryHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	in := TaskInput{Type: "courseware", Topic: "浮力", Pages: 12, Theme: "classroom", UseKB: true, PreviewFirst: true, IdempotencyKey: newID()}
	failing := true
	var requested []int
	ranges := regexp.MustCompile(`本批只返回第(\d+)至第(\d+)页`)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Messages []struct{ Content string } }
		json.NewDecoder(r.Body).Decode(&req)
		prompt := req.Messages[len(req.Messages)-1].Content
		m := ranges.FindString(prompt)
		var start, end int
		fmt.Sscanf(m, "本批只返回第%d至第%d页", &start, &end)
		if start < 1 || end > 12 || end < start {
			t.Errorf("invalid range %d-%d", start, end)
			w.WriteHeader(400)
			return
		}
		requested = append(requested, start)
		if failing && start >= 7 {
			w.WriteHeader(401)
			return
		}
		d := directHTTPFixtureDeck(sampleDeck(in))
		d.Slides = d.Slides[start-1 : end]
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(asJSON(d))}}}})
	}))
	defer provider.Close()
	s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "test", "test"
	configureDirectHTTPFixture(t, s, in, provider.URL)
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	route := "/api/tasks/" + id
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	preview := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	for _, entry := range preview["slide_reviews"].([]any) {
		m := entry.(map[string]any)
		worksCall(t, s, "POST", route+"/slides/"+m["id"].(string)+"/review", token, map[string]any{"revision": preview["revision"], "hash": m["hash"], "approved": true}, 200)
	}
	worksCall(t, s, "POST", route+"/expand", token, map[string]any{"revision": preview["revision"]}, 202)
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	failed := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
	slides := failed["result"].(map[string]any)["slides"].([]any)
	if failed["status"] != "failed" || len(slides) != 6 {
		t.Fatalf("completed pages lost: status=%v saved=%d, want 6", failed["status"], len(slides))
	}
	if !reflect.DeepEqual(preview["result"].(map[string]any)["slides"], slides[:3]) {
		t.Fatal("approved pages changed")
	}
	failing, requested = false, nil
	other := worksLogin(t, s, "other")
	worksCall(t, s, "POST", route+"/retry", other, nil, 404)
	child := worksObject(t, worksCall(t, s, "POST", route+"/retry", token, nil, 202))["id"].(string)
	childRoute := "/api/tasks/" + child
	if again := worksObject(t, worksCall(t, s, "POST", route+"/retry", token, nil, 202))["id"]; again != child {
		t.Fatal("duplicate continuation")
	}
	if err := s.workOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	completed := worksObject(t, worksCall(t, s, "GET", childRoute, token, nil, 200))
	if completed["status"] != "awaiting_confirmation" {
		t.Fatalf("resume failed: %v %v", completed["status"], completed["error"])
	}
	if len(requested) == 0 || requested[0] != 7 {
		t.Fatal("retry restarted completed pages", requested)
	}
	if !reflect.DeepEqual(slides, completed["result"].(map[string]any)["slides"].([]any)[:6]) {
		t.Fatal("saved pages changed during retry")
	}
	for _, review := range completed["slide_reviews"].([]any)[:3] {
		if !review.(map[string]any)["approved"].(bool) {
			t.Fatal("lost teacher approval")
		}
	}
	artifact := worksCall(t, s, "GET", childRoute+"/preview-pptx", token, nil, 200).Body.Bytes()
	if len(artifact) < 1000 || string(artifact[:2]) != "PK" {
		t.Fatal("missing real PPTX")
	}
	worksCall(t, s, "POST", childRoute+"/cancel", token, nil, 200)
}

func TestCoursewareLocalRepairHTTP(t *testing.T) {
	for _, mode := range []string{"quality", "renderer", "length-resume", "first-page", "last-page", "render-outage"} {
		t.Run(mode, func(t *testing.T) {
			s := worksServer(t)
			token := worksLogin(t, s, "teacher")
			in := TaskInput{Type: "courseware", Topic: "浮力", Pages: 12, Theme: "classroom", UseKB: true, PreviewFirst: true, IdempotencyKey: newID()}
			id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
			route := "/api/tasks/" + id
			d := directHTTPFixtureDeck(sampleDeck(in))
			assignSlideIDs(&d)
			page := 7
			if mode == "first-page" {
				page = 1
			}
			if mode == "last-page" {
				page = 12
			}
			for i := range d.Slides {
				d.Slides[i].Notes += fmt.Sprintf(" 唯一内容标记PAGE%02dEND", i+1)
			}
			good := d.Slides[page-1]
			good.Bullets = append([]string{}, good.Bullets...)
			if mode == "renderer" {
				d.Slides[page-1].HTML += `<p>未声明可编辑的文字</p>`
			} else if page > 1 {
				d.Slides[page-1].Title = d.Slides[page-2].Title
			} else {
				d.Slides[0].Title = ""
			}
			// For boundary and provider failures, explicitly persist the target.
			d.Generation = &CoursewareCheckpoint{Version: coursewarePageRepairVersion, Phase: "expand"}
			if mode == "length-resume" || mode == "first-page" || mode == "last-page" {
				d.Generation.RepairPage = page
				d.Generation.Issue = "当前页内容不完整"
			}
			before := append([]Slide{}, d.Slides...)
			if mode == "render-outage" {
				d.Slides = d.Slides[:6]
				d.Generation.RepairPage = 7
				d.Generation.Issue = ""
			}
			tx, err := s.tx(context.Background(), 1)
			if err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(context.Background(), "UPDATE tasks SET status='expand_pending',stage='expand_pending',result=$1,mode='live',input=jsonb_set(jsonb_set(input,'{detail}',to_jsonb($3::text)),'{pages}',to_jsonb($4::int)) WHERE id=$2", asJSON(d), id, "", in.Pages)
			if err != nil {
				t.Fatal(err)
			}
			for i, sl := range d.Slides {
				if i+1 == page {
					continue
				}
				_, err = tx.Exec(context.Background(), "INSERT INTO courseware_feedback(tenant_id,task_id,slide_id,content_hash,approved) VALUES(1,$1,$2,$3,true)", id, sl.ID, slideContentHash(d, sl))
				if err != nil {
					t.Fatal(err)
				}
			}
			if err = tx.Commit(context.Background()); err != nil {
				t.Fatal(err)
			}
			calls, truncate := 0, mode == "length-resume"
			outage := mode == "render-outage"
			realRender := s.cfg.RenderURL
			var renderer *httptest.Server
			if outage {
				renderer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
				defer renderer.Close()
				s.cfg.RenderURL = renderer.URL
			}
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var req struct{ Messages []struct{ Content string } }
				json.NewDecoder(r.Body).Decode(&req)
				prompt := req.Messages[len(req.Messages)-1].Content
				requestedPage := page
				if mode == "render-outage" {
					fmt.Sscanf(regexp.MustCompile(`本批只返回第\d+至第\d+页`).FindString(prompt), "本批只返回第%d至第%d页", &requestedPage, new(int))
				}
				if mode == "render-outage" && requestedPage < 7 {
					t.Error("outage regenerated a completed page")
				}
				if !strings.Contains(prompt, fmt.Sprintf("本批只返回第%d至第%d页", requestedPage, requestedPage)) {
					t.Error("nonlocal repair requested")
				}
				for i := 1; i <= 12 && mode != "render-outage"; i++ {
					present := strings.Contains(prompt, fmt.Sprintf("PAGE%02dEND", i))
					if present != (i >= page-1 && i <= page+1) {
						t.Errorf("context leaks or omits page %d", i)
					}
				}
				if !strings.Contains(prompt, coursewareTeachingSkill) || !strings.Contains(prompt, coursewarePageRepairSkill) {
					t.Error("project skill not loaded into actual request")
				}
				finish, raw := "stop", ""
				if truncate {
					finish = "length"
				} else {
					next := d
					next.Generation = nil
					next.Slides = []Slide{good}
					if mode == "render-outage" {
						next.Slides = []Slide{directHTTPFixtureDeck(sampleDeck(in)).Slides[requestedPage-1]}
					}
					raw = string(asJSON(directHTTPFixtureDeck(next)))
				}
				respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": finish, "message": map[string]string{"content": raw}}}})
			}))
			defer provider.Close()
			s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "test", "test"
			configureDirectHTTPFixture(t, s, in, provider.URL)
			if err = s.workOne(context.Background()); err != nil {
				t.Fatal(err)
			}
			if truncate {
				// Advance queue delays only; automatic retries retain their persisted counters.
				for round := 0; round < 8; round++ {
					state := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
					if state["status"] == "failed" {
						break
					}
					makeAutomaticRetryReady(t, s, id)
					if err = s.workOne(context.Background()); err != nil {
						t.Fatal(err)
					}
				}

				failed := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
				if failed["status"] != "failed" || failed["charge_state"] != "released" || calls != 6 {
					t.Fatal("unbounded length repair", calls, failed["status"])
				}
				truncate = false
				worksCall(t, s, "POST", route+"/retry", token, nil, 202)
				if err = s.workOne(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			if outage {
				failed := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
				if failed["status"] != "expand_pending" || failed["charge_state"] != "reserved" || len(failed["result"].(map[string]any)["slides"].([]any)) != 7 || calls != 1 {
					t.Fatal("renderer outage discarded generated page", failed["status"], failed["error"], calls)
				}
				s.cfg.RenderURL = realRender
				makeAutomaticRetryReady(t, s, id)
				if err = s.workOne(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			done := worksObject(t, worksCall(t, s, "GET", route, token, nil, 200))
			if done["status"] != "awaiting_confirmation" {
				t.Fatal("local repair failed", done["status"], done["error"])
			}
			var result Deck
			json.Unmarshal(asJSON(done["result"]), &result)
			if outage {
				before = before[:6]
			}
			for i, sl := range before {
				if i+1 != page && string(asJSON(sl)) != string(asJSON(result.Slides[i])) {
					t.Errorf("unrelated page %d changed", i+1)
				}
			}
			for i, review := range done["slide_reviews"].([]any) {
				m := review.(map[string]any)
				if m["approved"].(bool) != (i+1 != page && (!outage || i < 6)) {
					t.Errorf("incorrect approval on page %d", i+1)
				}
			}
			want := 1
			if mode == "length-resume" {
				want = 7
			}
			if outage {
				want = 6
			}
			if calls != want {
				t.Errorf("model calls=%d, want=%d", calls, want)
			}
			worksCall(t, s, "POST", route+"/cancel", token, nil, 200)
		})
	}
}

func TestCheckpointLessonTracksOnlyCurrentPages(t *testing.T) {
	d := sampleDeck(TaskInput{Topic: "浮力", Pages: 12, Theme: "classroom"})
	d.Lesson = checkpointLesson(d)
	d.Slides[6].Notes = "局部修改后的新讲解"
	d.Lesson = checkpointLesson(d)
	if strings.Count(d.Lesson, coursewareLessonPages) != 1 || !strings.Contains(d.Lesson, d.Slides[11].Notes) || !strings.Contains(d.Lesson, "局部修改后的新讲解") {
		t.Fatal("lesson omitted later pages or duplicated a prior plan")
	}
}

func TestPageCitationIssueStaysOnItsPage(t *testing.T) {
	d := sampleDeck(TaskInput{Topic: "浮力", Pages: 12, Theme: "classroom"})
	d.Slides[6].Notes += " [S404]"
	d.Lesson = checkpointLesson(d)
	if err := coursewarePageIssue(d, 1, nil); err != nil {
		t.Fatal("another page's bad citation was assigned to page 1", err)
	}
	if err := coursewarePageIssue(d, 7, nil); err == nil {
		t.Fatal("invalid citation on page 7 was accepted")
	}
}
