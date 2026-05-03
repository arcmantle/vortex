//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// spawnHostDetached starts vortex-host in a new session so it survives
// after the GUI exits. Runs headless since the GUI is the window.
func spawnHostDetached(bin string) error {
	cmd := exec.Command(bin, "--headless")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}

// processAlive checks if a process is still running via signal 0.
func processAlive(proc *os.Process) bool {
	err := proc.Signal(syscall.Signal(0))
	return err == nil
}
