//go:build !windows

package terminal

import "os/exec"

func setChildFlags(cmd *exec.Cmd) {
	// No-op on non-Windows platforms.
}
