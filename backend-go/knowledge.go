package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/html"
)

type Page struct {
	Page int    `json:"page"`
	Text string `json:"text"`
}

const documentUploadLimit = 100 << 20

func (s *Server) listDocuments(w http.ResponseWriter, r *http.Request, a Actor) error {
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	v, e := queryMaps(r.Context(), tx, "SELECT d.id,d.name,d.status,d.error,d.source_url,d.created_at,(d.upload_path<>'') AS original_available,jsonb_array_length(d.images) AS image_count,d.image_warnings,(SELECT count(*) FROM chunks c WHERE c.document_id=d.id) AS chunks FROM documents d ORDER BY d.created_at DESC")
	if e != nil {
		return e
	}
	respond(w, 200, v)
	return nil
}
func (s *Server) deleteDocument(w http.ResponseWriter, r *http.Request, a Actor) error {
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	var path string
	if e = tx.QueryRow(r.Context(), "SELECT upload_path FROM documents WHERE id=$1", r.PathValue("id")).Scan(&path); e != nil {
		return e
	}
	if e = s.deleteVectors(r.Context(), a.TenantID, r.PathValue("id")); e != nil {
		return e
	}
	res, e := tx.Exec(r.Context(), "DELETE FROM documents WHERE id=$1", r.PathValue("id"))
	if e != nil {
		return e
	}
	if res.RowsAffected() == 0 {
		return fail(404, "文档不存在")
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	if path != "" {
		if e = s.removeStoredFile(r.Context(), path); e != nil {
			return e
		}
	}
	respond(w, 200, map[string]bool{"ok": true})
	return nil
}
func (s *Server) uploadDocument(w http.ResponseWriter, r *http.Request, a Actor) error {
	r.Body = http.MaxBytesReader(w, r.Body, documentUploadLimit+(1<<20))
	if e := r.ParseMultipartForm(21 << 20); e != nil {
		return fail(400, "文件无效或超过100MB")
	}
	defer r.MultipartForm.RemoveAll()
	f, h, e := r.FormFile("file")
	if e != nil {
		return fail(400, "请选择文件")
	}
	defer f.Close()
	name := filepath.Base(h.Filename)
	ext := strings.ToLower(filepath.Ext(name))
	if !strings.Contains("|.pdf|.docx|.pptx|.txt|.md|", "|"+ext+"|") {
		return fail(400, "支持PDF/DOCX/PPTX/TXT/MD")
	}
	id := newID()
	path := storagePath(s, a.TenantID, id, ext)
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	data, e := io.ReadAll(io.LimitReader(f, documentUploadLimit+1))
	if e != nil {
		return e
	}
	if len(data) > documentUploadLimit {
		return fail(413, "文件超过100MB")
	}
	if e = s.writeStoredFile(r.Context(), path, data); e != nil {
		return e
	}
	tx, e := s.tx(r.Context(), a.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	_, e = tx.Exec(r.Context(), "INSERT INTO documents(id,tenant_id,member_id,name,upload_path) VALUES($1,$2,$3,$4,$5)", id, a.TenantID, a.MemberID, name, path)
	if e != nil {
		_ = s.removeStoredFile(r.Context(), path)
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	respond(w, 202, map[string]string{"id": id, "status": "pending"})
	return nil
}
func (s *Server) extractFile(ctx context.Context, path, name string) ([]Page, error) {
	data, err := s.extractDocument(ctx, path, name)
	return data.Pages, err
}

func (s *Server) extractDocument(ctx context.Context, path, name string) (DocumentExtraction, error) {
	var data DocumentExtraction
	content, e := s.readStoredFile(ctx, path)
	if e != nil {
		return data, e
	}
	f := bytes.NewReader(content)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, e := mw.CreateFormFile("file", name)
	if e != nil {
		return data, e
	}
	if _, e = io.Copy(part, io.LimitReader(f, documentUploadLimit)); e != nil {
		return data, e
	}
	mw.Close()
	req, e := http.NewRequestWithContext(ctx, "POST", s.cfg.RenderURL+"/extract", &buf)
	if e != nil {
		return data, e
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, e := s.http.Do(req)
	if e != nil {
		return data, fmt.Errorf("文档解析服务不可用")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return data, fmt.Errorf("文档无法解析：格式无效、空文档或扫描件不支持OCR")
	}
	e = json.NewDecoder(io.LimitReader(res.Body, 24<<20)).Decode(&data)
	return data, e
}

func (s *Server) saveDocument(ctx context.Context, a Actor, name, sourceURL string, pages []Page, existing ...string) (string, error) {
	// PDF glyph separators can contain NUL; preserve word boundaries before embedding or storage.
	for i := range pages {
		pages[i].Text = strings.ReplaceAll(pages[i].Text, "\x00", " ")
	}
	// Embedding happens before the transaction; failure never leaves a partially indexed document.
	type chunk struct{ text, location, vector string }
	chunks := []chunk{}
	for _, p := range pages {
		r := []rune(strings.TrimSpace(p.Text))
		for start := 0; start < len(r); start += 600 {
			end := min(start+700, len(r))
			c := chunk{text: string(r[start:end]), location: fmt.Sprintf("第%d页 · 段%d", p.Page, start/600+1)}
			if s.cfg.Mode == "live" && s.cfg.EmbeddingModel != "" {
				v, e := s.embed(ctx, a, c.text)
				if e != nil {
					return "", e
				}
				c.vector = v
			}
			chunks = append(chunks, c)
			if len(chunks) > 2500 {
				return "", fail(413, "文档切片过多，请拆分文档")
			}
			if end == len(r) {
				break
			}
		}
	}
	if len(chunks) == 0 {
		return "", fail(422, "文档没有可索引的文字")
	}
	tx, e := s.tx(ctx, a.TenantID)
	if e != nil {
		return "", e
	}
	defer tx.Rollback(ctx)
	id := newID()
	if len(existing) > 0 {
		id = existing[0]
		res, err := tx.Exec(ctx, "UPDATE documents SET status='ready',pages=$1,error='',lease_until=NULL WHERE id=$2 AND status='indexing'", asJSON(pages), id)
		if err != nil {
			return "", err
		}
		if res.RowsAffected() != 1 {
			return "", fmt.Errorf("文档已删除或状态变更")
		}
		if _, e = tx.Exec(ctx, "DELETE FROM chunks WHERE document_id=$1", id); e != nil {
			return "", e
		}
	} else {
		_, e = tx.Exec(ctx, "INSERT INTO documents(id,tenant_id,member_id,name,status,pages,source_url) VALUES($1,$2,$3,$4,'ready',$5,$6)", id, a.TenantID, a.MemberID, name, asJSON(pages), sourceURL)
		if e != nil {
			return "", e
		}
	}

	if e = s.deleteVectors(ctx, a.TenantID, id); e != nil {
		return "", e
	}
	for i, c := range chunks {
		cid := newID()
		if e = s.upsertVector(ctx, a.TenantID, id, cid, c.vector); e != nil {
			return "", e
		}
		_, e = tx.Exec(ctx, "INSERT INTO chunks(id,tenant_id,document_id,seq,content,location,embedding) VALUES($1,$2,$3,$4,$5,$6,nullif($7,'')::vector)", cid, a.TenantID, id, i, c.text, c.location, c.vector)
		if e != nil {
			return "", e
		}
	}
	return id, tx.Commit(ctx)
}
func (s *Server) embed(ctx context.Context, a Actor, input string) (string, error) {
	hash := sha256.Sum256([]byte(input))
	cacheKey := hex.EncodeToString(hash[:])
	provider := s.cfg.EmbeddingURL
	if provider == "" {
		provider = s.cfg.LLMURL
	}
	key := s.cfg.EmbeddingKey
	if key == "" {
		key = s.cfg.LLMKey
	}
	redisKey := s.cfg.RedisPrefix + "embedding:" + strconv.FormatInt(a.TenantID, 10) + ":" + provider + ":" + s.cfg.EmbeddingModel + ":" + cacheKey
	if s.cloud.redis != nil {
		if cached, err := s.cloud.redis.Get(ctx, redisKey).Result(); err == nil {
			return cached, nil
		}
	}
	tx, e := s.tx(ctx, a.TenantID)
	if e != nil {
		return "", e
	}
	var cached string
	e = tx.QueryRow(ctx, "SELECT embedding::text FROM embedding_cache WHERE provider=$1 AND model=$2 AND content_hash=$3", provider, s.cfg.EmbeddingModel, cacheKey).Scan(&cached)
	tx.Rollback(ctx)
	if e == nil {
		if s.cloud.redis != nil {
			s.cloud.redis.Set(ctx, redisKey, cached, 24*time.Hour)
		}
		return cached, nil
	}
	start := time.Now()
	b, e := s.postBytes(ctx, strings.TrimRight(provider, "/")+"/embeddings", map[string]any{"model": s.cfg.EmbeddingModel, "input": input, "dimensions": 1536}, key)
	if e != nil {
		return "", e
	}
	var v struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if e = json.Unmarshal(b, &v); e != nil || len(v.Data) != 1 {
		return "", fmt.Errorf("向量服务返回无效数据")
	}
	if len(v.Data[0].Embedding) != 1536 {
		return "", fmt.Errorf("当前索引要求1536维向量，请配置匹配模型")
	}
	result := string(asJSON(v.Data[0].Embedding))
	tx, e = s.tx(ctx, a.TenantID)
	if e != nil {
		return "", e
	}
	defer tx.Rollback(ctx)
	cast := "::vector"
	_, e = tx.Exec(ctx, "INSERT INTO embedding_cache(tenant_id,provider,model,content_hash,embedding) VALUES($1,$2,$3,$4,$5"+cast+") ON CONFLICT DO NOTHING", a.TenantID, provider, s.cfg.EmbeddingModel, cacheKey, result)
	if e != nil {
		return "", e
	}
	_, e = tx.Exec(ctx, "INSERT INTO usage_logs(tenant_id,member_id,operation,model,latency_ms,success) VALUES($1,$2,'embedding',$3,$4,true)", a.TenantID, a.MemberID, s.cfg.EmbeddingModel, time.Since(start).Milliseconds())
	if e != nil {
		return "", e
	}
	if e = tx.Commit(ctx); e != nil {
		return "", e
	}
	if s.cloud.redis != nil {
		s.cloud.redis.Set(ctx, redisKey, result, 24*time.Hour)
	}
	return result, nil
}

func publicIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, s := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "2001::/32", "2002::/16", "64:ff9b::/96", "64:ff9b:1::/48"} {
		_, n, _ := net.ParseCIDR(s)
		if n.Contains(ip) {
			return false
		}
	}
	return true
}
func validateURL(raw string) (*url.URL, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("仅支持公开HTTP/HTTPS网页")
	}
	if p := u.Port(); p != "" && p != "80" && p != "443" {
		return nil, fmt.Errorf("仅允许80/443端口")
	}
	return u, nil
}
func publicHTTPClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
		if e != nil {
			return nil, e
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("域名无有效地址")
		}
		for _, ip := range ips {
			if !publicIP(ip.IP) {
				return nil, fmt.Errorf("禁止访问私网或保留地址")
			}
		}
		d := net.Dialer{Timeout: 10 * time.Second}
		return d.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second}
	return &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 3 {
			return fmt.Errorf("网页跳转过多")
		}
		_, e := validateURL(r.URL.String())
		return e
	}}
}
func fetchPage(ctx context.Context, raw string) (string, string, error) {
	if _, e := validateURL(raw); e != nil {
		return "", "", e
	}
	client := publicHTTPClient(25 * time.Second)
	defer client.CloseIdleConnections()
	req, e := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if e != nil {
		return "", "", e
	}
	req.Header.Set("User-Agent", "EduCourseware/0.1 (+public-page-research)")
	res, e := client.Do(req)
	if e != nil {
		return "", "", fmt.Errorf("网页抓取失败或地址被安全策略拦截")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", "", fmt.Errorf("网页返回HTTP %d", res.StatusCode)
	}
	ct := res.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") && !strings.Contains(ct, "text/plain") {
		return "", "", fmt.Errorf("只采集HTML或纯文本网页")
	}
	b, e := io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
	if e != nil {
		return "", "", e
	}
	if len(b) > 2<<20 {
		return "", "", fmt.Errorf("网页超过2MB")
	}
	if strings.Contains(ct, "text/plain") {
		return res.Request.URL.String(), string(b), nil
	}
	node, e := html.Parse(bytes.NewReader(b))
	if e != nil {
		return "", "", e
	}
	var title strings.Builder
	var body strings.Builder
	var walk func(*html.Node, bool)
	walk = func(n *html.Node, inTitle bool) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "nav", "footer", "header", "noscript", "form", "svg":
				return
			case "title":
				inTitle = true
			}
		}
		if n.Type == html.TextNode {
			t := strings.TrimSpace(n.Data)
			if t != "" {
				if inTitle {
					title.WriteString(t)
				} else {
					body.WriteString(t)
					body.WriteRune('\n')
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, inTitle)
		}
	}
	walk(node, false)
	if strings.TrimSpace(body.String()) == "" {
		return "", "", fmt.Errorf("网页无可提取正文")
	}
	return title.String(), body.String(), nil
}
func (s *Server) importURL(w http.ResponseWriter, r *http.Request, a Actor) error {
	var in struct {
		URL     string `json:"url"`
		Confirm bool   `json:"confirm"`
		Hash    string `json:"hash"`
	}
	if e := decode(r, &in); e != nil {
		return e
	}
	title, body, e := fetchPage(r.Context(), in.URL)
	if e != nil {
		return fail(422, e.Error())
	}
	digest := sha256.Sum256([]byte(body))
	hash := hex.EncodeToString(digest[:])
	if !in.Confirm {
		respond(w, 200, map[string]string{"hash": hash, "title": title, "preview": string([]rune(body)[:min(2500, len([]rune(body)))])})
		return nil
	}
	if in.Hash != hash {
		return fail(409, "网页正文发生变化，请重新预览再确认入库")
	}
	id, e := s.saveDocument(r.Context(), a, title, in.URL, []Page{{1, body}})
	if e != nil {
		return e
	}
	respond(w, 201, map[string]string{"id": id})
	return nil
}
func terms(q string) []string {
	out := []string{}
	for _, word := range strings.FieldsFunc(strings.ToLower(q), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		r := []rune(word)
		if len(r) < 3 {
			out = append(out, word)
		} else {
			for i := 0; i < len(r)-1; i++ {
				out = append(out, string(r[i:i+2]))
			}
		}
	}
	return out
}
func usableTeachingEvidence(body string) bool {
	if len([]rune(strings.TrimSpace(body))) < 80 {
		return false
	}
	lines, noise := 0, 0
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines++
		for _, marker := range []string{"登录后", "立即购买", "下载地址", "点击下载", "开通会员", "扫码购买"} {
			if strings.Contains(line, marker) {
				noise++
				break
			}
		}
	}
	return lines > 0 && noise*2 < lines
}

