//go:build !darwin

package main

import (
	"os"
	"os/exec"
)

// launchVortex starts vortex as a child process (non-macOS platforms).
func launchVortex(bin string) {
	cmd := exec.Command(bin)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Start()
}
