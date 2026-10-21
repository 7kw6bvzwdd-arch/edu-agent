package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRepresentativePageRequestUsesTeachingPosition(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	in := TaskInput{Type: "courseware", Topic: "浮力", Pages: 10, Theme: "ocean", UseKB: true, PreviewFirst: true, IdempotencyKey: newID()}
	id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", token, in, 202))["id"].(string)
	defer worksCall(t, s, "POST", "/api/tasks/"+id+"/cancel", token, nil, 200)
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &a)
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(context.Background(), "UPDATE tasks SET status='researching',stage='researching' WHERE id=$1", id)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	d := sampleDeck(in)
	d.Plan = &TeachingPlan{Summary: "课堂规划"}
	for i, sl := range d.Slides {
		d.Plan.Pages = append(d.Plan.Pages, TeachingPage{Position: i + 1, Goal: sl.Title, Task: sl.Layout, Minutes: 4, Preview: i == 1 || i == 3 || i == 4})
	}
	candidate := d
	candidate.Slides = d.Slides[4:5]
	d.Slides = []Slide{d.Slides[1], d.Slides[3]}
	d.Slides[0].Order = 2
	d.Slides[1].Order = 4
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req struct{ Messages []struct{ Content string } }
		json.NewDecoder(r.Body).Decode(&req)
		prompt := req.Messages[len(req.Messages)-1].Content
		if !strings.Contains(prompt, "本批只返回第5至第5页") || strings.Contains(prompt, "本批只返回第3至第3页") {
			t.Error("preview index conflicts with requested teaching position")
		}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(asJSON(directHTTPFixtureDeck(candidate)))}}}})
	}))
	defer provider.Close()
	s.cfg.Mode, s.cfg.LLMURL, s.cfg.LLMKey, s.cfg.LLMModel = "live", provider.URL, "test", "test"
	sl, _, _, err := s.coursewareSinglePage(context.Background(), a, id, in, nil, d, 3, "", "preview")
	if err != nil || sl.Order != 5 || calls != 1 {
		t.Fatalf("representative page request failed: order %d calls %d error %v", sl.Order, calls, err)
	}
}
