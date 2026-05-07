//go:build windows

package uninstall

import (
	"os"
	"path/filepath"

	"arcmantle/vortex/internal/release"
)

// AllRemovalPaths returns every path that should be removed for a full
// uninstall on Windows. This is used by SpawnCleanupHelper so the detached
// process has a flat list to iterate.
func AllRemovalPaths(opts Options) []string {
	var paths []string

	// Host binaries and aliases.
	for _, name := range hostBinaryNames() {
		paths = append(paths, filepath.Join(opts.InstallDir, name))
	}

	// GUI binary and directory.
	if opts.GUIInstallDir != "" {
		paths = append(paths, filepath.Join(opts.GUIInstallDir, release.ManagedGUIBinaryName()))
		paths = append(paths, opts.GUIInstallDir)
	}

	// Start Menu shortcut.
	appData := os.Getenv("APPDATA")
	if appData != "" {
		paths = append(paths, filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Vortex.lnk"))
	}

	// Webview data.
	paths = append(paths, WebviewCachePaths()...)

	// Config.
	if opts.RemoveConfig && appData != "" {
		paths = append(paths, filepath.Join(appData, "Vortex"))
	}

	// Install directory itself (should be empty after everything else).
	paths = append(paths, opts.InstallDir)

	return paths
}
