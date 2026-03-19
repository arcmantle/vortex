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
	flag.Parse()

	if *commit == "" {
		*commit = gitCommit()
	}
	if *output == "" {
		*output = defaultOutput(*targetOS, *targetArch)
	}

	fmt.Printf("Building vortex %s (%s/%s) → %s\n", *version, *targetOS, *targetArch, *output)

	// Step 1: optionally build the frontend.
	if *buildUI {
		fmt.Println("── Building frontend UI")
		uiDir := filepath.Join("cmd", "vortex-ui", "web")
		run(uiDir, "pnpm", "install", "--frozen-lockfile")
		run(uiDir, "pnpm", "build")
	}

	// Step 2: compile.
	fmt.Println("── Compiling")
	now := time.Now().UTC().Format(time.RFC3339)
	ldflags := strings.Join([]string{
		"-s", "-w",
		"-X", "main.Version=" + *version,
		"-X", "main.BuildTime=" + now,
		"-X", "main.GitCommit=" + *commit,
	}, " ")
	if *targetOS == "windows" {
		ldflags += " -H=windowsgui"
	}

	env := os.Environ()
	env = setEnv(env, "CGO_ENABLED", "1")
	env = setEnv(env, "GOOS", *targetOS)
	env = setEnv(env, "GOARCH", *targetArch)

	cmd := exec.Command("go", "build",
		"-tags", "embed_ui",
		"-ldflags", ldflags,
		"-o", *output,
		"./cmd/vortex",
	)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fatal("go build failed: %v", err)
	}

	fmt.Printf("✓ Built %s\n", *output)
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
func defaultOutput(goos, goarch string) string {
	name := fmt.Sprintf("vortex-%s-%s", goos, goarch)
	if goos == "windows" {
		name += ".exe"
	}
	return name
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