func (s *Server) retrieve(ctx context.Context, a Actor, q string, kb, web bool, focus ...string) ([]Source, error) {
	selectionQuery := q
	if len(focus) > 0 && strings.TrimSpace(focus[0]) != "" {
		selectionQuery = focus[0]
	}
	out := []Source{}
	if kb {
		tx, e := s.tx(ctx, a.TenantID)
		if e != nil {
			return nil, e
		}
		// ponytail: scan at most 10k tenant chunks; use indexed Chinese search before larger deployments.
		rows, e := queryMaps(ctx, tx, "SELECT c.id,c.document_id,c.content,c.location,d.name,d.source_url FROM chunks c JOIN documents d ON d.id=c.document_id WHERE d.status='ready' ORDER BY c.id LIMIT 10000")
		tx.Rollback(ctx)
		if e != nil {
			return nil, e
		}
		ranked := []Source{}
		for _, r := range rows {
			score := 0.
			for _, term := range relevanceTerms(selectionQuery) {
				if strings.Contains(strings.ToLower(r["content"].(string)), term) {
					score++
				}
			}
			if score > 0 {
				ranked = append(ranked, Source{ID: r["id"].(string), DocumentID: r["document_id"].(string), Title: r["name"].(string), Content: r["content"].(string), Location: r["location"].(string), URL: r["source_url"].(string), Kind: "knowledge", Score: score})
			}
		}
		sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].Score > ranked[j].Score })
		scores := map[string]float64{}
		byID := map[string]Source{}
		for i, x := range ranked {
			scores[x.ID] = 1 / float64(60+i+1)
			byID[x.ID] = x
		}
		if s.cfg.Mode == "live" && s.cfg.EmbeddingModel != "" {
			v, e := s.embed(ctx, a, q)
			if e != nil {
				return nil, e
			}
			tx, e = s.tx(ctx, a.TenantID)
			if e != nil {
				return nil, e
			}
			var vr []map[string]any
			if s.cfg.MilvusURL != "" {
				ids, err := s.vectorIDs(ctx, a.TenantID, v)
				if err != nil {
					tx.Rollback(ctx)
					return nil, err
				}
				vr, e = queryMaps(ctx, tx, "SELECT c.id,c.document_id,c.content,c.location,d.name,d.source_url FROM unnest($1::text[]) WITH ORDINALITY m(id,rank) JOIN chunks c ON c.id=m.id JOIN documents d ON d.id=c.document_id WHERE d.status='ready' ORDER BY m.rank", ids)
			} else {
				vr, e = queryMaps(ctx, tx, "SELECT c.id,c.document_id,c.content,c.location,d.name,d.source_url FROM chunks c JOIN documents d ON d.id=c.document_id WHERE c.embedding IS NOT NULL ORDER BY c.embedding <=> $1::vector LIMIT 24", v)
			}
			tx.Rollback(ctx)
			if e != nil {
				return nil, e
			}
			for i, r := range vr {
				id := r["id"].(string)
				scores[id] += 1 / float64(61+i)
				byID[id] = Source{ID: id, DocumentID: r["document_id"].(string), Title: r["name"].(string), Content: r["content"].(string), Location: r["location"].(string), URL: r["source_url"].(string), Kind: "knowledge"}
			}
		}
		for id, x := range byID {
			x.Score = scores[id]
			selected, filter := relevantPassages(selectionQuery, x.Content, 1800)
			if selected != "" {
				x.Content = selected
				x.Filter = filter
			} else if s.cfg.Mode == "live" && s.cfg.EmbeddingModel != "" {
				// Semantic recall can contain synonyms with no lexical match: keep it visibly unfiltered.
				x.Filter = &PassageFilter{Method: "vector-candidate-unfiltered", Total: 1, Kept: 1}
			} else {
				continue
			}
			out = append(out, x)
		}
		sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
		if len(out) > 8 {
			out = out[:8]
		}
	}
	if web && s.cfg.Mode == "live" {
		if s.cfg.SearchKey == "" {
			return nil, fmt.Errorf("联网搜索未配置")
		}
		queries := []string{q}
		if len(focus) > 0 {
			switch {
			case strings.Contains(q, "历史"):
				queries = append(queries, q+" 关键事件 年代 人物 原始资料", q+" 事件经过 时间线 权威史料")
			case strings.Contains(q, "语文"), strings.Contains(q, "英语"):
				queries = append(queries, q+" 原文 作者 语境", q+" 作品解读 例句 练习")
			default:
				queries = append(queries, q+" 定义 公式 适用条件", q+" 典型例题 实验 常见误区")
			}
		}
		seenURL := map[string]bool{}
		for _, query := range queries {
			b, e := s.postBytes(ctx, s.cfg.SearchURL, map[string]any{"api_key": s.cfg.SearchKey, "query": query, "max_results": 5, "include_raw_content": true}, "")
			if e != nil {
				if len(out) > 0 {
					break
				}
				continue
			}
			var sr struct {
				Results []struct {
					Title, URL, Content string
					RawContent          string `json:"raw_content"`
				} `json:"results"`
			}
			if e = json.Unmarshal(b, &sr); e != nil {
				return nil, e
			}
			for i, x := range sr.Results {
				if i >= 5 {
					break
				}
				u, err := url.Parse(x.URL)
				if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
					continue
				}
				u.Fragment = ""
				for _, key := range []string{"utm_source", "utm_medium", "utm_campaign", "utm_content", "utm_term"} {
					values := u.Query()
					values.Del(key)
					u.RawQuery = values.Encode()
				}
				if seenURL[u.String()] {
					continue
				}
				seenURL[u.String()] = true
				body := x.RawContent
				kind, location := "web", "网页正文"
				var e error
				if strings.TrimSpace(body) != "" {
					kind, location = "web_extracted", "搜索服务提取正文"
				} else {
					_, body, e = fetchPage(ctx, x.URL)
				}
				if e != nil {
					// Search snippets locate candidates; they cannot substantiate teaching claims.
					continue
				}
				if !usableTeachingEvidence(body) {
					continue
				}
				body, selection := relevantPassages(selectionQuery, body, 1800)
				if body != "" {
					out = append(out, Source{Title: x.Title, URL: x.URL, Content: body, Kind: kind, Location: location, Filter: selection})
				}
			}
			if len(out) >= 8 {
				break
			}
		}
	}
	if s.cfg.Mode == "live" && s.cfg.RerankURL != "" && len(out) > 1 {
		texts := []string{}
		for _, src := range out {
			texts = append(texts, src.Content)
		}
		raw, e := s.postBytes(ctx, s.cfg.RerankURL, map[string]any{"model": s.cfg.RerankModel, "query": q, "documents": texts, "top_n": min(10, len(texts))}, s.cfg.RerankKey)
		if e != nil {
			return nil, fmt.Errorf("配置的重排服务调用失败")
		}
		var reply struct {
			Results []struct {
				Index int     `json:"index"`
				Score float64 `json:"relevance_score"`
			} `json:"results"`
		}
		if json.Unmarshal(raw, &reply) != nil || len(reply.Results) == 0 {
			return nil, fmt.Errorf("重排结果无效")
		}
		ranked := []Source{}
		used := map[int]bool{}
		for _, r := range reply.Results {
			if r.Index < 0 || r.Index >= len(out) || used[r.Index] {
				return nil, fmt.Errorf("重排索引无效")
			}
			used[r.Index] = true
			x := out[r.Index]
			x.Score = r.Score
			if x.Filter != nil {
				x.Filter.Method += "+rerank"
			}
			ranked = append(ranked, x)
		}
		out = ranked
	}
	seen := map[string]bool{}
	unique := []Source{}
	for _, x := range out {
		h := sha256.Sum256([]byte(strings.Join(strings.Fields(x.Content), " ")))
		key := hex.EncodeToString(h[:])
		if !seen[key] {
			seen[key] = true
			x.ID = "S" + strconv.Itoa(len(unique)+1)
			x.Hash = key
			x.RetrievedAt = time.Now().UTC().Format(time.RFC3339)
			x.Reliability = "待核验来源"
			if u, err := url.Parse(x.URL); err == nil {
				host := strings.ToLower(u.Hostname())
				if host == "pep.com.cn" || strings.HasSuffix(host, ".pep.com.cn") || strings.HasSuffix(host, ".edu.cn") || strings.HasSuffix(host, ".gov.cn") {
					x.Reliability = "教育或公共机构候选，仍需核验具体结论"
				}
			}
			unique = append(unique, x)
		}
	}
	sort.SliceStable(unique, func(i, j int) bool {
		return strings.HasPrefix(unique[i].Reliability, "教育") && !strings.HasPrefix(unique[j].Reliability, "教育")
	})
	if len(unique) > 12 {
		unique = unique[:12]
	}
	for i := range unique {
		unique[i].ID = "S" + strconv.Itoa(i+1)
	}
	groupSourceOrigins(unique)
	return unique, nil
}

func (s *Server) retrievalTest(w http.ResponseWriter, r *http.Request, a Actor) error {
	var in struct {
		Query string `json:"query"`
		Web   bool   `json:"web"`
	}
	if e := decode(r, &in); e != nil {
		return e
	}
	if strings.TrimSpace(in.Query) == "" || len([]rune(in.Query)) > 300 {
		return fail(400, "请输入1–300字查询")
	}
	v, e := s.retrieve(r.Context(), a, in.Query, true, in.Web)
	if e != nil {
		return e
	}
	respond(w, 200, map[string]any{"sources": v, "mode": s.cfg.Mode, "vector_enabled": s.cfg.Mode == "live" && s.cfg.EmbeddingModel != ""})
	return nil
}
