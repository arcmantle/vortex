//go:build darwin

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// launchVortex execs the installed vortex binary via a login shell so
// the user's full environment (PATH, homebrew, nvm, etc.) is available.
// This is needed because macOS LaunchServices doesn't source shell profiles.
func launchVortex(bin string) {
	cmdStr := "exec " + shellQuote(bin)
	for _, arg := range os.Args[1:] {
		cmdStr += " " + shellQuote(arg)
	}

	shell := loginShell()
	if shell == "" {
		// No usable shell — exec vortex directly.
		syscall.Exec(bin, append([]string{bin}, os.Args[1:]...), os.Environ())
		return
	}

	name := filepath.Base(shell)
	err := syscall.Exec(shell, []string{name, "--login", "-c", cmdStr}, os.Environ())
	if err != nil {
		os.Exit(1)
	}
}

// loginShell returns the path to a usable login shell: $SHELL first,
// then zsh, bash, and sh as fallbacks.
func loginShell() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		if _, err := os.Stat(sh); err == nil {
			return sh
		}
	}
	for _, candidate := range []string{"zsh", "bash", "sh"} {
		if p, err := exec.LookPath(candidate); err == nil {
			return p
		}
	}
	return ""
}

// shellQuote wraps a string in single quotes for shell safety.
func shellQuote(s string) string {
	result := "'"
	for _, c := range s {
		if c == '\'' {
			result += "'\\''"
		} else {
			result += string(c)
		}
	}
	result += "'"
	return result
}
