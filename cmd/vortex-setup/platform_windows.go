//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"arcmantle/vortex/internal/release"
	"arcmantle/vortex/internal/webview"

	"golang.org/x/sys/windows/registry"
)

// platformPostInstall creates a Start Menu shortcut and registers the app
// in Add/Remove Programs on Windows.
func platformPostInstall(installDir string) error {
	if err := webview.WriteWindowsIcon(filepath.Join(installDir, "vortex.ico")); err != nil {
		fmt.Fprintf(os.Stderr, "warning: icon install failed: %v\n", err)
	}

	if _, err := release.EnsurePathEntry(installDir); err != nil {
		// Non-fatal: log but continue.
		fmt.Fprintf(os.Stderr, "warning: PATH update failed: %v\n", err)
	}

	if err := createStartMenuShortcut(installDir); err != nil {
		fmt.Fprintf(os.Stderr, "warning: Start Menu shortcut: %v\n", err)
	}

	if err := registerUninstall(installDir); err != nil {
		fmt.Fprintf(os.Stderr, "warning: registry registration: %v\n", err)
	}

	return nil
}

// createStartMenuShortcut creates a .lnk shortcut in the user's Start Menu
// Programs folder pointing to vortex.exe run.
func createStartMenuShortcut(installDir string) error {
	startMenu := os.Getenv("APPDATA")
	if startMenu == "" {
		return fmt.Errorf("APPDATA not set")
	}
	shortcutDir := filepath.Join(startMenu, "Microsoft", "Windows", "Start Menu", "Programs")
	if err := os.MkdirAll(shortcutDir, 0o755); err != nil {
		return err
	}
	shortcutPath := filepath.Join(shortcutDir, "Vortex.lnk")
	target := filepath.Join(installDir, release.ManagedGUIBinaryName())

	// Use PowerShell to create the .lnk file (avoids COM interop in Go).
	script := fmt.Sprintf(`
$ws = New-Object -ComObject WScript.Shell
$sc = $ws.CreateShortcut('%s')
$sc.TargetPath = '%s'
$sc.Arguments = ''
$sc.WorkingDirectory = '%s'
$sc.Description = 'Vortex Terminal'
$iconPath = '%s'
if (Test-Path $iconPath) { $sc.IconLocation = $iconPath }
$sc.Save()
`, escape(shortcutPath), escape(target), escape(installDir), escape(filepath.Join(installDir, "vortex.ico")))

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNoWindow,
		HideWindow:    true,
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("powershell shortcut: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// registerUninstall adds an entry to HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\Vortex
// so the app appears in Settings → Apps → Installed Apps.
func registerUninstall(installDir string) error {
	keyPath := `Software\Microsoft\Windows\CurrentVersion\Uninstall\Vortex`
	key, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("create registry key: %w", err)
	}
	defer key.Close()

	uninstallExe := filepath.Join(installDir, "uninstall.exe")
	version := release.NormalizeVersion(Version)

	entries := map[string]string{
		"DisplayName":     "Vortex",
		"DisplayVersion":  version,
		"Publisher":       "Arcmantle",
		"InstallLocation": installDir,
		"UninstallString": uninstallExe + " --uninstall",
		"DisplayIcon":     filepath.Join(installDir, "vortex.ico"),
		"URLInfoAbout":    "https://github.com/arcmantle/vortex",
		"NoModify":        "",
		"NoRepair":        "",
	}

	for name, value := range entries {
		if name == "NoModify" || name == "NoRepair" {
			key.SetDWordValue(name, 1)
		} else {
			if err := key.SetStringValue(name, value); err != nil {
				return fmt.Errorf("set %s: %w", name, err)
			}
		}
	}

	return nil
}

// escape single quotes for PowerShell string literals.
func escape(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
