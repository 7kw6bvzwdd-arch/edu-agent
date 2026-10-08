package main

import (
	"encoding/json"
	"math"
	"testing"
)

func TestAnimationPlanGeometryAndBoundaries(t *testing.T) {
	base := sampleDeck(TaskInput{Type: "animation", Topic: "勾股定理", Pages: 8, Theme: "forest"})
	if err := validateAnimationPlan(base, nil, 0, false); err != nil {
		t.Fatal(err)
	}
	for _, angle := range []float64{0, 30, 60, 90, 180} {
		el := constructedElement(AnimationElement{Kind: "right_triangle", Points: [][2]float64{{0, 0}}, Size: [2]float64{1, 2}, Angle: angle})
		if !convexPolygon(el.Points) || math.Abs(polygonArea(el.Points)-1) > 1e-6 {
			t.Fatal("parameterized triangle changed area", angle)
		}
	}
	var malformed AnimationElement
	if json.Unmarshal([]byte(`{"points":[[1,2,3]]}`), &malformed) == nil {
		t.Fatal("extra coordinate silently discarded")
	}
	if convexPolygon([][2]float64{{0, 0}, {1, 0}, {0, 1}, {0, 0}, {1, 0}, {0, 1}}) {
		t.Fatal("repeated boundary inflated area")
	}
	for name, change := range map[string]func(*Deck){
		"missing scene":          func(d *Deck) { d.Slides[0].Scene = nil },
		"outside after movement": func(d *Deck) { d.Slides[0].Scene.Elements[0].Move = [2]float64{10, 0} },
		"fabricated citation":    func(d *Deck) { d.Slides[0].Scene.SourceIDs = []string{"S999"} },
		"flashing scene":         func(d *Deck) { d.Slides[0].Scene.Duration = .1 },
		"unverified proof":       func(d *Deck) { d.Slides[0].Scene.Evidence = "proof"; d.Slides[0].Scene.Checks = nil },
		"wrong right angle": func(d *Deck) {
			d.Slides[0].Scene.Elements = []AnimationElement{{ID: "t", Kind: "polygon", Color: "blue", Points: [][2]float64{{0, 0}, {2, 0}, {1, 1}}}}
			d.Slides[0].Scene.Elements[0].Points[2] = [2]float64{1, 1.5}
			d.Slides[0].Scene.Checks = []GeometryCheck{{Kind: "right_angle", Objects: []string{"t"}}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			var d Deck
			json.Unmarshal(asJSON(base), &d)
			change(&d)
			if validateAnimationPlan(d, nil, 0, false) == nil {
				t.Fatal("invalid plan accepted")
			}
		})
	}
	box := AnimationElement{Kind: "polygon", Points: [][2]float64{{0, 0}, {2, 0}, {2, 2}, {0, 2}}}
	left := AnimationElement{Kind: "polygon", Points: [][2]float64{{0, 0}, {1, 0}, {1, 2}, {0, 2}}}
	right := AnimationElement{Kind: "polygon", Points: [][2]float64{{1, 0}, {2, 0}, {2, 2}, {1, 2}}}
	if !validPartition([]AnimationElement{box, left, right}) {
		t.Fatal("valid partition rejected")
	}
	if validPartition([]AnimationElement{box, left, left}) {
		t.Fatal("overlap with equal area falsely proves partition")
	}
	right.Points[1][0] = 3
	if validPartition([]AnimationElement{box, left, right}) {
		t.Fatal("outside piece accepted")
	}
}
