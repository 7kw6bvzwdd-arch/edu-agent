package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestRenderValidationErrorIsActionableAndPrivate(t *testing.T) {
	code := "formula_invalid"
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, 422, map[string]any{"detail": map[string]any{"code": code, "page": 4, "field": "formula", "input": "private lesson and secret-token", "message": "secret-token"}})
	}))
	defer provider.Close()
	s := &Server{cfg: Config{RenderURL: provider.URL}, http: provider.Client()}
	_, err := s.renderPPTX(context.Background(), Deck{})
	var apiErr APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 422 || !strings.Contains(err.Error(), "第4页") || !strings.Contains(err.Error(), "公式") {
		t.Fatal("missing useful page error", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatal("input leaked")
	}
	code = "unknown-secret-token"
	_, err = s.renderPPTX(context.Background(), Deck{})
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("unknown response leaked", err)
	}
	code = "formula_invalid"
	_, err = s.postBytes(context.Background(), provider.URL+"/chat/completions", map[string]string{}, "secret-token")
	if err == nil || strings.Contains(err.Error(), "第4页") || strings.Contains(err.Error(), "secret") {
		t.Fatal("external error interpreted as local render error", err)
	}
}

func TestRenderFailureHTTPPreservesCreditsAndSavedRevision(t *testing.T) {
	s := worksServer(t)
	owner := worksLogin(t, s, "teacher")
	for _, invalidAtConfirm := range []string{"formula", "chart", ""} {
		before := worksObject(t, worksCall(t, s, "GET", "/api/billing", owner, nil, 200))
		id := worksObject(t, worksCall(t, s, "POST", "/api/tasks", owner, TaskInput{Type: "courseware", Topic: "勾股定理", Pages: 8, UseKB: true, IdempotencyKey: newID()}, 202))["id"].(string)
		if err := s.workOne(context.Background()); err != nil {
			t.Fatal(err)
		}
		path := "/api/tasks/" + id
		v := worksObject(t, worksCall(t, s, "GET", path, owner, nil, 200))
		d := v["result"].(map[string]any)
		if invalidAtConfirm != "" {
			slide := d["slides"].([]any)[3].(map[string]any)
			if invalidAtConfirm == "formula" {
				slide["formula"] = `x \unsupportedSecretCommand y`
			} else {
				slide["chart"] = map[string]any{"labels": []string{"a", "b"}, "values": []int{1}, "title": "secret-token"}
			}
			w := worksCall(t, s, "POST", path+"/confirm", owner, map[string]any{"result": d, "revision": v["revision"], "accept_review": true}, 422)
			current := worksObject(t, worksCall(t, s, "GET", path, owner, nil, 200))
			if current["status"] != "awaiting_confirmation" || current["revision"] != v["revision"] || !strings.Contains(w.Body.String(), "第4页") || strings.Contains(w.Body.String(), "secret-token") || strings.Contains(w.Body.String(), "SecretCommand") {
				t.Fatal("invalid edit must stay correctable without leaking formula", current["status"])
			}
			worksCall(t, s, "POST", path+"/cancel", owner, nil, 200)
			after := worksObject(t, worksCall(t, s, "GET", "/api/billing", owner, nil, 200))
			if before["balance"] != after["balance"] || before["reserved"] != after["reserved"] {
				t.Fatal("rejected confirmation charged credits")
			}
			continue
		}
		worksCall(t, s, "POST", path+"/confirm", owner, map[string]any{"result": d, "revision": v["revision"], "accept_review": true}, 202)
		if err := s.workOne(context.Background()); err != nil {
			t.Fatal(err)
		}
		v = worksObject(t, worksCall(t, s, "GET", path, owner, nil, 200))
		if v["status"] != "succeeded" {
			t.Fatal("valid render failed", v["error"])
		}
		oldFile := worksCall(t, s, "GET", path+"/download/pptx", owner, nil, 200).Body.String()
		ledger := worksCall(t, s, "GET", "/api/billing", owner, nil, 200).Body.String()
		d = v["result"].(map[string]any)
		d["slides"].([]any)[3].(map[string]any)["formula"] = `x \unsupportedSecretCommand y`
		w := worksCall(t, s, "POST", path+"/edit", owner, map[string]any{"result": d, "revision": v["revision"]}, 422)
		if !strings.Contains(w.Body.String(), "第4页") {
			t.Fatal("edit error hidden", w.Body.String())
		}
		current := worksObject(t, worksCall(t, s, "GET", path, owner, nil, 200))
		if current["revision"] != v["revision"] || worksCall(t, s, "GET", path+"/download/pptx", owner, nil, 200).Body.String() != oldFile || worksCall(t, s, "GET", "/api/billing", owner, nil, 200).Body.String() != ledger {
			t.Fatal("rejected edit changed saved content or billing")
		}
	}
}

func TestTextProviderContractAndFabricatedCitation(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	invalidCitation := false
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/v3/chat/completions" {
			t.Errorf("wrong Ark text path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing authorization")
		}
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		if in["model"] != "test-text-model" {
			t.Error("wrong model")
		}
		d := sampleDeck(TaskInput{Topic: "浮力", Theme: "forest", Pages: 8})
		if invalidCitation {
			d.Slides[0].Notes = "引用不存在[S999]"
		}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(asJSON(d))}}}, "usage": map[string]int{"prompt_tokens": 120, "completion_tokens": 240}})
	}))
	defer provider.Close()
	s, e := NewServer(context.Background(), Config{DatabaseURL: os.Getenv("TEST_DATABASE_URL"), RuntimeURL: os.Getenv("TEST_RUNTIME_DATABASE_URL"), Secret: "test-secret-at-least-thirty-two-bytes", SeedPassword: "test-pass-123", Mode: "live", Storage: t.TempDir(), RenderURL: os.Getenv("TEST_RENDER_URL"), LLMURL: provider.URL + "/api/v3", LLMKey: "test-key", LLMModel: "test-text-model"})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	tx, e := s.tx(context.Background(), 1)
	if e != nil {
		t.Fatal(e)
	}
	var member int64
	e = tx.QueryRow(context.Background(), "SELECT m.id FROM members m JOIN users u ON u.id=m.user_id WHERE u.username='teacher'").Scan(&member)
	tx.Rollback(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	a := Actor{TenantID: 1, MemberID: member}
	d, e := s.generate(context.Background(), a, "contract-check", TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "forest"}, nil)
	if e != nil || len(d.Slides) != 8 || !d.NeedsReview {
		t.Fatal("provider contract", e)
	}
	invalidCitation = true
	if _, e = s.generate(context.Background(), a, "citation-check", TaskInput{Type: "courseware", Topic: "浮力", Pages: 8, Theme: "forest"}, nil); e == nil {
		t.Fatal("fabricated citation accepted")
	}
	if calls != 3 {
		t.Fatal("unexpected provider calls", calls)
	}
}
