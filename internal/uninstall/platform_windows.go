//go:build windows

package uninstall

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// removePlatformArtifacts removes Windows-specific artifacts: Start Menu
// shortcut and the Add/Remove Programs registry entry.
func removePlatformArtifacts(_ Options) {
	appData := os.Getenv("APPDATA")
	if appData != "" {
		shortcut := filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Vortex.lnk")
		os.Remove(shortcut)
	}

	keyPath := `Software\Microsoft\Windows\CurrentVersion\Uninstall\Vortex`
	registry.DeleteKey(registry.CURRENT_USER, keyPath)
}
