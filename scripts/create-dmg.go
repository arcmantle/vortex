//go:build ignore

// create-dmg creates a macOS .dmg with Vortex.app and an Applications symlink.
//
// Usage:
//
//	go run scripts/create-dmg.go [--version VERSION] [--app-dir PATH] [--output PATH]
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	version := flag.String("version", "dev", "version string for the DMG filename")
	appDir := flag.String("app-dir", "./dist/Vortex.app", "path to the .app bundle")
	output := flag.String("output", "", "output DMG path (default: ./dist/Vortex-VERSION.dmg)")
	flag.Parse()

	if *output == "" {
		*output = fmt.Sprintf("./dist/Vortex-%s.dmg", *version)
	}

	if _, err := os.Stat(*appDir); err != nil {
		fatal("app directory %s does not exist — run create-app-bundle.go first", *appDir)
	}

	fmt.Printf("Creating DMG: %s (version: %s)\n", *output, *version)

	// Create a temporary staging directory.
	staging, err := os.MkdirTemp("", "vortex-dmg-*")
	must(err)
	defer os.RemoveAll(staging)

	// Copy the app bundle into staging.
	run("cp", "-R", *appDir, filepath.Join(staging, "Vortex.app"))

	// Create Applications symlink.
	must(os.Symlink("/Applications", filepath.Join(staging, "Applications")))

	// Create the DMG with zlib compression.
	must(os.MkdirAll(filepath.Dir(*output), 0o755))
	os.Remove(*output) // remove existing if present

	run("hdiutil", "create",
		"-volname", "Vortex",
		"-srcfolder", staging,
		"-ov",
		"-format", "UDZO",
		"-imagekey", "zlib-level=9",
		*output,
	)

	fmt.Printf("✓ Created %s\n", *output)
	fmt.Printf("\nTo verify: hdiutil verify %s\n", *output)
}

func run(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fatal("%s failed: %v", name, err)
	}
}

func must(err error) {
	if err != nil {
		fatal("%v", err)
	}
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}
