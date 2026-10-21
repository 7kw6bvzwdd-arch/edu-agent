package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"image"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type DocumentImage struct {
	Page    int    `json:"page"`
	Caption string `json:"caption"`
	AssetID string `json:"asset_id,omitempty"`
	DataURL string `json:"data_url,omitempty"`
}
type DocumentExtraction struct {
	Pages         []Page          `json:"pages"`
	Images        []DocumentImage `json:"images"`
	ImageWarnings []string        `json:"image_warnings"`
}

func (s *Server) saveDocumentImages(ctx context.Context, a Actor, id, name string, extracted DocumentExtraction) error {
	if len(extracted.Images) > 24 || len(extracted.ImageWarnings) > 12 {
		return fmt.Errorf("资料图片提取结果超过上限")
	}
	manifest := []DocumentImage{}
	total := 0
	for _, figure := range extracted.Images {
		if figure.Page < 1 || figure.Page > len(extracted.Pages) || len([]rune(figure.Caption)) > 600 || !strings.HasPrefix(figure.DataURL, "data:image/png;base64,") {
			return fmt.Errorf("资料图片来源无效")
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(figure.DataURL, "data:image/png;base64,"))
		total += len(raw)
		if err != nil || len(raw) > 8<<20 || total > 12<<20 {
			return fmt.Errorf("资料图片超过容量限制")
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
		if err != nil || format != "png" || cfg.Width < 32 || cfg.Height < 32 || cfg.Width > 4096 || cfg.Height > 4096 || cfg.Width*cfg.Height > 16000000 || !validMediaPayload(raw, cfg, format) {
			return fmt.Errorf("资料图片无效")
		}
		title := string([]rune(name)[:min(250, len([]rune(name)))])
		uri := fmt.Sprintf("knowledge:%s#page=%d", id, figure.Page)
		p := ImageProvenance{Provider: "knowledge_document", DocumentID: id, Page: figure.Page, Title: title, PageURL: uri, OriginalURL: uri, DownloadURL: uri, Author: "原文未声明", License: "not_declared", RightsStatus: "not_declared", Description: figure.Caption, RetrievedAt: time.Now().UTC().Format(time.RFC3339), SHA256: fmt.Sprintf("%x", sha256.Sum256(raw))}
		asset, err := s.storeCoursewareImage(ctx, a, p, raw, cfg, "image/png")
		if err != nil {
			return err
		}
		figure.AssetID, figure.DataURL = asset.ID, ""
		manifest = append(manifest, figure)
	}
	tx, err := s.tx(ctx, a.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, "UPDATE documents SET images=$1,image_warnings=$2 WHERE id=$3 AND member_id=$4 AND status='indexing'", asJSON(manifest), asJSON(extracted.ImageWarnings), id, a.MemberID)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("资料已删除或解析状态已改变")
	}
	return tx.Commit(ctx)
}

// Knowledge documents are tenant-shared; exported image references remain member-private.
func (s *Server) copyDocumentImage(ctx context.Context, a Actor, figure DocumentImage) (MediaAsset, error) {
	var asset MediaAsset
	tx, err := s.tx(ctx, a.TenantID)
	if err != nil {
		return asset, err
	}
	err = tx.QueryRow(ctx, "SELECT metadata FROM media_assets WHERE id=$1", figure.AssetID).Scan(&asset)
	tx.Rollback(ctx)
	if err != nil {
		return asset, err
	}
	if asset.Provenance == nil || asset.Provenance.Provider != "knowledge_document" || asset.Provenance.Page != figure.Page {
		return asset, fail(404, "资料图片来源无效")
	}
	raw, err := s.readStoredFile(ctx, filepath.Join(s.cfg.Storage, "media", asset.ID))
	if err != nil {
		return asset, err
	}
	if len(raw) != asset.Bytes || fmt.Sprintf("%x", sha256.Sum256(raw)) != asset.SHA256 {
		return asset, fail(503, "资料图片内容校验失败")
	}
	return s.storeCoursewareImage(ctx, a, *asset.Provenance, raw, image.Config{Width: asset.Width, Height: asset.Height}, asset.MIME)
}

