package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	pngcodec "image/png"
	"strings"
	"sync"
	"time"
)

type coursewareNextPageKey struct{}
type coursewareAbortNextKey struct{}
type coursewarePrefetchKey struct{}
type prefetchedPage struct {
	page     int
	position int
	key      string
	cancel   context.CancelFunc
	done     chan struct{}
	deck     Deck
	err      error
	used     bool
}

func (p *prefetchedPage) stop() {
	if p != nil {
		p.cancel()
		<-p.done
	}
}
func coursewareRequestKey(in TaskInput, sources []Source, instructions string, start int) string {
	return fmt.Sprintf("%x", sha256.Sum256(asJSON([]any{in, sources, instructions, start})))
}

type coursewareArtifactsKey struct{}
type coursewareArtifacts struct {
	mu      sync.Mutex
	files   map[string][]byte
	order   []string
	bytes   int
	metrics map[string]coursewareTiming
}
type coursewareTiming struct {
	Calls        int   `json:"calls"`
	Milliseconds int64 `json:"milliseconds"`
}

func coursewareMeasure(ctx context.Context, stage string) func() {
	started := time.Now()
	return func() {
		c, _ := ctx.Value(coursewareArtifactsKey{}).(*coursewareArtifacts)
		if c == nil {
			return
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		v := c.metrics[stage]
		v.Calls++
		v.Milliseconds += time.Since(started).Milliseconds()
		c.metrics[stage] = v
	}
}
func coursewareTimingReport(ctx context.Context) string {
	c, _ := ctx.Value(coursewareArtifactsKey{}).(*coursewareArtifacts)
	if c == nil {
		return "{}"
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return string(asJSON(c.metrics))
}

// One generation/edit request owns its cache; never shared across members.
func withCoursewareArtifacts(ctx context.Context) context.Context {
	if ctx.Value(coursewareArtifactsKey{}) != nil {
		return ctx
	}
	return context.WithValue(ctx, coursewareArtifactsKey{}, &coursewareArtifacts{files: map[string][]byte{}, metrics: map[string]coursewareTiming{}})
}
func artifactCacheGet(ctx context.Context, key string) []byte {
	c, _ := ctx.Value(coursewareArtifactsKey{}).(*coursewareArtifacts)
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.files[key]
}
func artifactCachePut(ctx context.Context, key string, b []byte) {
	c, _ := ctx.Value(coursewareArtifactsKey{}).(*coursewareArtifacts)
	if c == nil || len(b) > 64<<20 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.files[key]; ok {
		return
	}
	// A bounded request cache expires with the worker stage / HTTP request.
	for len(c.order) > 0 && (len(c.order) >= 100 || c.bytes+len(b) > 64<<20) {
		old := c.order[0]
		c.order = c.order[1:]
		c.bytes -= len(c.files[old])
		delete(c.files, old)
	}
	c.files[key] = b
	c.order = append(c.order, key)
	c.bytes += len(b)
}
func coursewareRenderKey(d Deck) string {
	// Only the renderer's actual inputs matter, including notes and answer.
	slides := append([]Slide(nil), d.Slides...)
	for i := range slides {
		slides[i].ID = ""
		slides[i].Order = 0
	}
	d = Deck{Title: d.Title, Theme: d.Theme, Slides: slides, EditableFormulas: d.EditableFormulas, NeedsReview: d.NeedsReview}
	return fmt.Sprintf("pptx:%x", sha256.Sum256(asJSON(d)))
}

// Trusted renderer facts complement model-supplied content, not replace image review.
func teachingRenderEvidence(d Deck) string {
	var out strings.Builder
	for _, sl := range d.Slides {
		if sl.Graph == nil {
			continue
		}
		g := sl.Graph
		out.WriteString("\n可信绘图规则：本页函数图由程序根据graph参数计算，x/y每1单位采用同一物理长度；轴上有数值刻度。范围仅限定可见窗口，不限定函数定义域。数学图无量纲时不要求虚构物理单位；物理量单位仍须核对正文和图标题。图例给出函数式及标记点坐标，mark_x只决定重点标记，不影响整条函数曲线。图例由graph.series生成，与单独的formula公式色带是不同对象；图例可用分隔符附带标记点坐标，不能把整段图例当formula字段读值。系数1显写或省略、零项省略属于等价写法，只作格式建议，不是数学内容差异；实际系数、正负号、次数或坐标错误仍须阻塞。\n")
		for _, series := range g.Series {
			fmt.Fprintf(&out, "曲线参数kind=%s k=%g b=%g a=%g c=%g；", series.Kind, series.K, series.B, series.A, series.C)
			for _, x := range series.MarkX {
				y := series.K*x + series.B
				switch series.Kind {
				case "quadratic":
					y = series.A*x*x + series.B*x + series.C
				case "inverse":
					if x == 0 {
						continue
					}
					y = series.K / x
				}
				fmt.Fprintf(&out, "重点标记(%g,%g)；", x, y)
			}
		}
	}
	return out.String()
}

// Magnify the actual renderer's graph panel, retaining the full-page image.
// This gives the reviewer legible ticks/points without another Office export.
func coursewareGraphDetail(d Deck, png []byte) []byte {
	if len(d.Slides) != 1 || d.Slides[0].Graph == nil {
		return nil
	}
	if d.Slides[0].Layout == "" {
		return nil
	} // Unknown inferred layout: full image remains available.
	source, err := pngcodec.Decode(bytes.NewReader(png))
	if err != nil {
		return nil
	}
	b := source.Bounds()
	if b.Dx() > 4000 || b.Dy() > 4000 {
		return nil
	}
	left, top := b.Min.X+b.Dx()*47/100, b.Min.Y+b.Dy()*30/100
	right, bottom := b.Min.X+b.Dx()*96/100, b.Min.Y+b.Dy()*82/100
	switch d.Slides[0].Layout {
	case "concept", "comparison", "summary":
		left, right = b.Min.X+b.Dx()*5/100, b.Min.X+b.Dx()*60/100
	}
	detail := image.NewRGBA(image.Rect(0, 0, (right-left)*2, (bottom-top)*2))
	for y := 0; y < detail.Bounds().Dy(); y++ {
		for x := 0; x < detail.Bounds().Dx(); x++ {
			detail.Set(x, y, source.At(left+x/2, top+y/2))
		}
	}
	var out bytes.Buffer
	if pngcodec.Encode(&out, detail) != nil {
		return nil
	}
	return out.Bytes()
}
