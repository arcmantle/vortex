//go:build !windows

package terminal

import "syscall"

// killProcessTree kills a process and all its children via process group.
func killProcessTree(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}
