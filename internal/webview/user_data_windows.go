//go:build windows

package webview

import (
	"os"
	"path/filepath"
)

const webview2UserDataFolderEnv = "WEBVIEW2_USER_DATA_FOLDER"

func prepareWindowsWebView2UserDataEnv() func() {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return func() {}
	}
	exePath, err := os.Executable()
	if err != nil {
		return func() {}
	}
	return prepareWebView2UserDataFolderEnv(configDir, exePath)
}

func prepareWebView2UserDataFolderEnv(configDir, exePath string) func() {
	previousValue, hadPreviousValue := os.LookupEnv(webview2UserDataFolderEnv)
	if previousValue != "" {
		return func() {}
	}

	userDataDir := webView2UserDataFolder(configDir, exePath)
	if userDataDir == "" {
		return func() {}
	}
	if err := os.Setenv(webview2UserDataFolderEnv, userDataDir); err != nil {
		return func() {}
	}

	return func() {
		if hadPreviousValue {
			_ = os.Setenv(webview2UserDataFolderEnv, previousValue)
			return
		}
		_ = os.Unsetenv(webview2UserDataFolderEnv)
	}
}

func webView2UserDataFolder(configDir, exePath string) string {
	if configDir == "" || exePath == "" {
		return ""
	}
	exeName := filepath.Base(exePath)
	if exeName == "" || exeName == "." {
		return ""
	}
	return filepath.Join(configDir, "Vortex", "WebView2", exeName)
}