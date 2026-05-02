// vortex-launcher is a tiny macOS Mach-O binary used as the CFBundleExecutable
// inside Vortex.app. It launches a login shell that execs the installed vortex
// binary, inheriting the user's full environment (PATH, homebrew, nvm, etc.).
//
// If vortex isn't installed yet, it runs the bundled vortex-bootstrap binary
// which handles first-launch download and setup.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func main() {
	home, _ := os.UserHomeDir()
	vortexBin := filepath.Join(home, ".local", "bin", "vortex")

	// If vortex is installed, launch it via a login shell to get full env.
	if isExecutable(vortexBin) {
		launchViaLoginShell(vortexBin)
		return
	}

	// First launch: run the bootstrap binary bundled alongside us.
	selfPath, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}
	bootstrapBin := filepath.Join(filepath.Dir(selfPath), "vortex-bootstrap")
	if isExecutable(bootstrapBin) {
		err = syscall.Exec(bootstrapBin, append([]string{bootstrapBin}, os.Args[1:]...), os.Environ())
		if err != nil {
			os.Exit(1)
		}
	}
	os.Exit(1)
}

// launchViaLoginShell execs $SHELL --login -c 'exec /path/to/vortex [args...]'
// so that the user's shell profile is sourced (PATH, nvm, pyenv, etc.).
func launchViaLoginShell(vortexBin string) {
	// Build the command string for the login shell.
	cmdStr := "exec " + shellQuote(vortexBin)
	for _, arg := range os.Args[1:] {
		cmdStr += " " + shellQuote(arg)
	}

	// Prefer the user's configured shell, then common fallbacks.
	shell := loginShell()
	if shell == "" {
		// No usable shell found — exec vortex directly without login env.
		syscall.Exec(vortexBin, append([]string{vortexBin}, os.Args[1:]...), os.Environ())
		return
	}

	name := filepath.Base(shell)
	err := syscall.Exec(shell, []string{name, "--login", "-c", cmdStr}, os.Environ())
	if err != nil {
		os.Exit(1)
	}
}

// loginShell returns the path to a usable login shell: $SHELL first,
// then zsh and sh as fallbacks.
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

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode()&0111 != 0
}

// shellQuote wraps a string in single quotes for shell safety.
func shellQuote(s string) string {
	// Replace single quotes with '\'' (end quote, escaped quote, start quote).
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
