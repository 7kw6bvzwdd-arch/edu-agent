package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScienceOfflinePackage(t *testing.T) {
	url := os.Getenv("SCIENCE_RENDER_URL")
	if url == "" {
		t.Skip("requires real scientific renderer")
	}
	t.Setenv("RENDER_SERVICE_URL", url)
	out := filepath.Join(t.TempDir(), "science.html")
	if err := animPackageCommand(context.Background(), "../fixtures/animation-v2-science.json", out, animVoiceTone); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"science-visuals-v1", "data:image/png;base64,", "测试音", "固定样例"} {
		if !strings.Contains(string(raw), text) {
			t.Fatal("offline package missing " + text)
		}
	}
}
