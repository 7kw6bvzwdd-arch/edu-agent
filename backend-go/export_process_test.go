package main

import (
	"context"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestRestrictedExportProcess(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skip("supported export platforms only")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "-e", "process.stdout.write('restricted-ready')")
	release, err := restrictExportProcess(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	out, err := cmd.CombinedOutput()
	if err != nil || string(out) != "restricted-ready" {
		t.Fatalf("restricted OS process failed: %v: %s", err, out)
	}
}
