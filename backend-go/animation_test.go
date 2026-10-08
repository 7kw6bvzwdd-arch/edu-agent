package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManimPreflightCollectsHistoricalErrors(t *testing.T) {
	if os.Getenv("TEST_VIDEO_SANDBOX") != "1" {
		t.Skip("requires local Docker sandbox")
	}
	s := &Server{cfg: Config{Storage: t.TempDir(), AnimationImage: "edu-manim:local"}}
	script := `from manim import *
class Lesson(Scene):
    def construct(self):
        self.camera.background = Rectangle()
        Dot(opacity=0.7)
        VGroup(*[Line(r, r+RIGHT*0.2) for r in [LEFT, 0, RIGHT]])
        self.wait(0)
        Dot().move_to(ORIGINAL)
`
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, err := s.runAnimation(ctx, 1, newID(), script, 0)
	if err == nil {
		t.Fatal("invalid script accepted")
	}
	for _, want := range []string{"预检", "background_color", "opacity", "三维坐标", "正数", "ORIGIN"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
}

func TestManimRendersCorrectedHistoricalAPIs(t *testing.T) {
	if os.Getenv("TEST_VIDEO_SANDBOX") != "1" {
		t.Skip("requires local Docker sandbox")
	}
	s := &Server{cfg: Config{Storage: t.TempDir(), AnimationImage: "edu-manim:local"}}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	script := `from manim import *
class Lesson(Scene):
    def construct(self):
        self.camera.background_color = '#182e29'
        dot = Dot(fill_opacity=0.7).move_to(ORIGIN)
        wires = VGroup(*[Line(r, r+RIGHT*0.2) for r in [LEFT, ORIGIN, RIGHT]])
        title = Text('欧姆定律 I = U / R', font='Noto Sans CJK SC', font_size=30)
        self.play(FadeIn(dot, wires, title), run_time=1)
        self.wait(20)
`
	path, err := s.runAnimation(ctx, 1, newID(), script, 0)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() < 1000 {
		t.Fatal("missing playable artifact", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	start := bytes.Index(content, []byte("mdat"))
	if start < 0 {
		t.Fatal("missing media frames")
	}
	clear(content[start+4:])
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Lesson.mp4"), content, 0600); err != nil {
		t.Fatal(err)
	}
	if s.inspectAnimation(ctx, dir) == nil {
		t.Fatal("corrupt frames passed delivery checks")
	}
}

func TestManimMediaContract(t *testing.T) {
	good := "{\"streams\":[{\"codec_type\":\"video\",\"codec_name\":\"h264\",\"width\":1280,\"height\":720}],\"format\":{\"duration\":\"21.0\"}}"
	if err := validateAnimationMetadata([]byte(good)); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		strings.ReplaceAll(good, "21.0", "NaN"), strings.ReplaceAll(good, "21.0", "Inf"),
		strings.ReplaceAll(good, "21.0", "12"), strings.ReplaceAll(good, "21.0", "47"),
		strings.ReplaceAll(good, "h264", "vp9"), strings.ReplaceAll(good, "video", "audio"),
		strings.ReplaceAll(good, "720", "480"), "{}",
	} {
		if validateAnimationMetadata([]byte(bad)) == nil {
			t.Fatal("invalid media accepted", bad)
		}
	}
}

func TestControlledAnimationAcceptanceHTTP(t *testing.T) {
	if os.Getenv("TEST_VIDEO_SANDBOX") != "1" {
		t.Skip("requires Docker")
	}
	s := worksServer(t)
	s.cfg.AnimationImage = "edu-manim:local"
	token := worksLogin(t, s, "teacher")
	other := worksLogin(t, s, "other")
	var actor Actor
	json.Unmarshal(worksCall(t, s, "GET", "/api/auth/me", token, nil, 200).Body.Bytes(), &actor)
	in := TaskInput{Type: "animation", AnimationEngine: "manim", Topic: "浮力", Pages: 8, Theme: "forest", UseKB: true, IdempotencyKey: newID()}
	id := reservedVideoFixture(t, s, token, actor, in)
	before := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))["balance"]
	d := sampleDeck(in)
	d.Script = "raise Exception('untrusted script must never execute')"
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	if err := s.renderAnimation(ctx, actor, id, d); err != nil {
		t.Fatal(err)
	}
	v := worksObject(t, worksCall(t, s, "GET", "/api/tasks/"+id, token, nil, 200))
	if v["status"] != "awaiting_acceptance" || v["charge_state"] != "reserved" {
		t.Fatal("rendering bypassed quality gate", v)
	}
	if after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))["balance"]; after != before {
		t.Fatal("charged before acceptance")
	}
	worksCall(t, s, "GET", "/api/tasks/"+id+"/download/mp4", token, nil, 200)
	worksCall(t, s, "GET", "/api/tasks/"+id+"/frames/0", token, nil, 200)
	worksCall(t, s, "GET", "/api/tasks/"+id+"/frames/0", other, nil, 404)
	worksCall(t, s, "POST", "/api/tasks/"+id+"/accept-animation", other, map[string]any{"revision": v["revision"], "accept": true}, 404)
	worksCall(t, s, "POST", "/api/tasks/"+id+"/accept-animation", token, map[string]any{"revision": v["revision"], "accept": false}, 400)
	worksCall(t, s, "POST", "/api/tasks/"+id+"/accept-animation", token, map[string]any{"revision": -1, "accept": true}, 409)
	worksCall(t, s, "POST", "/api/tasks/"+id+"/accept-animation", token, map[string]any{"revision": v["revision"], "accept": true}, 200)
	worksCall(t, s, "POST", "/api/tasks/"+id+"/accept-animation", token, map[string]any{"revision": v["revision"], "accept": true}, 409)
	after := worksObject(t, worksCall(t, s, "GET", "/api/billing", token, nil, 200))["balance"].(float64)
	if after != before.(float64)-20 {
		t.Fatal("fixture reservation settled incorrectly", after, before)
	}
	worksCall(t, s, "DELETE", "/api/tasks/"+id, token, nil, 200)
	worksCall(t, s, "GET", "/api/tasks/"+id+"/frames/0", token, nil, 404)
	if err := s.renderAnimation(ctx, actor, id, d); err == nil {
		t.Fatal("deleted task rerendered")
	}
}
