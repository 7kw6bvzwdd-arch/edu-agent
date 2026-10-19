package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Render boundary: the same author data is packaged for offline playback and MP4.
func TestFunctionFamiliesRenderPackage(t *testing.T) {
	out := filepath.Join(t.TempDir(), "functions.html")
	if err := animPackageCommand(context.Background(), "../fixtures/animation-v2-functions.json", out, animVoiceTone); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil || len(b) < 10000 {
		t.Fatal("missing offline package", err)
	}
}

func TestFunctionFamiliesRejectInvalidPackage(t *testing.T) {
	for _, kind := range []string{"quadratic-zero", "inverse-zero", "inverse-point"} {
		t.Run(kind, func(t *testing.T) {
			raw, err := os.ReadFile("../fixtures/animation-v2-functions.json")
			if err != nil {
				t.Fatal(err)
			}
			var sc AnimScript
			if err = json.Unmarshal(raw, &sc); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "quadratic-zero":
				sc.Scenes[0].Actions[0].To = f64(0)
			case "inverse-zero":
				sc.Scenes[1].Actions[0].To = f64(-2)
			case "inverse-point":
				sc.Scenes[1].Objects = append(sc.Scenes[1].Objects, AnimObject{ID: "p", Type: "point", Slot: "main", Line: "f", Params: map[string]float64{"x": 1}})
				sc.Scenes[1].Actions = append(sc.Scenes[1].Actions, AnimAction{ID: "point-change", Op: "tween", Object: "p", Param: "x", To: f64(-1), At: AnimTrigger{Scene: "start"}})
			}
			dir := t.TempDir()
			source, out := filepath.Join(dir, "script.json"), filepath.Join(dir, "out.html")
			if err = os.WriteFile(source, asJSON(sc), 0600); err != nil {
				t.Fatal(err)
			}
			if err = animPackageCommand(context.Background(), source, out, animVoiceTone); err == nil {
				t.Fatal("invalid teaching script produced a package")
			}
			if _, err = os.Stat(out); !os.IsNotExist(err) {
				t.Fatal("rejected script left an output artifact")
			}
		})
	}
}
