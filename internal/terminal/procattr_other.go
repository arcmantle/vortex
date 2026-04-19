//go:build !windows

package terminal

import "os/exec"

// setChildFlags is a no-op on Unix when using a PTY because forkpty() already
// calls setsid(), which creates both a new session and a new process group.
// Setting Setpgid on top of that causes EPERM on macOS for binaries with
// com.apple.provenance (e.g. node installed via version managers).
func setChildFlags(cmd *exec.Cmd) {
	_ = cmd // intentionally empty
}
