//go:build windows

package terminal

import (
	"os/exec"
	"syscall"
)

// setChildFlags prevents child console processes from spawning a visible
// console window when vortex itself runs detached (no console).
func setChildFlags(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
