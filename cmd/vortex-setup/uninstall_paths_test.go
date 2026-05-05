package main

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestWebviewDataCleanupTargetsForGOOSDarwin(t *testing.T) {
	home := filepath.Join("Users", "tester")
	targets := webviewDataCleanupTargetsForGOOS("darwin", home)

	for _, expected := range []string{
		filepath.Join(home, "Library", "Caches", macOSBundleIdentifier),
		filepath.Join(home, "Library", "Caches", macOSBundleIdentifier+".WebKit.Networking"),
		filepath.Join(home, "Library", "HTTPStorages", macOSBundleIdentifier),
		filepath.Join(home, "Library", "Saved Application State", macOSBundleIdentifier+".savedState"),
		filepath.Join(home, "Library", "WebKit", macOSBundleIdentifier),
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