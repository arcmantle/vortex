//go:build ignore

// Build script for Vortex. Handles platform-specific icon embedding,
// version injection, and UI compilation.
//
// Usage:
//
//	go run build.go [flags]
//
// Examples:
//
//	go run build.go                          # build for current OS/arch, dev version
//	go run build.go -ui                      # build frontend first, then compile
//	go run build.go -version 1.0.0           # inject version
//	go run build.go -os linux -arch amd64    # target specific platform
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func main() {
	targetOS := flag.String("os", runtime.GOOS, "target GOOS")
	targetArch := flag.String("arch", runtime.GOARCH, "target GOARCH")
	version := flag.String("version", "dev", "version string")
	commit := flag.String("commit", "", "git commit hash (default: from git)")
	output := flag.String("output", "", "output binary path (default: auto)")
	buildUI := flag.Bool("ui", false, "build frontend UI before compiling")
	local := flag.Bool("local", false, "build runnable local binaries in ./bin")
	flag.Parse()

	if *commit == "" {
		*commit = gitCommit()
	}
	if *output == "" {
		*output = defaultOutput(*targetOS, *targetArch, *local)
	}

	fmt.Printf("Building vortex %s (%s/%s) → %s\n", *version, *targetOS, *targetArch, *output)

	// Step 1: optionally build the frontend.
	if *buildUI {
		fmt.Println("── Building frontend UI")
		uiDir := filepath.Join("cmd", "vortex-ui", "web")
		run(uiDir, "pnpm", "install", "--frozen-lockfile")
		run(uiDir, "pnpm", "build")
	}

	// Step 2: compile vortex (host — always console subsystem).
	fmt.Println("── Compiling vortex (host)")
	now := time.Now().UTC().Format(time.RFC3339)
	ldflags := strings.Join([]string{
		"-s", "-w",
		"-X", "main.Version=" + *version,
		"-X", "main.BuildTime=" + now,
		"-X", "main.GitCommit=" + *commit,
	}, " ")

	env := os.Environ()
	env = setEnv(env, "CGO_ENABLED", "1")
	env = setEnv(env, "GOOS", *targetOS)
	env = setEnv(env, "GOARCH", *targetArch)
	env = configureLocalToolchain(env, *targetOS, *targetArch, *local)

	cmd := exec.Command("go", "build",
		"-ldflags", ldflags,
		"-o", *output,
		"./cmd/vortex",
	)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fatal("go build (vortex) failed: %v", err)
	}
	fmt.Printf("✓ Built %s\n", *output)

	// Step 3: compile vortex-window (GUI subsystem on Windows).
	windowOutput := windowBinaryOutput(*output, *targetOS, *targetArch, *local)
	fmt.Printf("── Compiling vortex-window (GUI) → %s\n", windowOutput)
	windowLdflags := "-s -w"
	if *targetOS == "windows" {
		windowLdflags += " -H=windowsgui"
	}

	windowCmd := exec.Command("go", "build",
		"-ldflags", windowLdflags,
		"-o", windowOutput,
		"./cmd/vortex-window",
	)
	windowCmd.Env = env
	windowCmd.Stdout = os.Stdout
	windowCmd.Stderr = os.Stderr
	if err := windowCmd.Run(); err != nil {
		fatal("go build (vortex-window) failed: %v", err)
	}
	fmt.Printf("✓ Built %s\n", windowOutput)

	// Step 4: compile vortex-install (standalone installer, pinned to this version).
	installerOutput := installerBinaryOutput(*output, *targetOS, *targetArch, *local)
	fmt.Printf("── Compiling vortex-install → %s\n", installerOutput)
	installerLdflags := strings.Join([]string{
		"-s", "-w",
		"-X", "main.Version=" + *version,
	}, " ")

	installerCmd := exec.Command("go", "build",
		"-ldflags", installerLdflags,
		"-o", installerOutput,
		"./cmd/vortex-install",
	)
	installerCmd.Env = env
	installerCmd.Stdout = os.Stdout
	installerCmd.Stderr = os.Stderr
	if err := installerCmd.Run(); err != nil {
		fatal("go build (vortex-install) failed: %v", err)
	}
	fmt.Printf("✓ Built %s\n", installerOutput)

	// Step 5: compile vortex-bootstrap (macOS first-launch helper, only for darwin).
	if *targetOS == "darwin" {
		bootstrapOutput := bootstrapBinaryOutput(*output, *targetOS, *targetArch, *local)
		fmt.Printf("── Compiling vortex-bootstrap → %s\n", bootstrapOutput)
		bootstrapLdflags := strings.Join([]string{
			"-s", "-w",
			"-X", "main.Version=" + *version,
		}, " ")

		bootstrapCmd := exec.Command("go", "build",
			"-ldflags", bootstrapLdflags,
			"-o", bootstrapOutput,
			"./cmd/vortex-bootstrap",
		)
		bootstrapCmd.Env = env
		bootstrapCmd.Stdout = os.Stdout
		bootstrapCmd.Stderr = os.Stderr
		if err := bootstrapCmd.Run(); err != nil {
			fatal("go build (vortex-bootstrap) failed: %v", err)
		}
		fmt.Printf("✓ Built %s\n", bootstrapOutput)

		// Step 5b: compile vortex-launcher (macOS .app bundle executable).
		launcherOutput := launcherBinaryOutput(*output, *targetOS, *targetArch, *local)
		fmt.Printf("── Compiling vortex-launcher → %s\n", launcherOutput)
		launcherCmd := exec.Command("go", "build",
			"-ldflags", "-s -w",
			"-o", launcherOutput,
			"./cmd/vortex-launcher",
		)
		launcherCmd.Env = env
		launcherCmd.Stdout = os.Stdout
		launcherCmd.Stderr = os.Stderr
		if err := launcherCmd.Run(); err != nil {
			fatal("go build (vortex-launcher) failed: %v", err)
		}
		fmt.Printf("✓ Built %s\n", launcherOutput)
	}

	// Step 6: compile vortex-install-gui (GUI installer for Windows, -H=windowsgui).
	if *targetOS == "windows" {
		guiInstallerOutput := guiInstallerBinaryOutput(*output, *targetOS, *targetArch, *local)
		fmt.Printf("── Compiling vortex-install-gui → %s\n", guiInstallerOutput)
		guiInstallerLdflags := strings.Join([]string{
			"-s", "-w",
			"-X", "main.Version=" + *version,
			"-H=windowsgui",
		}, " ")

		guiInstallerCmd := exec.Command("go", "build",
			"-ldflags", guiInstallerLdflags,
			"-o", guiInstallerOutput,
			"./cmd/vortex-install-gui",
		)
		guiInstallerCmd.Env = env
		guiInstallerCmd.Stdout = os.Stdout
		guiInstallerCmd.Stderr = os.Stderr
		if err := guiInstallerCmd.Run(); err != nil {
			fatal("go build (vortex-install-gui) failed: %v", err)
		}
		fmt.Printf("✓ Built %s\n", guiInstallerOutput)
	}
}

