package uninstall

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"arcmantle/vortex/internal/release"
)

const macOSBundleIdentifier = "com.arcmantle.vortex"

// WebviewCachePaths returns platform-specific webview/browser data paths to
// remove during uninstall.
func WebviewCachePaths() []string {
	switch runtime.GOOS {
	case "darwin":
		return darwinWebviewCachePaths()
	case "windows":
		return windowsWebviewCachePaths()
	default:
		return nil
	}
}

func darwinWebviewCachePaths() []string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return nil
	}
	return []string{
		filepath.Join(home, "Library", "Caches", macOSBundleIdentifier),
		filepath.Join(home, "Library", "Caches", macOSBundleIdentifier+".WebKit.Networking"),
		filepath.Join(home, "Library", "HTTPStorages", macOSBundleIdentifier),
		filepath.Join(home, "Library", "Saved Application State", macOSBundleIdentifier+".savedState"),
		filepath.Join(home, "Library", "WebKit", macOSBundleIdentifier),
	}
}

func windowsWebviewCachePaths() []string {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return nil
	}

	targets := make(map[string]struct{})
	targets[filepath.Join(appData, "Vortex", "WebView2")] = struct{}{}

	addExeDataDirs := func(name string) {
		if name == "" {
			return
		}
		targets[filepath.Join(appData, name)] = struct{}{}
		targets[filepath.Join(appData, name+".WebView2")] = struct{}{}
	}

	addExeDataDirs(release.ManagedGUIBinaryName())
	addExeDataDirs(release.BinaryName("vortex-setup"))

	// Scan for setup variants (different arch suffixes).
	entries, err := os.ReadDir(appData)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			lower := strings.ToLower(name)
			if strings.HasPrefix(lower, "vortex-setup") &&
				(strings.HasSuffix(lower, ".exe") || strings.HasSuffix(lower, ".exe.webview2")) {
				targets[filepath.Join(appData, name)] = struct{}{}
			}
		}
	}

	paths := make([]string, 0, len(targets))
	for p := range targets {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// DarwinWebviewCachePathsForHome is a testable variant that accepts an
// explicit home directory and GOOS.
func DarwinWebviewCachePathsForHome(goos, home string) []string {
	if goos != "darwin" || home == "" {
		return nil
	}
	return []string{
		filepath.Join(home, "Library", "Caches", macOSBundleIdentifier),
		filepath.Join(home, "Library", "Caches", macOSBundleIdentifier+".WebKit.Networking"),
		filepath.Join(home, "Library", "HTTPStorages", macOSBundleIdentifier),
		filepath.Join(home, "Library", "Saved Application State", macOSBundleIdentifier+".savedState"),
		filepath.Join(home, "Library", "WebKit", macOSBundleIdentifier),
	}
}

// WindowsWebviewCachePathsForAppData is a testable variant that accepts an
// explicit appData directory.
func WindowsWebviewCachePathsForAppData(appData string) []string {
	if appData == "" {
		return nil
	}

	targets := make(map[string]struct{})
	targets[filepath.Join(appData, "Vortex", "WebView2")] = struct{}{}

	addExeDataDirs := func(name string) {
		if name == "" {
			return
		}
		targets[filepath.Join(appData, name)] = struct{}{}
		targets[filepath.Join(appData, name+".WebView2")] = struct{}{}
	}

	addExeDataDirs(release.ManagedGUIBinaryName())
	addExeDataDirs(release.BinaryName("vortex-setup"))

	entries, err := os.ReadDir(appData)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			lower := strings.ToLower(name)
			if strings.HasPrefix(lower, "vortex-setup") &&
				(strings.HasSuffix(lower, ".exe") || strings.HasSuffix(lower, ".exe.webview2")) {
				targets[filepath.Join(appData, name)] = struct{}{}
			}
		}
	}

	paths := make([]string, 0, len(targets))
	for p := range targets {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}
