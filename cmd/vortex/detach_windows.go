//go:build windows

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
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x00000008, // DETACHED_PROCESS
	}
	if err := cmd.Start(); err != nil {
		log.Fatalf("failed to relaunch detached: %v", err)
	}
}
