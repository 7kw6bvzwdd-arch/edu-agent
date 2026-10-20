package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

type coursewareImageReviewBudgetKey struct{}

// Search uses the existing configured service; actual pages supply image URLs.
// The system selects relevant images; source metadata never invents permission.
func (s *Server) searchCoursewareImages(ctx context.Context, a Actor, task string, need ImageNeed, slide Slide, maxCandidates int) ImageSearchResult {
	if s.cfg.VisionURL == "" || s.cfg.VisionKey == "" || s.cfg.VisionModel == "" {
		return ImageSearchResult{Status: "unavailable", Message: "配图视觉审核未配置，未自动采用网络图片。", Candidates: []ImageCandidate{}}
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	budget := new(int)
	ctx = context.WithValue(ctx, coursewareImageReviewBudgetKey{}, budget)
	primaryCtx, stop := context.WithTimeout(ctx, 65*time.Second)
	out := s.searchWebImages(primaryCtx, a, task, need, slide, maxCandidates)
	stop()
	if out.Status == "found" || ctx.Err() != nil {
		return out
	}
	if *budget >= 3 {
		out.Message = "本次各来源合计3次图片视觉审核预算已用完，未再等待备用 Commons；未找到合适配图时保留本页内容。"
		return out
	}
	// A failed secondary source must not leave the UI waiting for 90 seconds.
	fallbackCtx, stopFallback := context.WithTimeout(ctx, 25*time.Second)
	defer stopFallback()
	fallbackNeed := need
	fallbackNeed.Queries = need.Queries[:1]
	backup := s.searchCommonsImages(fallbackCtx, a, task, fallbackNeed, slide, maxCandidates)
	if s.cfg.SearchURL == "" || s.cfg.SearchKey == "" {
		return backup
	}
	if backup.Status == "found" {
		backup.Message = "已自动从备用 Commons 选取相关配图，来源已记录。"
		return backup
	}
	if out.Status == "not_found" && backup.Status == "not_found" {
		out.Message = "网页及备用 Commons 均未找到合适图片，可修改检索词或上传。"
	} else {
		out.Status = "unavailable"
		out.Message = "网页检索未取得可用图片；备用 Commons 暂时无法访问或未完成检索，不代表没有图片。可上传配图或稍后重试，课件内容已保留。"
	}
	return out
}

func webImageURL(raw string) (*url.URL, error) {
	u, err := validateURL(raw)
	if err != nil || len(raw) > 4000 {
		return nil, fmt.Errorf("invalid image source URL")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && !publicIP(ip) {
		return nil, fmt.Errorf("private image source")
	}
	u.Fragment = ""
	return u, nil
}

func (s *Server) webImageGet(ctx context.Context, raw string, limit int64) ([]byte, string, error) {
	u, err := webImageURL(raw)
	if err != nil {
		return nil, "", err
	}
	client := *s.mediaHTTP
	client.Timeout = 7 * time.Second
	prior := client.CheckRedirect
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) > 3 {
			return fmt.Errorf("too many image redirects")
		}
		if _, err := webImageURL(r.URL.String()); err != nil {
			return err
		}
		if prior != nil {
			return prior(r, via)
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "EduCourseware/0.1 (teaching-image-research)")
	res, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 || res.ContentLength > limit {
		return nil, "", fmt.Errorf("image source unavailable")
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, "", fmt.Errorf("image source too large")
	}
	return b, res.Request.URL.String(), nil
}

func imageNodeAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
func walkImageNodes(n *html.Node, visit func(*html.Node)) {
	visit(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkImageNodes(c, visit)
	}
}
func imageNodeText(n *html.Node) string {
	var b strings.Builder
	walkImageNodes(n, func(c *html.Node) {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
			b.WriteByte(' ')
		}
	})
	return plainImageMetadata(b.String(), 1500)
}

func webPageImages(raw []byte, pageURL, title string) []ImageProvenance {
	reader, err := charset.NewReader(bytes.NewReader(raw), "")
	if err != nil {
		return nil
	}
	root, err := html.Parse(reader)
	if err != nil {
		return nil
	}
	base, _ := url.Parse(pageURL)
	resolve := func(raw string) string {
		u, e := url.Parse(strings.TrimSpace(raw))
		if e != nil || strings.TrimSpace(raw) == "" {
			return ""
		}
		u = base.ResolveReference(u)
		if _, e = webImageURL(u.String()); e != nil {
			return ""
		}
		return u.String()
	}
	author, description := "未标明", ""
	walkImageNodes(root, func(n *html.Node) {
		if n.Type != html.ElementNode {
			return
		}
		if n.Data == "title" {
			if t := imageNodeText(n); t != "" {
				title = t
			}
		}
		if n.Data == "meta" {
			switch strings.ToLower(imageNodeAttr(n, "name")) {
			case "author":
				author = plainImageMetadata(imageNodeAttr(n, "content"), 1500)
			case "description":
				description = plainImageMetadata(imageNodeAttr(n, "content"), 1500)
			}
		}
	})
	out := []ImageProvenance{}
	seen := map[string]bool{}
	add := func(src, desc, creator, license string) {
		src = resolve(src)
		if src == "" || seen[src] || len(out) >= 6 {
			return
		}
		seen[src] = true
		if desc == "" {
			desc = description
		}
		if creator == "" {
			creator = author
		}
		p := ImageProvenance{Provider: "web_page", Title: plainImageMetadata(title, 250), PageURL: pageURL, OriginalURL: src, DownloadURL: src, Author: creator, Description: plainImageMetadata(desc, 1500), License: "来源未标明许可", RightsStatus: "not_declared"}
		// Only an explicit image/figure license is accepted automatically; a
		// page footer's general copyright statement does not license photos.
		license = resolve(license)
		if commonsCCLicense.MatchString(license) {
			p.LicenseURL = license
			p.RightsStatus = "source_declared"
			u, _ := url.Parse(license)
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if parts[0] == "licenses" {
				p.License = "CC " + strings.ToUpper(parts[1]) + " " + parts[2]
			} else if parts[1] == "zero" {
				p.License = "CC0"
			} else {
				p.License = "Public domain"
			}
		}
		out = append(out, p)
	}
	walkImageNodes(root, func(n *html.Node) {
		if n.Type != html.ElementNode {
			return
		}
		if n.Data == "img" {
			src := imageNodeAttr(n, "data-src")
			if src == "" {
				src = imageNodeAttr(n, "src")
			}
			desc, license := imageNodeAttr(n, "alt"), ""
			if desc == "" {
				desc = imageNodeAttr(n, "title")
			}
			for parent := n.Parent; parent != nil; parent = parent.Parent {
				if parent.Data == "figure" {
					desc = imageNodeText(parent) + " " + desc
					walkImageNodes(parent, func(c *html.Node) {
						if c.Type == html.ElementNode && strings.Contains(" "+strings.ToLower(imageNodeAttr(c, "rel"))+" ", " license ") {
							license = imageNodeAttr(c, "href")
						}
					})
					break
				}
			}
			add(src, desc, "", license)
		}
		if n.Data == "meta" && imageNodeAttr(n, "property") == "og:image" {
			add(imageNodeAttr(n, "content"), description, "", "")
		}
	})
	return out
}

