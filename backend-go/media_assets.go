package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"image/gif"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
)

type mediaActorKey struct{}

var mediaID = regexp.MustCompile(`^[a-f0-9]{32}$`)

type MediaAsset struct {
	ID         string           `json:"asset_id"`
	MIME       string           `json:"mime"`
	SHA256     string           `json:"sha256"`
	Bytes      int              `json:"bytes"`
	Width      int              `json:"width"`
	Height     int              `json:"height"`
	Provenance *ImageProvenance `json:"provenance,omitempty"`
}

func (s *Server) mediaMetadata(w http.ResponseWriter, r *http.Request, a Actor) error {
	asset, err := s.ownedMedia(r.Context(), a, r.PathValue("asset"))
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "private, no-store")
	respond(w, 200, asset)
	return nil
}

func (s *Server) uploadMedia(w http.ResponseWriter, r *http.Request, a Actor) error {
	if !allowed(a, "courseware.generate") && !allowed(a, "animation.render") {
		return fail(403, "没有素材上传权限")
	}
	r.Body = http.MaxBytesReader(w, r.Body, (8<<20)+(64<<10))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		return fail(400, "请选择8MB以内的PNG、JPEG或GIF图片")
	}
	defer r.MultipartForm.RemoveAll()
	f, _, err := r.FormFile("file")
	if err != nil {
		return fail(400, "请选择图片")
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, (8<<20)+1))
	if err != nil {
		return err
	}
	if len(raw) > 8<<20 {
		return fail(413, "图片不能超过8MB")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	mime := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "gif": "image/gif"}[format]
	if err != nil || mime == "" || cfg.Width < 32 || cfg.Height < 32 || cfg.Width > 4096 || cfg.Height > 4096 || cfg.Width*cfg.Height > 16000000 {
		return fail(400, "图片格式或尺寸无效：边长32至4096像素，总像素不超过1600万")
	}
	if !validMediaPayload(raw, cfg, format) {
		return fail(400, "图片数据损坏，或GIF超过120帧/8000万总像素")
	}
	asset := MediaAsset{ID: newID(), MIME: mime, SHA256: fmt.Sprintf("%x", sha256.Sum256(raw)), Bytes: len(raw), Width: cfg.Width, Height: cfg.Height}
	path := filepath.Join(s.cfg.Storage, "media", asset.ID)
	if err = s.writeStoredFile(r.Context(), path, raw); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = s.removeStoredFile(context.Background(), path)
		}
	}()
	tx, err := s.tx(r.Context(), a.TenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), "INSERT INTO media_assets(id,tenant_id,member_id,metadata) VALUES($1,$2,$3,$4)", asset.ID, a.TenantID, a.MemberID, asJSON(asset))
	if err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	committed = true
	respond(w, 201, asset)
	return nil
}

func (s *Server) ownedMedia(ctx context.Context, a Actor, id string) (MediaAsset, error) {
	var asset MediaAsset
	if !mediaID.MatchString(id) {
		return asset, fail(404, "图片不存在或无权访问")
	}
	tx, err := s.tx(ctx, a.TenantID)
	if err != nil {
		return asset, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, "SELECT metadata FROM media_assets WHERE id=$1 AND member_id=$2", id, a.MemberID).Scan(&asset)
	return asset, err
}

func (s *Server) mediaFile(w http.ResponseWriter, r *http.Request, a Actor) error {
	asset, err := s.ownedMedia(r.Context(), a, r.PathValue("asset"))
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", asset.MIME)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	return s.serveStoredFile(w, r, filepath.Join(s.cfg.Storage, "media", asset.ID))
}

