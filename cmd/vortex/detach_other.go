//go:build !windows

package main

import (
	"log"
	"os"
	"os/exec"
	"syscall"
)

func relaunchDetached() {
	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("cannot find own executable: %v", err)
	}

	args := append(os.Args[1:], "--detached")
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
	if err := cmd.Start(); err != nil {
		log.Fatalf("failed to relaunch detached: %v", err)
	}
}