func (s *Server) searchWebImages(ctx context.Context, a Actor, task string, need ImageNeed, slide Slide, maxCandidates int) ImageSearchResult {
	out := ImageSearchResult{Status: "not_found", Message: "网页未找到符合用途的图片。", Candidates: []ImageCandidate{}}
	if s.cfg.SearchURL == "" || s.cfg.SearchKey == "" {
		out.Status = "unavailable"
		out.Message = "网页搜索未配置，将尝试备用 Commons。"
		return out
	}
	seen := map[string]bool{}
	checked, pages, downloads := 0, 0, 0
	shared, _ := ctx.Value(coursewareImageReviewBudgetKey{}).(*int)
	if shared == nil {
		shared = new(int)
	}
	for _, query := range need.Queries {
		client := *s.http
		client.Timeout = 10 * time.Second
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		b, err := s.postBytesWithClient(ctx, &client, s.cfg.SearchURL, map[string]any{"api_key": s.cfg.SearchKey, "query": strings.TrimSpace(query) + " 图片 照片", "max_results": 4, "include_raw_content": false}, "")
		if err != nil {
			out.Status = "unavailable"
			continue
		}
		var response struct {
			Results []struct{ Title, URL string } `json:"results"`
		}
		if json.Unmarshal(b, &response) != nil {
			out.Status = "unavailable"
			continue
		}
		for _, result := range response.Results {
			u, err := webImageURL(result.URL)
			if err != nil || seen[u.String()] || strings.HasSuffix(u.Hostname(), "wikimedia.org") {
				continue
			}
			if pages >= 4 || checked >= 3 || *shared >= 3 || ctx.Err() != nil {
				break
			}
			pages++
			seen[u.String()] = true
			page, finalURL, err := s.webImageGet(ctx, u.String(), 2<<20)
			if err != nil {
				out.Status = "unavailable"
				continue
			}
			for _, p := range webPageImages(page, finalURL, result.Title) {
				if seen[p.OriginalURL] {
					continue
				}
				seen[p.OriginalURL] = true
				if checked >= 3 || *shared >= 3 || downloads >= 8 || ctx.Err() != nil {
					break
				}
				downloads++
				raw, _, err := s.webImageGet(ctx, p.DownloadURL, 8<<20)
				if err != nil {
					continue
				}
				cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
				mime := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "gif": "image/gif"}[format]
				if err != nil || mime == "" || cfg.Width < 320 || cfg.Height < 200 || cfg.Width > 4096 || cfg.Height > 4096 || cfg.Width*cfg.Height > 16000000 || !validMediaPayload(raw, cfg, format) {
					continue
				}
				checked++
				*shared++
				relevant, reason, err := s.reviewCoursewareImage(ctx, a, task, need, slide, p, mime, raw)
				if err != nil {
					out.Status = "unavailable"
					continue
				}
				if !relevant {
					continue
				}
				p.RetrievedAt = time.Now().UTC().Format(time.RFC3339)
				p.SHA256 = fmt.Sprintf("%x", sha256.Sum256(raw))
				asset, err := s.storeCoursewareImage(ctx, a, p, raw, cfg, mime)
				if err != nil {
					out.Status = "unavailable"
					continue
				}
				candidate := ImageCandidate{Picture: TeachingPicture{AssetID: asset.ID, Caption: need.Purpose, Source: imageAttribution(asset.Provenance)}, Provenance: *asset.Provenance, Reason: reason}
				out.Candidates = append(out.Candidates, candidate)
				if len(out.Candidates) >= maxCandidates {
					break
				}
			}
			if len(out.Candidates) >= maxCandidates {
				break
			}
		}
		if len(out.Candidates) >= maxCandidates || checked >= 3 || pages >= 4 || ctx.Err() != nil {
			break
		}
	}
	if len(out.Candidates) > 0 {
		out.Status = "found"
		out.Message = "已自动选取与本页用途相关的网页配图，实际来源与许可信息已记录。"
	}
	return out
}
