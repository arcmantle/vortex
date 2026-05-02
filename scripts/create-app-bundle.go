//go:build ignore

// create-app-bundle assembles the Vortex.app bundle from template files.
//
// Usage:
//
//	go run scripts/create-app-bundle.go [--version VERSION] [--output DIR] [--setup BINARY]
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	version := flag.String("version", "dev", "version string to embed in Info.plist")
	output := flag.String("output", "./dist", "output directory for the .app bundle")
	setup := flag.String("setup", "", "path to the vortex-setup binary")
	flag.Parse()

	projectRoot := resolveProjectRoot()
	packagingDir := filepath.Join(projectRoot, "packaging", "macos")

	appDir := filepath.Join(*output, "Vortex.app")
	contents := filepath.Join(appDir, "Contents")
	macosDir := filepath.Join(contents, "MacOS")
	resourcesDir := filepath.Join(contents, "Resources")

	fmt.Printf("Creating Vortex.app bundle (version: %s)\n", *version)

	// Clean and create structure.
	os.RemoveAll(appDir)
	must(os.MkdirAll(macosDir, 0o755))
	must(os.MkdirAll(resourcesDir, 0o755))

	// Info.plist — substitute version placeholder.
	plistData, err := os.ReadFile(filepath.Join(packagingDir, "Info.plist"))
	must(err)
	plistOut := strings.ReplaceAll(string(plistData), "__VERSION__", *version)
	must(os.WriteFile(filepath.Join(contents, "Info.plist"), []byte(plistOut), 0o644))

	// Setup binary (CFBundleExecutable).
	if *setup != "" {
		if _, err := os.Stat(*setup); err == nil {
			must(copyFile(*setup, filepath.Join(macosDir, "vortex-setup")))
			must(os.Chmod(filepath.Join(macosDir, "vortex-setup"), 0o755))
		}
	}

	// Icon (if generated).
	icns := filepath.Join(projectRoot, "packaging", "icons", "vortex.icns")
	if _, err := os.Stat(icns); err == nil {
		must(copyFile(icns, filepath.Join(resourcesDir, "vortex.icns")))
	} else {
		fmt.Println("  Warning: vortex.icns not found — app bundle will have no icon")
	}

	fmt.Printf("✓ Created %s\n", appDir)
}

func resolveProjectRoot() string {
	// scripts/ is one level below project root.
	exe, err := os.Getwd()
	must(err)
	// Check if we're already at project root (has go.mod).
	if _, err := os.Stat(filepath.Join(exe, "go.mod")); err == nil {
		return exe
	}
	// Try parent.
	parent := filepath.Dir(exe)
	if _, err := os.Stat(filepath.Join(parent, "go.mod")); err == nil {
		return parent
	}
	// Fallback: use the directory of the script source.
	return exe
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func must(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
