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
		fallbackErr := syscall.Kill(pid, syscall.SIGKILL)
		if fallbackErr == nil || errors.Is(fallbackErr, syscall.ESRCH) {
			return nil
		}
		return fallbackErr
	}
	return err
}

// stopProcessTree sends SIGTERM to the process group for graceful shutdown.
func stopProcessTree(pid int) error {
	err := syscall.Kill(-pid, syscall.SIGTERM)
	if err == nil {
		return nil
	}
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
