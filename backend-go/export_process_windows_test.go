package main

import (
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsExportPrivilegeBoundary(t *testing.T) {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		t.Fatal(err)
	}
	defer token.Close()
	cmd := exec.Command("node", "-e", "throw new Error('must not start during admission')")
	release, err := restrictExportProcess(cmd)
	if token.IsElevated() {
		if err == nil || err.Error() != "video export requires a non-administrator service account" || release != nil {
			t.Fatal("elevated exports must be rejected")
		}
	} else {
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow {
			t.Fatal("ordinary service exports must use a hidden child process")
		}
	}
	if cmd.Process != nil {
		t.Fatal("admission must not start a process")
	}
}
