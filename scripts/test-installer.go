//go:build ignore

// Test script for the native installer flow.
//
// Builds a self-contained Vortex.app (macOS) or installer (Windows) with
// pre-built binaries bundled inside. No GitHub access needed.
//
// macOS:
//  1. Produces a DMG at /tmp/vortex-installer-test/Vortex.dmg
//  2. Opens it in Finder
//  3. You drag Vortex.app to /Applications (or anywhere)
//  4. Launch it — the bootstrap progress UI appears, installs locally, done.
//
// Windows:
//  1. Produces vortex-setup.exe with bundled binaries
//  2. Launches it — the installer UI appears, installs, done.
//
// Usage:
//
//	go run scripts/test-installer.go          # build + open DMG / run installer
//	go run scripts/test-installer.go --build  # build only, don't open
//	go run scripts/test-installer.go --clean  # remove build artifacts
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var workDir = filepath.Join(os.TempDir(), "vortex-installer-test")

func main() {
	buildOnly := flag.Bool("build", false, "build only, don't open/run")
	clean := flag.Bool("clean", false, "remove build artifacts and exit")
	flag.Parse()

	if *clean {
		doClean()
		return
	}

	doBuild()

	if !*buildOnly {
		doOpen()
	}
}

func doBuild() {
	os.RemoveAll(workDir)
	must(os.MkdirAll(workDir, 0o755))

	// Rebuild the frontend UI so the embedded assets are current.
	step("Building UI")
	uiDir := filepath.Join("cmd", "vortex-ui", "web")
	run("pnpm", "--dir", uiDir, "install", "--frozen-lockfile")
	run("pnpm", "--dir", uiDir, "build")

	// Build the two binaries that get "installed" by the bootstrap/installer.
	step("Building vortex")
	goBuild(filepath.Join(workDir, binaryName("vortex")), "./cmd/vortex/")

	step("Building vortex-window")
	goBuild(filepath.Join(workDir, binaryName("vortex-window")), "./cmd/vortex-window/")

	switch runtime.GOOS {
	case "darwin":
		buildMacOS()
	case "windows":
		buildWindows()
	default:
		fatal("unsupported OS: %s", runtime.GOOS)
	}
}

func buildMacOS() {
	step("Building vortex-setup")
	goBuild(filepath.Join(workDir, "vortex-setup"), "./cmd/vortex-setup/")

	step("Creating .app bundle")
	run("go", "run", "scripts/create-app-bundle.go",
		"--version", "0.0.0-local",
		"--output", workDir,
		"--setup", filepath.Join(workDir, "vortex-setup"),
	)

	// Embed local binaries inside the bundle so vortex-setup finds them
	// without needing env vars (works when launched from Finder).
	localBinDir := filepath.Join(workDir, "Vortex.app", "Contents", "Resources", "local-binaries")
	must(os.MkdirAll(localBinDir, 0o755))
	copyFileTo(filepath.Join(workDir, "vortex"), filepath.Join(localBinDir, "vortex"))
	copyFileTo(filepath.Join(workDir, "vortex-window"), filepath.Join(localBinDir, "vortex-window"))

	step("Creating DMG")
	dmgPath := filepath.Join(workDir, "Vortex.dmg")
	run("go", "run", "scripts/create-dmg.go",
		"--version", "0.0.0-local",
		"--app-dir", filepath.Join(workDir, "Vortex.app"),
		"--output", dmgPath,
	)

	success("DMG ready: %s", dmgPath)
	fmt.Println()
	info("To test the full flow:")
	info("  1. Open the DMG (will happen automatically unless --build)")
	info("  2. Drag Vortex.app to Applications (or anywhere)")
	info("  3. Launch Vortex.app — setup progress UI will appear")
	info("  4. Binaries get installed to ~/.local/bin/")
}

func buildWindows() {
	step("Building vortex-setup")
	goBuild(filepath.Join(workDir, binaryName("vortex-setup")), "./cmd/vortex-setup/")

	// Place binaries where the installer can find them via VORTEX_BOOTSTRAP_LOCAL.
	binDir := filepath.Join(workDir, "binaries")
	must(os.MkdirAll(binDir, 0o755))
	must(os.Rename(filepath.Join(workDir, "vortex.exe"), filepath.Join(binDir, "vortex.exe")))
	must(os.Rename(filepath.Join(workDir, "vortex-window.exe"), filepath.Join(binDir, "vortex-window.exe")))

	success("Installer ready: %s", filepath.Join(workDir, binaryName("vortex-setup")))
	fmt.Println()
	info("To test the full flow:")
	info("  1. Run the installer (will happen automatically unless --build)")
	info("  2. The installer GUI will appear and install to %%LOCALAPPDATA%%\\Programs\\Vortex")
}

func doOpen() {
	// Kill any running vortex instances so the new binary is used.
	killVortex()

	// Remove existing vortex install so the bootstrap actually runs.
	uninstallVortex()

	switch runtime.GOOS {
	case "darwin":
		// Detach any previously mounted Vortex DMG volumes.
		detachVortexDMG()

		dmgPath := filepath.Join(workDir, "Vortex.dmg")
		step("Opening DMG")
		run("open", dmgPath)
		fmt.Println()
		info("Drag Vortex.app to Applications, then launch it.")
		info("The bootstrap UI will install vortex to ~/.local/bin/")

	case "windows":
		installer := filepath.Join(workDir, "vortex-setup.exe")
		step("Running installer")
		cmd := exec.Command(installer)
		cmd.Env = append(os.Environ(), "VORTEX_BOOTSTRAP_LOCAL="+filepath.Join(workDir, "binaries"))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			info("Installer exited: %v", err)
		}
	}
}

