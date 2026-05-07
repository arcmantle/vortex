package main

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestWebviewDataCleanupTargetsForGOOSDarwin(t *testing.T) {
	home := filepath.Join("Users", "tester")
	targets := webviewDataCleanupTargetsForGOOS("darwin", home)

	const bundleID = "com.arcmantle.vortex"
	for _, expected := range []string{
		filepath.Join(home, "Library", "Caches", bundleID),
		filepath.Join(home, "Library", "Caches", bundleID+".WebKit.Networking"),
		filepath.Join(home, "Library", "HTTPStorages", bundleID),
		filepath.Join(home, "Library", "Saved Application State", bundleID+".savedState"),
		filepath.Join(home, "Library", "WebKit", bundleID),
	} {
		if !slices.Contains(targets, expected) {
			t.Fatalf("expected cleanup target %q in %v", expected, targets)
		}
	}
}

func TestWebviewDataCleanupTargetsForGOOSNonDarwin(t *testing.T) {
	targets := webviewDataCleanupTargetsForGOOS("linux", filepath.Join("Users", "tester"))
	if len(targets) != 0 {
		t.Fatalf("expected no cleanup targets for linux, got %v", targets)
	}
}
