//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"arcmantle/vortex/internal/release"
)

// runUninstall removes binaries and the .app bundle on non-Windows platforms.
func runUninstall() {
	installDir, err := release.ManagedInstallDir()
	if err != nil {
		showError(fmt.Sprintf("resolve install directory: %v", err))
		return
	}

	// Remove binaries.
	for _, name := range []string{release.ManagedHostBinaryName(), release.ManagedGUIBinaryName()} {
		path := filepath.Join(installDir, name)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "warning: remove %s: %v\n", path, err)
		}
	}

	// Remove /Applications/Vortex.app if it exists.
	appPath := "/Applications/Vortex.app"
	if _, err := os.Stat(appPath); err == nil {
		os.RemoveAll(appPath)
	}

	// Check for --remove-config flag.
	removeConfig := false
	for _, arg := range os.Args[1:] {
		if arg == "--remove-config" {
			removeConfig = true
		}
	}
	if removeConfig {
		home, _ := os.UserHomeDir()
		if home != "" {
			os.RemoveAll(filepath.Join(home, ".config", "vortex"))
		}
	}

	home, _ := os.UserHomeDir()
	for _, path := range webviewDataCleanupTargetsForGOOS(runtime.GOOS, home) {
		if err := os.RemoveAll(path); err != nil {
			fmt.Fprintf(os.Stderr, "warning: remove %s: %v\n", path, err)
		}
	}

	fmt.Println("Vortex has been uninstalled.")
}
