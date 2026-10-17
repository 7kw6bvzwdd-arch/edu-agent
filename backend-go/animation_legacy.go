package main

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
)

const animationRebuildMessage = "旧版动画生成已移除，请使用HTML动画；历史作品仍可查看和下载，不重新生成或补扣积分。"

func animationUnavailable() error { return fail(503, animationRebuildMessage) }

// Stored legacy metadata is read-only; it never drives generation or settlement.
type QualityFrame struct {
	Index  int     `json:"index"`
	Second float64 `json:"second"`
}
type AnimationQuality struct {
	Frames     []QualityFrame `json:"frames,omitempty"`
	Version    string         `json:"version"`
	Status     string         `json:"status"`
	Checks     []string       `json:"checks"`
	Visual     any            `json:"visual"`
	AcceptedBy int64          `json:"accepted_by,omitempty"`
}

func (s *Server) acceptAnimation(w http.ResponseWriter, r *http.Request, a Actor) error {
	tx, err := s.tx(r.Context(), a.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	var id string
	if err = tx.QueryRow(r.Context(), "SELECT id FROM tasks WHERE id=$1 AND member_id=$2 AND type='animation' AND deleted_at IS NULL", r.PathValue("id"), a.MemberID).Scan(&id); err != nil {
		return err
	}
	return fail(410, "旧版动画仅供查看和下载，不能重新验收扣费；HTML动画请使用当前工作台。")
}

func (s *Server) animationFrame(w http.ResponseWriter, r *http.Request, a Actor) error {
	i, err := strconv.Atoi(r.PathValue("frame"))
	if err != nil || i < 0 || i >= 36 {
		return fail(404, "关键帧不存在")
	}
	tx, err := s.tx(r.Context(), a.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	var d Deck
	err = tx.QueryRow(r.Context(), "SELECT result FROM tasks WHERE id=$1 AND member_id=$2 AND type='animation' AND deleted_at IS NULL AND status IN ('awaiting_acceptance','succeeded')", r.PathValue("id"), a.MemberID).Scan(&d)
	if err != nil {
		return err
	}
	if d.AnimationQuality == nil || i >= len(d.AnimationQuality.Frames) {
		return fail(404, "关键帧不存在")
	}
	w.Header().Set("Cache-Control", "private, no-store")
	return s.serveStoredFile(w, r, filepath.Join(s.cfg.Storage, "animation-quality", r.PathValue("id"), fmt.Sprintf("frame-%d.jpg", i)))
}
