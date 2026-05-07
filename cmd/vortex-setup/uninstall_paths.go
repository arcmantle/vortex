package main

import "arcmantle/vortex/internal/uninstall"

// webviewDataCleanupTargetsForGOOS delegates to the shared uninstall package.
// Retained for test compatibility.
func webviewDataCleanupTargetsForGOOS(goos, home string) []string {
	return uninstall.DarwinWebviewCachePathsForHome(goos, home)
}
