//go:build !windows && !linux

package main

import (
	"fmt"
	"os/exec"
)

func restrictExportProcess(cmd *exec.Cmd) (func(), error) {
	return nil, fmt.Errorf("restricted video export requires Windows or Linux")
}
