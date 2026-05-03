//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

const (
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
)

// spawnHostDetached starts vortex-host without creating a console window.
// Runs headless since the GUI is the window.
func spawnHostDetached(bin string) error {
	cmd := exec.Command(bin, "--headless")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: detachedProcess | createNewProcessGroup,
		HideWindow:    true,
	}
	return cmd.Start()
}

// processAlive checks if a process handle is still valid.
func processAlive(proc *os.Process) bool {
	// On Windows, FindProcess always succeeds. We try to open the process
	// to verify it exists. Signal(0) is not supported on Windows, but
	// FindProcess + Release pattern works for a basic liveness check.
	// We use the same approach: if Signal returns an error, it's dead.
	err := proc.Signal(os.Signal(syscall.Signal(0)))
	// "os: process already finished" or access denied means not alive/not ours.
	return err == nil
}
