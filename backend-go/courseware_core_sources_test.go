package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestCoreTeachingSourcesArePublicSnapshotsHTTP(t *testing.T) {
	s := worksServer(t)
	token := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "other")
	s.cfg.Mode = "live"
	for _, topic := range []string{"一次函数", "二次函数", "浮力", "串并联电路"} {
		for _, login := range []string{token, other} {
			v := worksObject(t, worksCall(t, s, "POST", "/api/retrieval/test", login, map[string]any{"query": topic}, 200))
			var sources []Source
			json.Unmarshal(asJSON(v["sources"]), &sources)
			found := false
			for _, src := range sources {
				if src.Kind != "core_excerpt" {
					continue
				}
				found = true
				if src.Hash == "" || src.RetrievedAt == "" || src.Location == "" || !strings.HasPrefix(src.URL, "https://openstax.org/books/") || src.DocumentID != "" || src.Textbook != nil || !strings.Contains(src.Reliability, "非人教") {
					t.Fatal("public excerpt lacks identity or pretends to be a private/PEP textbook", src)
				}
			}
			if !found {
				t.Fatal("missing durable verified primary excerpt", topic)
			}
		}
	}
	worksCall(t, s, "POST", "/api/retrieval/test", "", map[string]any{"query": "浮力"}, 401)
	for _, flags := range [][2]bool{{false, false}, {true, false}} {
		query := "语文古诗"
		if !flags[0] {
			query = "浮力"
		}
		sources, err := s.retrieve(context.Background(), Actor{TenantID: 1, MemberID: 1}, query, flags[0], flags[1])
		if err != nil {
			t.Fatal(err)
		}
		for _, src := range sources {
			if src.Kind == "core_excerpt" {
				t.Fatal("core excerpt ignored subject/knowledge preference")
			}
		}
	}
}
