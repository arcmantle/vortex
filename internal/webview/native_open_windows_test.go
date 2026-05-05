//go:build windows

package webview

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWebView2UserDataFolder(t *testing.T) {
	configDir := filepath.Join(`C:\Users\roen`, "AppData", "Roaming")
	exePath := filepath.Join(`C:\Users\roen`, "Downloads", "vortex-setup-windows-arm64.exe")

	got := webView2UserDataFolder(configDir, exePath)
	want := filepath.Join(configDir, "Vortex", "WebView2", "vortex-setup-windows-arm64.exe")
	if got != want {
		t.Fatalf("webView2UserDataFolder() = %q, want %q", got, want)
	}
}

func TestPrepareWebView2UserDataFolderEnvSetsAndRestores(t *testing.T) {
	t.Setenv(webview2UserDataFolderEnv, "")
	configDir := filepath.Join(`C:\Users\roen`, "AppData", "Roaming")
	exePath := filepath.Join(`C:\Users\roen`, "AppData", "Local", "Programs", "Vortex", "vortex-window.exe")

	restore := prepareWebView2UserDataFolderEnv(configDir, exePath)
	t.Cleanup(restore)

	got := os.Getenv(webview2UserDataFolderEnv)
	want := filepath.Join(configDir, "Vortex", "WebView2", "vortex-window.exe")
	if got != want {
		t.Fatalf("%s = %q, want %q", webview2UserDataFolderEnv, got, want)
	}

	restore()
	if got := os.Getenv(webview2UserDataFolderEnv); got != "" {
		t.Fatalf("restore left %s = %q, want empty string", webview2UserDataFolderEnv, got)
	}
	restore = func() {}
}

func TestPrepareWebView2UserDataFolderEnvRespectsExistingOverride(t *testing.T) {
	const existing = `D:\Custom\WebView2`
	t.Setenv(webview2UserDataFolderEnv, existing)

	restore := prepareWebView2UserDataFolderEnv(`C:\Users\roen\AppData\Roaming`, `C:\Apps\vortex-window.exe`)
	t.Cleanup(restore)

	if got := os.Getenv(webview2UserDataFolderEnv); got != existing {
		t.Fatalf("%s = %q, want existing override %q", webview2UserDataFolderEnv, got, existing)
	}
}