func (s *Server) documentImages(w http.ResponseWriter, r *http.Request, a Actor) error {
	tx, err := s.tx(r.Context(), a.TenantID)
	if err != nil {
		return err
	}
	var figures []DocumentImage
	var warnings []string
	err = tx.QueryRow(r.Context(), "SELECT images,image_warnings FROM documents WHERE id=$1 AND status='ready'", r.PathValue("id")).Scan(&figures, &warnings)
	tx.Rollback(r.Context())
	if err != nil {
		return err
	}
	out := []map[string]any{}
	for _, figure := range figures {
		asset, err := s.copyDocumentImage(r.Context(), a, figure)
		if err != nil {
			return err
		}
		out = append(out, map[string]any{"asset_id": asset.ID, "page": figure.Page, "caption": figure.Caption, "provenance": asset.Provenance})
	}
	respond(w, 200, map[string]any{"images": out, "warnings": warnings})
	return nil
}

func (s *Server) reparseDocument(w http.ResponseWriter, r *http.Request, a Actor) error {
	tx, err := s.tx(r.Context(), a.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	result, err := tx.Exec(r.Context(), "UPDATE documents SET status='pending',error='' WHERE id=$1 AND upload_path<>'' AND status IN ('ready','failed')", r.PathValue("id"))
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fail(409, "只有保留原文件且未在解析中的资料可以重新解析")
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	respond(w, 202, map[string]bool{"ok": true})
	return nil
}

func (s *Server) searchKnowledgeImages(ctx context.Context, a Actor, task string, in TaskInput, need ImageNeed, slide Slide, maxCandidates int) ImageSearchResult {
	out := ImageSearchResult{Status: "not_found", Message: "知识库未找到合适图片，页面内容保留。", Candidates: []ImageCandidate{}}
	if (!in.UseKB && in.MaterialID == "") || s.db == nil {
		return out
	}
	tx, err := s.tx(ctx, a.TenantID)
	if err != nil {
		out.Status = "unavailable"
		return out
	}
	// ponytail: bounded scan of 200 tenant documents; add indexed captions if the library grows.
	rows, err := tx.Query(ctx, "SELECT id,name,images FROM documents WHERE status='ready' AND jsonb_array_length(images)>0 AND ($1='' OR id=$1) ORDER BY created_at DESC LIMIT 200", in.MaterialID)
	if err != nil {
		tx.Rollback(ctx)
		out.Status = "unavailable"
		return out
	}
	type candidate struct {
		figure DocumentImage
		score  int
	}
	candidates := []candidate{}
	tokens := relevanceTerms(strings.Join(need.Queries, " ") + " " + need.Purpose + " " + slide.Title)
	for rows.Next() {
		var id, name string
		var figures []DocumentImage
		if rows.Scan(&id, &name, &figures) != nil {
			continue
		}
		for _, figure := range figures {
			if in.MaterialID != "" && in.MaterialStart > 0 && (figure.Page < in.MaterialStart || figure.Page > in.MaterialEnd) {
				continue
			}
			score := 0
			body := strings.ToLower(name + " " + figure.Caption)
			for _, term := range tokens {
				if strings.Contains(body, term) {
					score++
				}
			}
			if score > 0 || in.MaterialID == id {
				candidates = append(candidates, candidate{figure, score})
			}
		}
	}
	rowErr := rows.Err()
	rows.Close()
	tx.Rollback(ctx)
	if rowErr != nil {
		out.Status = "unavailable"
		return out
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	shared, _ := ctx.Value(coursewareImageReviewBudgetKey{}).(*int)
	if shared == nil {
		shared = new(int)
	}
	for _, candidate := range candidates {
		if *shared >= 3 || ctx.Err() != nil {
			break
		}
		asset, err := s.copyDocumentImage(ctx, a, candidate.figure)
		if err != nil {
			continue
		}
		raw, err := s.readStoredFile(ctx, filepath.Join(s.cfg.Storage, "media", asset.ID))
		if err != nil {
			continue
		}
		(*shared)++
		relevant, reason, err := s.reviewCoursewareImage(ctx, a, task, need, slide, *asset.Provenance, asset.MIME, raw)
		if err != nil {
			out.Status = "unavailable"
			out.Message = "知识库图片相关性审核未完成，未自动采用图片。"
			continue
		}
		if !relevant {
			continue
		}
		out.Candidates = append(out.Candidates, ImageCandidate{Picture: TeachingPicture{AssetID: asset.ID, Caption: need.Purpose, Source: imageAttribution(asset.Provenance)}, Provenance: *asset.Provenance, Reason: reason})
		if len(out.Candidates) >= maxCandidates {
			break
		}
	}
	if len(out.Candidates) > 0 {
		out.Status = "found"
		out.Message = "已从企业资料自动匹配配图，文档和页码出处已记录。"
	}
	return out
}
