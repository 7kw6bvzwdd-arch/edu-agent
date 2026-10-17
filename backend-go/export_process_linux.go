//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
)

func restrictExportProcess(cmd *exec.Cmd) (func(), error) {
	if os.Geteuid() == 0 {
		return nil, fmt.Errorf("video export requires a non-root service account")
	}
	setpriv, err := exec.LookPath("setpriv")
	if err != nil {
		return nil, fmt.Errorf("setpriv is required for restricted video export")
	}
	// Dropping the bounding set requires CAP_SETPCAP, which ordinary service
	// accounts do not have. no-new-privs prevents file capabilities/setuid from
	// granting privileges; clear inheritable and ambient sets before exec.
	cmd.Args = append([]string{setpriv, "--no-new-privs", "--inh-caps=-all", "--ambient-caps=-all", "--"}, cmd.Args...)
	cmd.Path = setpriv
	return func() {}, nil
}
