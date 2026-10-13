package main

import (
	"context"
	"net/http"
	"time"
)

func (s *Server) coursewareFormula(w http.ResponseWriter, r *http.Request, a Actor) error {
	var in struct {
		Formula string `json:"formula"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if len([]rune(in.Formula)) > 250 || in.Formula == "" {
		return fail(400, "请填写250字以内的公式")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	raw, err := s.postBytes(ctx, s.cfg.RenderURL+"/render/formula", in, "")
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(raw)
	return nil
}
