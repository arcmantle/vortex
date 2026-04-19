//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// maybeFork re-launches the process in a new session so the calling terminal
// is freed. Returns (true, nil) if a child was spawned (caller should exit).
// On Windows this returns (false, nil) (handled by -H=windowsgui at build time).
func maybeFork() (bool, error) {
	exe, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("cannot find own executable: %w", err)
	}
	args := append(append([]string(nil), os.Args[1:]...), "--forked")
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return false, fmt.Errorf("failed to fork: %w", err)
	}
	return true, nil
}
