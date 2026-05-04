//go:build ignore

// create-dmg creates a styled macOS .dmg installer using sindresorhus/create-dmg.
//
// Requires: Node.js (uses npx to run sindresorhus/create-dmg)
//
// Usage:
//
//	go run create-dmg.go [--version VERSION] [--app-dir PATH] [--output PATH]
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
	output := flag.String("output", "", "output DMG path (default: Vortex-VERSION.dmg)")
	flag.Parse()

	if *output == "" {
		*output = fmt.Sprintf("Vortex-%s.dmg", *version)
	}

	if _, err := os.Stat(*appDir); err != nil {
		fmt.Fprintf(os.Stderr, "error: app directory %s does not exist — run create-app-bundle.go first\n", *appDir)
		os.Exit(1)
	}

	absApp, err := filepath.Abs(*appDir)
	must(err)
	absOutput, err := filepath.Abs(*output)
	must(err)

	fmt.Printf("Creating DMG: %s (version: %s)\n", absOutput, *version)

	// create-dmg writes the DMG into the destination directory.
	outDir := filepath.Dir(absOutput)
	must(os.MkdirAll(outDir, 0o755))

	cmd := exec.Command("npx", "create-dmg",
		"--overwrite",
		"--dmg-title", "Vortex",
		"--no-code-sign",
		absApp,
		outDir,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	must(cmd.Run())

	// create-dmg generates the filename as "AppName X.Y.Z.dmg" using the
	// version from Info.plist. Rename to the requested output name.
	matches, _ := filepath.Glob(filepath.Join(outDir, "Vortex *.dmg"))
	if len(matches) == 1 && matches[0] != absOutput {
		must(os.Rename(matches[0], absOutput))
	}

	fmt.Printf("✓ Created %s\n", absOutput)
}

func must(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
