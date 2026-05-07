// Package uninstall provides shared logic for removing Vortex from a system.
// Both cmd/vortex and cmd/vortex-setup use this package to avoid divergent
// uninstall paths.
package uninstall

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"arcmantle/vortex/internal/release"
)

// Options configures what to remove during an uninstall.
type Options struct {
	InstallDir    string
	GUIInstallDir string
	RemoveConfig  bool
}

// Remove performs the uninstall for the current platform. On Unix, it removes
// everything directly. On Windows, it removes what it can (the running binary
// cannot be deleted) and returns nil; use SpawnCleanupHelper for post-exit
// cleanup of the caller's own binary and install directory.
func Remove(opts Options) error {
	// Remove host binary and alias (symlink on Unix, hardlink on Windows).
	for _, name := range hostBinaryNames() {
		path := filepath.Join(opts.InstallDir, name)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "warning: remove %s: %v\n", path, err)
		}
	}

	// Remove GUI binary from its separate directory.
	if opts.GUIInstallDir != "" {
		guiPath := filepath.Join(opts.GUIInstallDir, release.ManagedGUIBinaryName())
		if err := os.Remove(guiPath); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "warning: remove %s: %v\n", guiPath, err)
		}
		os.Remove(opts.GUIInstallDir) // remove dir if empty
	}

	// macOS: remove the .app bundle.
	if runtime.GOOS == "darwin" {
		appPath := "/Applications/Vortex.app"
		if _, err := os.Stat(appPath); err == nil {
			if err := os.RemoveAll(appPath); err != nil {
				fmt.Fprintf(os.Stderr, "warning: remove %s: %v\n", appPath, err)
			}
		}
	}

	// Platform-specific cleanup (registry, shortcuts, etc).
	removePlatformArtifacts(opts)

	// Remove configuration if requested.
	if opts.RemoveConfig {
		removeConfig()
	}

	// Remove webview/cache data.
	for _, path := range WebviewCachePaths() {
		if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "warning: remove %s: %v\n", path, err)
		}
	}

	// Try to remove install directory if empty.
	os.Remove(opts.InstallDir)

	return nil
}

// hostBinaryNames returns the binary names to remove from the install directory.
func hostBinaryNames() []string {
	return []string{
		release.ManagedHostBinaryName(),
		release.ManagedHostSymlinkName(),
	}
}

// removeConfig removes user configuration files.
func removeConfig() {
	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData != "" {
			os.RemoveAll(filepath.Join(appData, "Vortex"))
		}
	default:
		home, _ := os.UserHomeDir()
		if home != "" {
			os.RemoveAll(filepath.Join(home, ".config", "vortex"))
		}
	}
}
