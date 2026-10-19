package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

type CanvasOffset struct {
	DX       float64  `json:"dx"`
	DY       float64  `json:"dy"`
	Scale    *float64 `json:"scale,omitempty"`
	Rotation float64  `json:"rotation,omitempty"`
	Locked   bool     `json:"locked,omitempty"`
}

type CanvasObject struct {
	ID         string           `json:"id"`
	Kind       string           `json:"kind"`
	X          float64          `json:"x"`
	Y          float64          `json:"y"`
	Width      float64          `json:"width"`
	Height     float64          `json:"height"`
	Rotation   float64          `json:"rotation,omitempty"`
	Locked     bool             `json:"locked,omitempty"`
	Text       string           `json:"text,omitempty"`
	FontSize   float64          `json:"font_size,omitempty"`
	Bold       bool             `json:"bold,omitempty"`
	Align      string           `json:"align,omitempty"`
	Color      string           `json:"color,omitempty"`
	Background string           `json:"background,omitempty"`
	Shape      string           `json:"shape,omitempty"`
	LineWidth  float64          `json:"line_width,omitempty"`
	AssetID    string           `json:"asset_id,omitempty"`
	DataURL    string           `json:"data_url,omitempty"`
	Caption    string           `json:"caption,omitempty"`
	Provenance *ImageProvenance `json:"provenance,omitempty"`
}

var canvasID = regexp.MustCompile(`^object-[A-Za-z0-9-]{1,64}$`)
var canvasColor = regexp.MustCompile(`^[A-Fa-f0-9]{6}$`)

func canvasFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func validateCanvasObjects(sl Slide) error {
	if len(sl.CanvasObjects) > 24 || len(sl.CanvasOrder) > 29 {
		return fail(400, "每页最多24个新增对象")
	}
	ids := map[string]bool{"title": true, "body": len(sl.Bullets) > 0, "focus": sl.Focus != "", "formula": sl.Formula != "", "visual": sl.Graph != nil || sl.Scientific != nil || sl.Picture != nil || sl.Table != nil || sl.Figure != nil || sl.Chart != nil || sl.Diagram != ""}
	for _, o := range sl.CanvasObjects {
		if !canvasID.MatchString(o.ID) || ids[o.ID] {
			return fail(400, "对象编号无效或重复")
		}
		ids[o.ID] = true
		if !canvasFinite(o.X) || !canvasFinite(o.Y) || !canvasFinite(o.Width) || !canvasFinite(o.Height) || !canvasFinite(o.Rotation) || o.X < 0 || o.Y < 0 || o.Width < .01 || o.Height < .005 || o.X+o.Width > 1.000001 || o.Y+o.Height > 1.000001 || math.Abs(o.Rotation) > 180 {
			return fail(400, "对象尺寸或位置超出画布")
		}
		if o.Color != "" && !canvasColor.MatchString(o.Color) || o.Background != "" && !canvasColor.MatchString(o.Background) || !canvasFinite(o.LineWidth) || o.LineWidth != 0 && (o.LineWidth < .5 || o.LineWidth > 8) {
			return fail(400, "对象颜色或线宽无效")
		}
		switch o.Kind {
		case "text":
			if strings.TrimSpace(o.Text) == "" || len([]rune(o.Text)) > 600 || !canvasFinite(o.FontSize) || o.FontSize != 0 && (o.FontSize < 16 || o.FontSize > 60) || o.Align != "" && o.Align != "left" && o.Align != "center" && o.Align != "right" {
				return fail(400, "文本框须有文字，最多600字，字号16至60")
			}
		case "image":
			if len([]rune(o.Caption)) > 60 {
				return fail(400, "图片说明最多60字")
			}
			if err := validateTeachingMedia(Slide{Picture: &TeachingPicture{AssetID: o.AssetID, DataURL: o.DataURL, Caption: "教师配图", Source: "教师添加"}}); err != nil {
				return err
			}
		case "shape":
			if o.Shape != "" && o.Shape != "rect" && o.Shape != "ellipse" && o.Shape != "line" && o.Shape != "arrow" {
				return fail(400, "不支持的图形")
			}
		default:
			return fail(400, "不支持的画布对象")
		}
		if o.Kind != "image" && (o.DataURL != "" || o.AssetID != "") {
			return fail(400, "图片内容与对象类型不符")
		}
	}
	seen := map[string]bool{}
	for _, id := range sl.CanvasOrder {
		if !ids[id] || seen[id] {
			return fail(400, "图层顺序包含不存在或重复的对象")
		}
		seen[id] = true
	}
	return nil
}

