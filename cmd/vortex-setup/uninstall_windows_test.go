//go:build windows

package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestWebviewDataCleanupTargets(t *testing.T) {
	appData := t.TempDir()

	for _, dir := range []string{
		"vortex-setup-windows-arm64.exe",
		"vortex-setup-windows-amd64.exe.WebView2",
		"something-else.exe",
	} {
		if err := os.MkdirAll(filepath.Join(appData, dir), 0o755); err != nil {
			t.Fatalf("create temp dir %q: %v", dir, err)
		}
	}

	targets := webviewDataCleanupTargets(appData)

	for _, expected := range []string{
		filepath.Join(appData, "Vortex", "WebView2"),
		filepath.Join(appData, "uninstall.exe"),
		filepath.Join(appData, "uninstall.exe.WebView2"),
		filepath.Join(appData, "vortex-window.exe"),
		filepath.Join(appData, "vortex-window.exe.WebView2"),
		filepath.Join(appData, "vortex-setup.exe"),
		filepath.Join(appData, "vortex-setup.exe.WebView2"),
		filepath.Join(appData, "vortex-setup-windows-arm64.exe"),
		filepath.Join(appData, "vortex-setup-windows-amd64.exe.WebView2"),
	} {
		if !slices.Contains(targets, expected) {
			t.Fatalf("expected cleanup target %q in %v", expected, targets)
		}
	}

	unexpected := filepath.Join(appData, "something-else.exe")
	if slices.Contains(targets, unexpected) {
		t.Fatalf("unexpected cleanup target %q in %v", unexpected, targets)
	}
}