//go:build !windows

package terminal

import (
	"os/exec"
	"syscall"
)

func setChildFlags(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
