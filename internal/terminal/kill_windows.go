//go:build windows

package terminal

import (
	"fmt"
	"os/exec"
	"syscall"
)

// killProcessTree kills a process and all its children on Windows.
func killProcessTree(pid int) error {
	kill := exec.Command("taskkill", "/T", "/F", "/PID", fmt.Sprint(pid))
	kill.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	kill.Stdout = nil
	kill.Stderr = nil
	return kill.Run()
}

// stopProcessTree on Windows has no portable graceful termination for console
// processes. Falls back to force kill.
func stopProcessTree(pid int) error {
	return killProcessTree(pid)
}
