//go:build !windows

package terminal

import (
	"errors"
	"syscall"
)

// killProcessTree kills a process and all its children via process group.
func killProcessTree(pid int) error {
	err := syscall.Kill(-pid, syscall.SIGKILL)
	if err == nil {
		return nil
	}
	if errors.Is(err, syscall.ESRCH) {
		if fallbackErr := syscall.Kill(pid, syscall.SIGKILL); fallbackErr == nil || errors.Is(fallbackErr, syscall.ESRCH) {
			return nil
		}
	}
	return err
}
