package main

import (
	"path/filepath"
	"sort"
)

const macOSBundleIdentifier = "com.arcmantle.vortex"

func webviewDataCleanupTargetsForGOOS(goos, home string) []string {
	switch goos {
	case "darwin":
		return darwinWebviewDataCleanupTargets(home)
	default:
		return nil
	}
}

func darwinWebviewDataCleanupTargets(home string) []string {
	if home == "" {
		return nil
	}

	targets := map[string]struct{}{
		filepath.Join(home, "Library", "Caches", macOSBundleIdentifier):                          {},
		filepath.Join(home, "Library", "Caches", macOSBundleIdentifier+".WebKit.Networking"):    {},
		filepath.Join(home, "Library", "HTTPStorages", macOSBundleIdentifier):                     {},
		filepath.Join(home, "Library", "Saved Application State", macOSBundleIdentifier+".savedState"): {},
		filepath.Join(home, "Library", "WebKit", macOSBundleIdentifier):                          {},
	}

	paths := make([]string, 0, len(targets))
	for path := range targets {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}