func uninstallVortex() {
	var installDir string
	switch runtime.GOOS {
	case "darwin", "linux":
		home, _ := os.UserHomeDir()
		installDir = filepath.Join(home, ".local", "bin")
	case "windows":
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			home, _ := os.UserHomeDir()
			base = filepath.Join(home, "AppData", "Local")
		}
		installDir = filepath.Join(base, "Programs", "Vortex")
	}

	binaries := []string{"vortex", "vortex-window"}
	removed := false
	for _, name := range binaries {
		p := filepath.Join(installDir, binaryName(name))
		if _, err := os.Stat(p); err == nil {
			os.Remove(p)
			removed = true
		}
	}

	switch runtime.GOOS {
	case "darwin":
		appPath := "/Applications/Vortex.app"
		if _, err := os.Stat(appPath); err == nil {
			os.RemoveAll(appPath)
			removed = true
			step("Removed " + appPath)
		}
	case "windows":
		// Remove uninstall.exe from install dir.
		uninstallExe := filepath.Join(installDir, "uninstall.exe")
		if _, err := os.Stat(uninstallExe); err == nil {
			os.Remove(uninstallExe)
			removed = true
		}
		// Remove Start Menu shortcut.
		appData := os.Getenv("APPDATA")
		if appData != "" {
			shortcut := filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Vortex.lnk")
			if _, err := os.Stat(shortcut); err == nil {
				os.Remove(shortcut)
				removed = true
			}
		}
		// Remove Add/Remove Programs registry entry.
		exec.Command("reg", "delete",
			`HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\Vortex`,
			"/f").Run()
		// Remove the install directory itself if empty.
		os.Remove(installDir)
	}

	if removed {
		step("Uninstalled existing vortex from " + installDir)
	}
}

func killVortex() {
	killed := false
	switch runtime.GOOS {
	case "darwin", "linux":
		// Kill vortex-window first (the UI), then vortex itself.
		for _, name := range []string{"vortex-window", "vortex"} {
			cmd := exec.Command("pkill", "-f", name)
			if cmd.Run() == nil {
				killed = true
			}
		}
	case "windows":
		for _, name := range []string{"vortex-window.exe", "vortex.exe"} {
			cmd := exec.Command("taskkill", "/F", "/IM", name)
			cmd.Run()
			killed = true
		}
	}
	if killed {
		step("Killed running vortex processes")
	}
}

func doClean() {
	step("Cleaning up")
	if runtime.GOOS == "darwin" {
		detachVortexDMG()
	}
	os.RemoveAll(workDir)
	info("Removed %s", workDir)
	success("Clean")
}

func detachVortexDMG() {
	// Detach all mounted Vortex volumes (handles "Vortex", "Vortex 1", etc.)
	for i := 0; i < 10; i++ {
		vol := "/Volumes/Vortex"
		if i > 0 {
			vol = fmt.Sprintf("/Volumes/Vortex %d", i)
		}
		if _, err := os.Stat(vol); err != nil {
			continue
		}
		exec.Command("hdiutil", "detach", vol).Run()
	}
}

// --- helpers ---

func goBuild(output, pkg string) {
	cmd := exec.Command("go", "build", "-o", output, pkg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fatal("go build %s failed: %v", pkg, err)
	}
}

func run(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fatal("%s failed: %v", name, err)
	}
}

func copyFileTo(src, dst string) {
	data, err := os.ReadFile(src)
	if err != nil {
		fatal("read %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		fatal("write %s: %v", dst, err)
	}
}

func binaryName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

func must(err error) {
	if err != nil {
		fatal("%v", err)
	}
}

func step(msg string)                    { fmt.Printf("\n\033[32m▸ %s\033[0m\n", msg) }
func info(format string, args ...any)    { fmt.Printf("  \033[2m"+format+"\033[0m\n", args...) }
func success(format string, args ...any) { fmt.Printf("\033[32m✓ "+format+"\033[0m\n", args...) }
func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "\033[31m✗ "+format+"\033[0m\n", args...)
	os.Exit(1)
}

func init() {
	// Ensure we're running from the repo root.
	if _, err := os.Stat("go.mod"); err != nil {
		// Try to find it by walking up.
		dir, _ := os.Getwd()
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				os.Chdir(dir)
				return
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}

		// Check if invoked via go run from the scripts/ dir.
		if exe, err := os.Executable(); err == nil {
			dir := filepath.Dir(exe)
			for range 3 {
				if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
					os.Chdir(dir)
					return
				}
				dir = filepath.Dir(dir)
			}
		}

		fmt.Fprintln(os.Stderr, "Run this from the repository root: go run scripts/test-installer.go")
		os.Exit(1)
	}

	// Windows: replace ANSI escapes with empty if not a terminal.
	if runtime.GOOS == "windows" {
		// Enable virtual terminal processing on Windows 10+.
		enableWindowsANSI()
	}
}

func enableWindowsANSI() {
	// Best-effort: if it fails, ANSI codes just won't render.
	if strings.Contains(os.Getenv("WT_SESSION"), "") {
		return // Windows Terminal supports ANSI natively.
	}
}
