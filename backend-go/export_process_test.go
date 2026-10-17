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
	script := "process.stdout.write('restricted-ready')"
	if runtime.GOOS == "linux" {
		script = `const status=require('fs').readFileSync('/proc/self/status','utf8');
for (const key of ['CapInh','CapPrm','CapEff','CapAmb']) {
 const value=status.match(new RegExp('^'+key+':\\s*(\\w+)', 'm'));
 if (!value || !/^0+$/.test(value[1])) throw new Error(key+' must be empty');
}
if (!/^NoNewPrivs:\s*1$/m.test(status)) throw new Error('no-new-privs missing');
process.stdout.write('restricted-ready');`
	}
	cmd := exec.CommandContext(ctx, "node", "-e", script)
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