// windowBinaryOutput derives the vortex-window binary path from the host
// binary path by placing it alongside the host binary.
func windowBinaryOutput(hostOutput, goos, goarch string, local bool) string {
	dir := filepath.Dir(hostOutput)
	if local {
		name := "vortex-window"
		if goos == "windows" {
			name += ".exe"
		}
		return filepath.Join(dir, name)
	}
	name := fmt.Sprintf("vortex-window-%s-%s", goos, goarch)
	if goos == "windows" {
		name += ".exe"
	}
	return filepath.Join(dir, name)
}

// installerBinaryOutput derives the vortex-install binary path.
func installerBinaryOutput(hostOutput, goos, goarch string, local bool) string {
	dir := filepath.Dir(hostOutput)
	if local {
		name := "vortex-install"
		if goos == "windows" {
			name += ".exe"
		}
		return filepath.Join(dir, name)
	}
	name := fmt.Sprintf("vortex-install-%s-%s", goos, goarch)
	if goos == "windows" {
		name += ".exe"
	}
	return filepath.Join(dir, name)
}

// bootstrapBinaryOutput derives the vortex-bootstrap binary path (macOS only).
func bootstrapBinaryOutput(hostOutput, goos, goarch string, local bool) string {
	dir := filepath.Dir(hostOutput)
	if local {
		return filepath.Join(dir, "vortex-bootstrap")
	}
	return filepath.Join(dir, fmt.Sprintf("vortex-bootstrap-%s-%s", goos, goarch))
}

