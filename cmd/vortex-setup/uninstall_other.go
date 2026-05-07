//go:build !windows

package main

import (
	"fmt"
	"os"

	"arcmantle/vortex/internal/release"
	"arcmantle/vortex/internal/uninstall"
)

// runUninstall removes binaries and the .app bundle on non-Windows platforms.
func runUninstall() {
	installDir, err := release.ManagedInstallDir()
	if err != nil {
		showError(fmt.Sprintf("resolve install directory: %v", err))
		return
	}

	guiInstallDir, _ := release.ManagedGUIInstallDir()

	removeConfig := false
	for _, arg := range os.Args[1:] {
		if arg == "--remove-config" {
			removeConfig = true
		}
	}

	uninstall.Remove(uninstall.Options{
		InstallDir:    installDir,
		GUIInstallDir: guiInstallDir,
		RemoveConfig:  removeConfig,
	})

	fmt.Println("Vortex has been uninstalled.")
}
