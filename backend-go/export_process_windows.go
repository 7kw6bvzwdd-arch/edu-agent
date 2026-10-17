//go:build windows

package main

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os/exec"
	"syscall"
)

// Export only from a non-elevated service identity. A filtered token cannot
// regain administrator access in descendants. This is not a filesystem sandbox.
func restrictExportProcess(cmd *exec.Cmd) (func(), error) {
	var current windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &current); err != nil {
		return nil, err
	}
	defer current.Close()
	if current.IsElevated() {
		return nil, fmt.Errorf("video export requires a non-administrator service account")
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return func() {}, nil
}
