package main

import (
	"context"
	"encoding/json"
	"testing"
)

func TestWorkspaceHistoryIsolationClearAndLateSaveHTTP(t *testing.T) {
	s := worksServer(t)
	owner, other, admin := worksLogin(t, s, "teacher"), worksLogin(t, s, "other"), worksLogin(t, s, "admin")
	route := "/api/workspace/conversations"
	id := newID()
	path := route + "/" + id
	state := map[string]any{"version": 1, "session": map[string]any{"messages": []any{map[string]any{"role": "user", "content": "讲解光的折射"}}, "contextMemory": map[string]any{"text": "保留观察任务", "through": 0}}, "form": map[string]any{"topic": "光的折射"}}
	body := map[string]any{"revision": 0, "state": state}
	before := worksCall(t, s, "GET", "/api/billing", owner, nil, 200).Body.String()
	worksCall(t, s, "GET", route, "", nil, 401)
	// Clear must also work when the first save never reached the database.
	unsaved := route + "/" + newID()
	worksCall(t, s, "DELETE", unsaved, owner, nil, 200)
	worksCall(t, s, "PUT", unsaved, owner, body, 409)
	worksCall(t, s, "PUT", path, owner, map[string]any{"revision": -1, "state": state}, 400)
	worksCall(t, s, "PUT", path, owner, body, 200)
	worksCall(t, s, "PUT", path, owner, body, 200) // Lost response may safely retry the identical save.
	for _, token := range []string{other, admin} {
		worksCall(t, s, "GET", path, token, nil, 404)
		worksCall(t, s, "PUT", path, token, body, 404)
		worksCall(t, s, "DELETE", path, token, nil, 404)
		if worksCall(t, s, "GET", route, token, nil, 200).Body.String() != "[]\n" {
			t.Fatal("foreign history visible")
		}
	}
	saved := worksObject(t, worksCall(t, s, "GET", path, owner, nil, 200))
	if saved["revision"] != float64(1) {
		t.Fatal(saved)
	}
	var got map[string]any
	json.Unmarshal(asJSON(saved["state"]), &got)
	if got["session"].(map[string]any)["contextMemory"].(map[string]any)["text"] != "保留观察任务" {
		t.Fatal("memory lost")
	}
	state["form"].(map[string]any)["topic"] = "光的反射"
	worksCall(t, s, "PUT", path, owner, body, 409)
	body["revision"] = 1
	worksCall(t, s, "PUT", path, owner, body, 200)
	worksCall(t, s, "DELETE", path, owner, nil, 200)
	worksCall(t, s, "GET", path, owner, nil, 404)
	worksCall(t, s, "PUT", path, owner, body, 409)
	body["revision"] = 0
	worksCall(t, s, "PUT", path, owner, body, 409)
	second := route + "/" + newID()
	foreign := route + "/" + newID()
	worksCall(t, s, "PUT", foreign, other, body, 200)
	worksCall(t, s, "PUT", second, owner, body, 200)
	worksCall(t, s, "DELETE", route, owner, nil, 200)
	worksCall(t, s, "GET", foreign, other, nil, 200)
	if worksCall(t, s, "GET", route, owner, nil, 200).Body.String() != "[]\n" {
		t.Fatal("clear all failed")
	}
	worksCall(t, s, "PUT", second, owner, body, 409)
	var a Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", owner, nil, 200).Body.Bytes(), &a)
	tx, err := s.tx(context.Background(), a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var count int
	if err = tx.QueryRow(context.Background(), "SELECT count(*) FROM workspace_conversations WHERE member_id=$1 AND state<>'{}'::jsonb", a.MemberID).Scan(&count); err != nil || count != 0 {
		t.Fatal("cleared message data retained", err, count)
	}
	if worksCall(t, s, "GET", "/api/billing", owner, nil, 200).Body.String() != before {
		t.Fatal("history changed billing")
	}
}