func validateCanvasPositions(sl Slide) error {
	if err := validateCanvasObjects(sl); err != nil {
		return err
	}
	for key, offset := range sl.CanvasPositions {
		switch key {
		case "title", "body", "visual", "formula", "focus":
		default:
			return fail(400, "无效画布对象")
		}
		if math.IsNaN(offset.DX) || math.IsNaN(offset.DY) || math.IsInf(offset.DX, 0) || math.IsInf(offset.DY, 0) || math.Abs(offset.DX) > .8 || math.Abs(offset.DY) > .8 {
			return fail(400, "对象位置超出画布范围")
		}
		if offset.Scale != nil && (math.IsNaN(*offset.Scale) || math.IsInf(*offset.Scale, 0) || *offset.Scale < .8 || *offset.Scale > 1.5) {
			return fail(400, "对象缩放比例须在80%至150%之间")
		}
		if !canvasFinite(offset.Rotation) || math.Abs(offset.Rotation) > 180 {
			return fail(400, "旋转角度须在-180至180度之间")
		}
		if offset.Rotation != 0 && (key == "formula" || key == "visual" && (sl.Graph != nil || sl.Scientific != nil || sl.Chart != nil || sl.Diagram != "")) {
			return fail(400, "学科坐标图、受力图、电路图和公式保持原方向；可以移动和等比例缩放")
		}
	}
	return nil
}

func (s *Server) coursewareObjectMap(w http.ResponseWriter, r *http.Request, a Actor) error {
	tx, err := s.tx(r.Context(), a.TenantID)
	if err != nil {
		return err
	}
	st, err := ownedCourseware(r.Context(), tx, a, r.PathValue("id"), "")
	tx.Rollback(r.Context())
	if err != nil {
		return err
	}
	if r.URL.Query().Get("revision") != strconv.Itoa(st.Revision) {
		return fail(409, "预览版本已变化，请重新加载")
	}
	page := 0
	for i, sl := range st.Deck.Slides {
		if sl.ID == r.PathValue("slide") {
			page = i + 1
		}
	}
	if page == 0 || st.Path == "" {
		return pgx.ErrNoRows
	}
	data, err := s.readStoredFile(r.Context(), st.Path)
	if err != nil {
		return err
	}
	result, err := s.renderCanvasObjectMap(r.Context(), data, page)
	if err != nil {
		return err
	}
	// Stored files from earlier canvas releases lack rotation metadata.
	sl := st.Deck.Slides[page-1]
	for i := range result.Objects {
		switch result.Objects[i].ID {
		case "title", "body", "focus":
			result.Objects[i].Rotatable = true
		case "visual":
			result.Objects[i].Rotatable = sl.Graph == nil && sl.Scientific == nil && sl.Chart == nil && sl.Diagram == ""
		}
	}
	respond(w, 200, result)
	return nil
}

type CanvasMap struct {
	Objects []struct {
		ID        string  `json:"id"`
		X         float64 `json:"x"`
		Y         float64 `json:"y"`
		Width     float64 `json:"width"`
		Height    float64 `json:"height"`
		MinScale  float64 `json:"min_scale"`
		Rotatable bool    `json:"rotatable"`
		Rotation  float64 `json:"rotation"`
	} `json:"objects"`
	Version string `json:"version"`
}

