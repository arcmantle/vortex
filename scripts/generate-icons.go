//go:build ignore

// generate-icons generates macOS .icns and Windows .ico files from the SVG source.
//
// Requirements:
//   - rsvg-convert (from librsvg: brew install librsvg)
//   - iconutil (macOS built-in)
//   - For .ico: ImageMagick (brew install imagemagick)
//
// Usage:
//
//	go run scripts/generate-icons.go
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

func main() {
	projectRoot := resolveProjectRoot()
	svg := filepath.Join(projectRoot, "assets", "icon.svg")
	outDir := filepath.Join(projectRoot, "packaging", "icons")
	iconsetDir := filepath.Join(outDir, "vortex.iconset")

	if _, err := os.Stat(svg); err != nil {
		fatal("SVG source not found: %s", svg)
	}

	if _, err := exec.LookPath("rsvg-convert"); err != nil {
		fatal("rsvg-convert not found. Install with: brew install librsvg")
	}

	must(os.MkdirAll(iconsetDir, 0o755))

	fmt.Printf("Generating icon PNGs from %s\n", svg)

	// macOS iconset requires specific filenames and sizes.
	sizes := []int{16, 32, 128, 256, 512}
	for _, size := range sizes {
		s := strconv.Itoa(size)
		out := filepath.Join(iconsetDir, fmt.Sprintf("icon_%sx%s.png", s, s))
		run("rsvg-convert", "-w", s, "-h", s, svg, "-o", out)

		// Retina (@2x) versions.
		double := size * 2
		if double <= 1024 {
			d := strconv.Itoa(double)
			out2x := filepath.Join(iconsetDir, fmt.Sprintf("icon_%sx%s@2x.png", s, s))
			run("rsvg-convert", "-w", d, "-h", d, svg, "-o", out2x)
		}
	}
	fmt.Println("  Generated iconset PNGs")

	// Generate .icns using iconutil (macOS only).
	if _, err := exec.LookPath("iconutil"); err == nil {
		icns := filepath.Join(outDir, "vortex.icns")
		run("iconutil", "--convert", "icns", "--output", icns, iconsetDir)
		fmt.Printf("✓ Generated %s\n", icns)
	} else {
		fmt.Println("  Warning: iconutil not available (not on macOS?) — skipping .icns")
	}

	// Generate .ico using ImageMagick.
	magick := ""
	if p, err := exec.LookPath("magick"); err == nil {
		magick = p
	} else if p, err := exec.LookPath("convert"); err == nil {
		magick = p
	}

	if magick != "" {
		icoSizes := []int{16, 32, 48, 256}
		var pngs []string
		for _, size := range icoSizes {
			s := strconv.Itoa(size)
			png := filepath.Join(outDir, fmt.Sprintf("icon_%s.png", s))
			run("rsvg-convert", "-w", s, "-h", s, svg, "-o", png)
			pngs = append(pngs, png)
		}

		ico := filepath.Join(outDir, "vortex.ico")
		args := append(pngs, ico)
		run(magick, args...)

		// Clean up temp PNGs.
		for _, png := range pngs {
			os.Remove(png)
		}
		fmt.Printf("✓ Generated %s\n", ico)
	} else {
		fmt.Println("  Warning: ImageMagick not found — skipping .ico generation")
		fmt.Println("  Install with: brew install imagemagick")
	}

	// Clean up iconset directory.
	os.RemoveAll(iconsetDir)
	fmt.Println("Done.")
}

func resolveProjectRoot() string {
	cwd, err := os.Getwd()
	must(err)
	if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
		return cwd
	}
	parent := filepath.Dir(cwd)
	if _, err := os.Stat(filepath.Join(parent, "go.mod")); err == nil {
		return parent
	}
	return cwd
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
