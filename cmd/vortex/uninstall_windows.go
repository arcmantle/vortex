//go:build windows

package main

import (
	"fmt"

	"arcmantle/vortex/internal/uninstall"
)

// scheduleWindowsUninstall removes platform artifacts (registry, shortcuts),
// then spawns a detached cleanup helper to delete the files after this process exits.
func scheduleWindowsUninstall(opts uninstall.Options) error {
	// Remove registry entry and Start Menu shortcut now (doesn't require
	// file locks to be released).
	uninstall.Remove(opts)

	// Spawn detached helper to remove binaries and directories after we exit.
	paths := uninstall.AllRemovalPaths(opts)
	if err := uninstall.SpawnCleanupHelper(paths); err != nil {
		return err
	}

	fmt.Println("Vortex will be uninstalled after this process exits.")
	return nil
}

// runUninstallCleanup is the detached helper entry point.
func runUninstallCleanup(args []string) {
	uninstall.RunCleanupHelper(args)
}

