package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"arcmantle/vortex/internal/instance"
)

const (
	hostInstanceName = "vortex"
	hostBinaryName   = "vortex-host"
	pollInterval     = 100 * time.Millisecond
	pollTimeout      = 5 * time.Second
)

// selfLaunchURL ensures a bare-mode host is running and returns the URL
// (with session token) for the webview to connect to.
func selfLaunchURL() (string, error) {
	// Check if the host is already running via the instance registry.
	meta, err := instance.GetMetadata(hostInstanceName)
	if err == nil && meta.PID > 0 && !isProcessDead(meta.PID) {
		url := fmt.Sprintf("http://127.0.0.1:%d?token=%s", meta.HTTPPort, meta.Token)
		log.Printf("host already running (pid %d), connecting to %s", meta.PID, sanitizeURL(meta.HTTPPort))
		return url, nil
	}

	// If metadata exists but process is dead, clean it up.
	if err == nil && meta.PID > 0 {
		log.Printf("stale host metadata (pid %d dead), cleaning up", meta.PID)
		_ = instance.RemoveMetadata(hostInstanceName)
	}

	// Spawn the host.
	bin, err := resolveHostBinary()
	if err != nil {
		return "", fmt.Errorf("cannot find %s: %w", hostBinaryName, err)
	}

	log.Printf("spawning host: %s", bin)
	if err := spawnHostDetached(bin); err != nil {
		return "", fmt.Errorf("failed to spawn host: %w", err)
	}

	// Poll the registry until the host registers.
	deadline := time.Now().Add(pollTimeout)
	for time.Now().Before(deadline) {
		time.Sleep(pollInterval)
		meta, err = instance.GetMetadata(hostInstanceName)
		if err != nil {
			continue
		}
		if meta.Token == "" || meta.HTTPPort == 0 {
			continue
		}
		url := fmt.Sprintf("http://127.0.0.1:%d?token=%s", meta.HTTPPort, meta.Token)
		log.Printf("host ready (pid %d), connecting to %s", meta.PID, sanitizeURL(meta.HTTPPort))
		return url, nil
	}

	return "", fmt.Errorf("host did not become ready within %s", pollTimeout)
}

// resolveHostBinary locates the installed host binary next to this executable,
// then falls back to PATH.
func resolveHostBinary() (string, error) {
	candidates := []string{hostBinaryName}
	if runtime.GOOS == "windows" {
		candidates = []string{"vortex", hostBinaryName}
	}

	self, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(self)
		for _, base := range candidates {
			candidate := filepath.Join(dir, base)
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
			candidate += ".exe"
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
	}
	for _, base := range candidates {
		if candidate, err := exec.LookPath(base); err == nil || errors.Is(err, exec.ErrDot) {
			if filepath.IsAbs(candidate) {
				return candidate, nil
			}
			if abs, absErr := filepath.Abs(candidate); absErr == nil {
				return abs, nil
			}
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no host binary found next to %s or in PATH (tried %s)", self, strings.Join(candidates, ", "))
}

// sanitizeURL returns a log-safe version of the URL (no token).
func sanitizeURL(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

// isProcessDead checks if a PID is no longer running.
func isProcessDead(pid int) bool {
	if pid <= 0 {
		return true
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return true
	}
	return !processAlive(proc)
}
