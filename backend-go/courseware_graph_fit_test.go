package main

import (
	"context"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestGraphTitleOverflowRetainsAllCoordinates(t *testing.T) {
	if os.Getenv("TEST_RENDER_URL") == "" {
		t.Skip("requires actual PPTX renderer")
	}
	s := &Server{cfg: Config{RenderURL: os.Getenv("TEST_RENDER_URL")}, http: &http.Client{Timeout: 30 * time.Second}}
	title := "描点法：y=2x+1，五点(-2,-3)、(-1,-1)、(0,1)、(1,3)、(2,5)"
	d := Deck{Title: "一次函数", Theme: "ocean", EditableFormulas: true, Slides: []Slide{{Title: "描点法画图", Layout: "steps", Bullets: []string{"列表计算五个点", "描点连线，图像向两边延伸"}, Notes: "完整计算", Formula: "y=2x+1", Focus: "图像是什么形状？", Graph: &FunctionGraph{Title: title, XMin: -4, XMax: 4, YMin: -3, YMax: 5, Series: []FunctionSeries{{K: 2, B: 1, MarkX: []float64{0, 1}}}}}}}
	original := d.Slides[0].Graph
	before := append([]FunctionSeries(nil), original.Series...)
	b, err := s.renderCoursewarePage(context.Background(), &d, 1)
	if err != nil || len(b) < 1000 || string(b[:2]) != "PK" {
		t.Fatal("graph title still blocks actual PPTX", err)
	}
	if !strings.Contains(d.Slides[0].Notes, title) || !reflect.DeepEqual(d.Slides[0].Graph.Series, before) {
		t.Fatal("fit lost coordinates or math")
	}
	if original.Title != title {
		t.Fatal("fit mutated source graph")
	}
}

func TestFunctionComparisonReservesSpaceBeforeModelRepair(t *testing.T) {
	if os.Getenv("TEST_RENDER_URL") == "" {
		t.Skip("requires actual PPTX renderer")
	}
	s := &Server{cfg: Config{RenderURL: os.Getenv("TEST_RENDER_URL")}, http: &http.Client{Timeout: 30 * time.Second}}
	focus := "只看k和b的符号，能确定图像经过的象限吗？"
	d := Deck{Title: "一次函数", Theme: "ocean", EditableFormulas: true, Slides: []Slide{{Title: "象限对比", Layout: "example", Bullets: []string{"y=2x+1：k>0，b>0", "y=-x+2：k<0，b>0"}, Notes: "保留推导与答案", Focus: focus, Formula: `y=kx+b\ (k\neq 0)`, Graph: &FunctionGraph{Title: "象限对比", XMin: -4, XMax: 4, YMin: -3, YMax: 5, Series: []FunctionSeries{{K: 2, B: 1, MarkX: []float64{0, 1}}, {K: -1, B: 2, MarkX: []float64{0, 2}}}}}}}
	b, err := s.renderCoursewarePage(context.Background(), &d, 1)
	if err != nil || len(b) < 1000 {
		t.Fatal("comparison graph still blocked", err)
	}
	if !strings.Contains(d.Slides[0].Notes+d.Slides[0].Focus+strings.Join(d.Slides[0].Bullets, " "), focus) || len(d.Slides[0].Graph.Series) != 2 || d.Slides[0].Formula != `y=kx+b\ (k\neq 0)` {
		t.Fatal("fit removed evidence")
	}
}
