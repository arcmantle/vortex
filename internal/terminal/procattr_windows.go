//go:build windows

package terminal

import (
	"os/exec"
	"syscall"
)

// setChildFlags configures process creation flags for child processes on
// Windows. Note: when using ConPTY, createConPTYProcess merges these flags
// with EXTENDED_STARTUPINFO_PRESENT. CREATE_NO_WINDOW is intentionally NOT
// set here because it conflicts with ConPTY's pseudo console allocation.
// The pseudo console itself ensures no visible console window appears.
func setChildFlags(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{}
}