// Resolve only at the trusted render boundary; persisted courseware keeps IDs.
// Authentication and worker entry points attach the current task member.
func (s *Server) hydrateDeckAssets(ctx context.Context, d Deck) (Deck, error) {
	d.Slides = append([]Slide(nil), d.Slides...)
	total := 0
	cache := map[string]string{}
	metadata := map[string]MediaAsset{}
	load := func(id string) (string, error) {
		a, ok := ctx.Value(mediaActorKey{}).(Actor)
		if !ok {
			return "", fail(403, "图片引用缺少当前成员身份")
		}
		asset, err := s.ownedMedia(ctx, a, id)
		if err != nil {
			return "", err
		}
		metadata[id] = asset
		if asset.Provenance != nil && asset.Provenance.RightsStatus == "pending_confirmation" {
			return "", fail(400, "此网页图片使用权限待确认，请先确认权限再插入课件")
		}
		total += asset.Bytes
		if total > 64<<20 {
			return "", fail(400, "本次导出图片总量超过64MB，请缩减图片尺寸或拆分课件")
		}
		if data, ok := cache[id]; ok {
			return data, nil
		}
		raw, err := s.readStoredFile(ctx, filepath.Join(s.cfg.Storage, "media", asset.ID))
		if err != nil {
			return "", err
		}
		if len(raw) != asset.Bytes || fmt.Sprintf("%x", sha256.Sum256(raw)) != asset.SHA256 {
			return "", fail(503, "图片内容校验失败")
		}
		data := "data:" + asset.MIME + ";base64," + base64.StdEncoding.EncodeToString(raw)
		cache[id] = data
		return data, nil
	}
	for i := range d.Slides {
		sl := &d.Slides[i]
		if sl.Picture != nil {
			p := *sl.Picture
			// Attribution supplied by clients is never a source snapshot.
			p.Provenance = nil
			if p.AssetID != "" {
				var err error
				p.DataURL, err = load(p.AssetID)
				if err != nil {
					return d, err
				}
				p.Provenance = metadata[p.AssetID].Provenance
				if p.Provenance != nil {
					p.Source = imageAttribution(p.Provenance)
				}
			}
			sl.Picture = &p
		}
		sl.CanvasObjects = append([]CanvasObject(nil), sl.CanvasObjects...)
		for j := range sl.CanvasObjects {
			o := &sl.CanvasObjects[j]
			o.Provenance = nil
			if o.AssetID != "" {
				var err error
				o.DataURL, err = load(o.AssetID)
				if err != nil {
					return d, err
				}
				o.Provenance = metadata[o.AssetID].Provenance
			}
		}
	}
	return d, nil
}

func validPictureReference(p *TeachingPicture) bool {
	return p.AssetID != "" && mediaID.MatchString(p.AssetID) && strings.TrimSpace(p.DataURL) == ""
}

// Count GIF blocks before decoding frames so compressed uploads cannot allocate
// an unbounded number of full-size bitmaps. No remote URLs or generated code.
func validMediaPayload(raw []byte, cfg image.Config, format string) bool {
	if format != "gif" {
		_, _, err := image.Decode(bytes.NewReader(raw))
		return err == nil
	}
	if len(raw) < 13 {
		return false
	}
	i, frames := 13, 0
	if raw[10]&128 != 0 {
		i += 3 * (1 << ((raw[10] & 7) + 1))
	}
	skip := func() bool {
		for i < len(raw) {
			n := int(raw[i])
			i++
			if n == 0 {
				return true
			}
			i += n
		}
		return false
	}
	for i < len(raw) {
		block := raw[i]
		i++
		switch block {
		case 0x3b:
			if frames == 0 {
				return false
			}
			_, err := gif.DecodeAll(bytes.NewReader(raw))
			return err == nil
		case 0x21:
			i++
			if !skip() {
				return false
			}
		case 0x2c:
			if i+9 > len(raw) {
				return false
			}
			w, h := int(binary.LittleEndian.Uint16(raw[i+4:i+6])), int(binary.LittleEndian.Uint16(raw[i+6:i+8]))
			frames++
			if frames > 120 || cfg.Width*cfg.Height*frames > 80000000 || w > cfg.Width || h > cfg.Height {
				return false
			}
			packed := raw[i+8]
			i += 9
			if packed&128 != 0 {
				i += 3 * (1 << ((packed & 7) + 1))
			}
			i++
			if !skip() {
				return false
			}
		default:
			return false
		}
	}
	return false
}
