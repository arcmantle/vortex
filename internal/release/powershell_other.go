//go:build !windows

package release

import "os/exec"

func configureBackgroundCommand(_ *exec.Cmd) {}