// launcherBinaryOutput derives the vortex-launcher binary path (macOS only).
func launcherBinaryOutput(hostOutput, goos, goarch string, local bool) string {
	dir := filepath.Dir(hostOutput)
	if local {
		return filepath.Join(dir, "vortex-launcher")
	}
	return filepath.Join(dir, fmt.Sprintf("vortex-launcher-%s-%s", goos, goarch))
}

// guiInstallerBinaryOutput derives the vortex-install-gui binary path (Windows only).
func guiInstallerBinaryOutput(hostOutput, goos, goarch string, local bool) string {
	dir := filepath.Dir(hostOutput)
	if local {
		return filepath.Join(dir, "vortex-install-gui.exe")
	}
	return filepath.Join(dir, fmt.Sprintf("vortex-install-gui-%s-%s.exe", goos, goarch))
}

// gitCommit returns the short HEAD commit hash, or "unknown".
func gitCommit() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// defaultOutput returns a platform-appropriate binary name.
func defaultOutput(goos, goarch string, local bool) string {
	if local {
		name := filepath.Join("bin", "vortex")
		if goos == "windows" {
			name += ".exe"
		}
		return name
	}

	name := fmt.Sprintf("vortex-%s-%s", goos, goarch)
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

func configureLocalToolchain(env []string, goos, goarch string, local bool) []string {
	if !local || goos != "windows" || goarch != "arm64" {
		return env
	}

	binDir, ok := findLocalLLVMMinGWBin()
	if !ok {
		return env
	}

	fmt.Printf("── Using llvm-mingw toolchain from %s\n", binDir)
	env = prependPath(env, binDir)
	if envValue(env, "CC") == "" {
		env = setEnv(env, "CC", "aarch64-w64-mingw32-clang")
	}
	if envValue(env, "CXX") == "" {
		env = setEnv(env, "CXX", "aarch64-w64-mingw32-clang++")
	}
	return env
}

func findLocalLLVMMinGWBin() (string, bool) {
	patterns := []string{
		filepath.Join(".tools", "llvm-mingw-local", "llvm-mingw-*-ucrt-x86_64", "bin"),
		filepath.Join(".tools", "llvm-mingw", "llvm-mingw-*-ucrt-x86_64", "bin"),
	}

	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, match := range matches {
			clang := filepath.Join(match, "aarch64-w64-mingw32-clang.exe")
			clangxx := filepath.Join(match, "aarch64-w64-mingw32-clang++.exe")
			if fileExists(clang) && fileExists(clangxx) {
				absMatch, err := filepath.Abs(match)
				if err == nil {
					return absMatch, true
				}
				return match, true
			}
		}
	}

	return "", false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func prependPath(env []string, value string) []string {
	current := envValue(env, "PATH")
	if current == "" {
		return setEnv(env, "PATH", value)
	}
	if strings.Contains(strings.ToLower(current), strings.ToLower(value)) {
		return env
	}
	return setEnv(env, "PATH", value+string(os.PathListSeparator)+current)
}

// run executes a command in the given directory and exits on failure.
func run(dir string, name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fatal("%s failed: %v", name, err)
	}
}

// setEnv sets or replaces an environment variable in a slice.
func setEnv(env []string, key, val string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + val
			return env
		}
	}
	return append(env, prefix+val)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
