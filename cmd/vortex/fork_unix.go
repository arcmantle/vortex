//go:build !windows

package main

import (
	"log"
	"os"
	"os/exec"
	"syscall"
)

// maybeFork re-launches the process in a new session so the calling terminal
// is freed. Returns true if a child was spawned (caller should exit).
// On Windows this returns false (handled by -H=windowsgui at build time).
func maybeFork() bool {
	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("cannot find own executable: %v", err)
	}
	args := append(os.Args[1:], "--forked")
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		log.Fatalf("failed to fork: %v", err)
	}
	return true
}
