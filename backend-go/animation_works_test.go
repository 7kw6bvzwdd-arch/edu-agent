package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTMLAnimationSavedWorksHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	admin, other := worksLogin(t, s, "admin"), worksLogin(t, s, "other")
	var actor Actor
	if err := json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &actor); err != nil {
		t.Fatal(err)
	}
	marker := newID()
	// Stored snapshots verify retention and file access, not AI/media generation.
	cases := []struct {
		status   string
		delivery bool
	}{
		{"anim_plan", false}, {"anim_representative", false}, {"succeeded", true},
		{"anim_edit_running", true}, {"anim_edit_acceptance", true}, {"failed", true}, {"cancelled", true}, {"failed", false},
	}
	ids := make([]string, len(cases))
	for i, c := range cases {
		ids[i] = worksFixture(t, s, token, "animation", c.status, marker, TaskInput{AnimationEngine: "html"})
		html := ""
		charge := "unreserved"
		if c.delivery {
			html = filepath.Join(s.cfg.Storage, ids[i]+".html")
			if err := os.WriteFile(html, []byte("<!doctype html><title>saved animation fixture</title>"), 0600); err != nil {
				t.Fatal(err)
			}
			charge = "settled"
		}
		tx, err := s.tx(context.Background(), actor.TenantID)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(context.Background(), "UPDATE tasks SET html_path=$1,charge_state=$2,animation=$3 WHERE id=$4", html, charge, asJSON(map[string]any{"script": loadLinearKFixture(t)}), ids[i])
		if err != nil {
			tx.Rollback(context.Background())
			t.Fatal(err)
		}
		if err = tx.Commit(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	query := "/api/tasks?type=animation&q=" + marker
	rows := func(view, who string) []map[string]any {
		t.Helper()
		var result []map[string]any
		if err := json.Unmarshal(worksCall(t, s, "GET", query+"&view="+view, who, nil, 200).Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	before := worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String()
	worksCall(t, s, "GET", query+"&view=works", "", nil, 401)
	saved := rows("works", token)
	if len(saved) != len(cases)-1 {
		t.Fatalf("current works lost saved animations after edit interruption: got %d, want %d", len(saved), len(cases)-1)
	}
	for _, row := range saved {
		if strings.Contains(string(asJSON(row)), s.cfg.Storage) {
			t.Fatal("collection leaked server storage path")
		}
		id := row["id"].(string)
		index := -1
		for i, want := range ids {
			if want == id {
				index = i
				break
			}
		}
		if index < 0 || row["has_delivery"] != cases[index].delivery {
			t.Fatal("incorrect delivery indicator", row)
		}
		worksCall(t, s, "GET", "/api/tasks/"+id+"/animation", token, nil, 200)
		if cases[index].delivery {
			worksCall(t, s, "GET", "/api/tasks/"+id+"/download/html", token, nil, 200)
		}
	}
	if got := rows("interrupted", token); len(got) != 1 || got[0]["id"] != ids[len(ids)-1] {
		t.Fatal("only unfinished failed animation belongs in interrupted", got)
	}
	if len(rows("history", token)) != len(cases) {
		t.Fatal("history lost animation records")
	}
	for _, who := range []string{admin, other} {
		for _, view := range []string{"works", "interrupted", "history"} {
			if len(rows(view, who)) != 0 {
				t.Fatal("animation collection crossed membership boundary")
			}
		}
		worksCall(t, s, "GET", "/api/tasks/"+ids[5]+"/animation", who, nil, 404)
		worksCall(t, s, "GET", "/api/tasks/"+ids[5]+"/download/html", who, nil, 404)
	}
	worksCall(t, s, "DELETE", "/api/tasks/"+ids[5], token, nil, 200)
	worksCall(t, s, "GET", "/api/tasks/"+ids[5]+"/download/html", token, nil, 404)
	if len(rows("works", token)) != len(cases)-2 {
		t.Fatal("explicit deletion did not remove saved work")
	}
	if worksCall(t, s, "GET", "/api/billing", token, nil, 200).Body.String() != before {
		t.Fatal("viewing or deleting saved animations changed billing")
	}
}
