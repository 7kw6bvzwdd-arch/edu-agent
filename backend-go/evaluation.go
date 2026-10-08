package main

import (
	"net/http"
	"strings"
	"time"
)

var evaluationCases = []struct{ Query, Document string }{
	{"浮力的大小与排开液体的关系", "验收-浮力.md"},
	{"直角三角形已知两边求第三边", "验收-勾股定理.md"},
	{"一次函数斜率与增减性", "验收-一次函数.md"},
	{"电压电流电阻的关系", "验收-欧姆定律.md"},
	{"入射角和反射角如何测量", "验收-光的反射.md"},
}

func (s *Server) evaluateRetrieval(w http.ResponseWriter, r *http.Request, a Actor) error {
	start := time.Now()
	results := []map[string]any{}
	hits := 0
	rr := 0.
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	docs, e := queryMaps(r.Context(), tx, "SELECT id,name FROM documents WHERE status='ready'")
	tx.Rollback(r.Context())
	if e != nil {
		return e
	}
	byName := map[string]bool{}
	for _, d := range docs {
		byName[d["name"].(string)] = true
	}
	for _, c := range evaluationCases {
		if !byName[c.Document] {
			return fail(400, "请先导入仓库fixtures目录中的五份验收文档；缺少 "+c.Document)
		}
	}
	for _, c := range evaluationCases {
		src, e := s.retrieve(r.Context(), a, c.Query, true, false)
		if e != nil {
			return e
		}
		rank := 0
		for i, x := range src {
			if strings.EqualFold(x.Title, c.Document) {
				rank = i + 1
				break
			}
		}
		if rank > 0 {
			hits++
			rr += 1 / float64(rank)
		}
		results = append(results, map[string]any{"query": c.Query, "relevant_document": c.Document, "rank": rank, "sources": src})
	}
	metrics := map[string]any{"recall_at_8": float64(hits) / 5, "hit_rate_at_8": float64(hits) / 5, "mrr": rr / 5, "queries": 5, "latency_ms": time.Since(start).Milliseconds(), "dataset": "junior-science-v1", "note": "每条query标注1份相关文档，当前集合中Recall与HitRate相同"}
	tx, e = s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	id := newID()
	_, e = tx.Exec(r.Context(), "INSERT INTO eval_runs(id,tenant_id,member_id,config,metrics,results,status) VALUES($1,$2,$3,$4,$5,$6,'succeeded')", id, a.TenantID, a.MemberID, asJSON(map[string]any{"mode": s.cfg.Mode, "embedding_model": s.cfg.EmbeddingModel, "dataset": "junior-science-v1"}), asJSON(metrics), asJSON(results))
	if e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	respond(w, 200, map[string]any{"id": id, "metrics": metrics, "results": results})
	return nil
}