func (s *Server) renderCanvasObjectMap(ctx context.Context, data []byte, page int) (CanvasMap, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", s.cfg.RenderURL+"/render/object-map?page="+strconv.Itoa(page), bytes.NewReader(data))
	if err != nil {
		return CanvasMap{}, err
	}
	req.Header.Set("Content-Type", "application/vnd.openxmlformats-officedocument.presentationml.presentation")
	res, err := s.http.Do(req)
	if err != nil {
		return CanvasMap{}, fail(503, "画布对象暂时无法读取，已保存内容保留")
	}
	defer res.Body.Close()
	var result CanvasMap
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 32768)).Decode(&result) != nil || result.Version != "canvas-v3" {
		return CanvasMap{}, fail(503, "当前文件暂不支持对象移动，请先保存以更新真实预览")
	}
	return result, nil
}

// ponytail: one Office preview at a time on this local runtime; return busy
// instead of building an unbounded queue of abandoned pointer previews.
var canvasPreviewSlot = make(chan struct{}, 1)

func (s *Server) coursewareCanvasPreview(w http.ResponseWriter, r *http.Request, a Actor) error {
	var body struct {
		Revision int  `json:"revision"`
		Result   Deck `json:"result"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	tx, err := s.tx(r.Context(), a.TenantID)
	if err != nil {
		return err
	}
	st, err := ownedCourseware(r.Context(), tx, a, r.PathValue("id"), "")
	tx.Rollback(r.Context())
	if err != nil {
		return err
	}
	if st.Revision != body.Revision {
		return fail(409, "预览版本已变化；当前修改仍保留")
	}
	if st.Status != "succeeded" && st.Status != "awaiting_preview" && st.Status != "awaiting_confirmation" {
		return fail(409, "制作中暂不能预览画布修改")
	}
	if err := validateDeckMinimum(body.Result, 1); err != nil {
		return err
	}
	var page *Slide
	pageNumber := 0
	for i := range body.Result.Slides {
		if body.Result.Slides[i].ID == r.PathValue("slide") {
			page = &body.Result.Slides[i]
			pageNumber = i + 1
		}
	}
	owned := false
	for _, sl := range st.Deck.Slides {
		if sl.ID == r.PathValue("slide") {
			owned = true
		}
	}
	if page == nil || !owned {
		return fail(404, "页面不存在")
	}
	select {
	case canvasPreviewSlot <- struct{}{}:
		defer func() { <-canvasPreviewSlot }()
	default:
		return fail(429, "实际预览正在排版，请稍后重试")
	}
	pptx, err := s.postBytes(r.Context(), s.cfg.RenderURL+"/render/canvas-preview-pptx?page="+strconv.Itoa(pageNumber), body.Result, "")
	if err != nil {
		return err
	}
	objectMap, err := s.renderCanvasObjectMap(r.Context(), pptx, 1)
	if err != nil {
		return err
	}
	png, err := s.coursewarePagePNG(r.Context(), a, r.PathValue("id"), pptx)
	if err != nil {
		return fail(503, "实际预览暂不可用，当前修改保留")
	}
	// Revalidate ownership/version after rendering; a deleted or updated work
	// cannot return its preview through the earlier authorization snapshot.
	tx, err = s.tx(r.Context(), a.TenantID)
	if err != nil {
		return err
	}
	current, err := ownedCourseware(r.Context(), tx, a, r.PathValue("id"), "")
	tx.Rollback(r.Context())
	if err != nil {
		return err
	}
	if current.Status != "succeeded" && current.Status != "awaiting_preview" && current.Status != "awaiting_confirmation" {
		return fail(409, "制作状态已变化，当前修改仍保留")
	}
	if current.Revision != body.Revision {
		return fail(409, "排版期间版本已更新；当前修改仍保留")
	}
	respond(w, 200, map[string]any{"image": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), "revision": body.Revision, "objects": objectMap.Objects})
	return nil
}
