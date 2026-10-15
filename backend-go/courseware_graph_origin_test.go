package main

import "testing"

func TestFunctionGraphIncludesOriginOnAxisBoundary(t *testing.T) {
	for _, bounds := range [][4]float64{{0, 5, 0, 12}, {-5, 0, -12, 0}} {
		sl := Slide{Graph: &FunctionGraph{Title: "按题设计算，非实测", XMin: bounds[0], XMax: bounds[1], YMin: bounds[2], YMax: bounds[3], Series: []FunctionSeries{{Kind: "linear", K: 2, B: 0, MarkX: []float64{0}}}}}
		if err := validateTeachingMedia(sl); err != nil {
			t.Fatal("valid graph contains (0,0) on boundary", err)
		}
	}
	for _, bounds := range [][4]float64{{1, 5, 0, 12}, {0, 5, 1, 12}, {0, 0, 0, 12}} {
		sl := Slide{Graph: &FunctionGraph{Title: "无效范围", XMin: bounds[0], XMax: bounds[1], YMin: bounds[2], YMax: bounds[3], Series: []FunctionSeries{{Kind: "linear", K: 2}}}}
		if validateTeachingMedia(sl) == nil {
			t.Fatal("missing origin or zero span passed", bounds)
		}
	}
}
