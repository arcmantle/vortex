//go:build ignore

// build-local-windows-arm64 cross-compiles vortex for Windows arm64 from
// an x64 host using llvm-mingw. It downloads the toolchain if needed.
//
// Usage:
//
//	go run scripts/build-local-windows-arm64.go [--version VERSION] [--skip-ui]
package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const toolchainVersion = "20260324"

func main() {
	version := flag.String("version", "dev", "version to embed")
	skipUI := flag.Bool("skip-ui", false, "skip UI build")
	flag.Parse()

	projectRoot := resolveProjectRoot()
	must(os.Chdir(projectRoot))

	toolchainRoot := filepath.Join(projectRoot, ".tools", "llvm-mingw-local")
	archiveName := fmt.Sprintf("llvm-mingw-%s-ucrt-x86_64.zip", toolchainVersion)
	archivePath := filepath.Join(toolchainRoot, archiveName)
	toolchainDir := filepath.Join(toolchainRoot, strings.TrimSuffix(archiveName, ".zip"))
	toolchainBin := filepath.Join(toolchainDir, "bin")

	clangName := "aarch64-w64-mingw32-clang"
	if runtime.GOOS == "windows" {
		clangName += ".exe"
	}
	clangPath := filepath.Join(toolchainBin, clangName)

	must(os.MkdirAll(toolchainRoot, 0o755))

	// Download if needed.
	if _, err := os.Stat(archivePath); err != nil {
		url := fmt.Sprintf("https://github.com/mstorsjo/llvm-mingw/releases/download/%s/%s", toolchainVersion, archiveName)
		fmt.Printf("Downloading llvm-mingw %s\n", toolchainVersion)
		must(downloadFile(url, archivePath))
	}

	// Extract if needed.
	if _, err := os.Stat(clangPath); err != nil {
		os.RemoveAll(toolchainDir)
		fmt.Printf("Extracting llvm-mingw %s\n", toolchainVersion)
		must(unzip(archivePath, toolchainRoot))
	}

	// Verify.
	run(clangPath, "--version")

	// Set environment and build.
	os.Setenv("PATH", toolchainBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	os.Setenv("CGO_ENABLED", "1")
	os.Setenv("CC", "aarch64-w64-mingw32-clang")
	os.Setenv("CXX", "aarch64-w64-mingw32-clang++")

	buildArgs := []string{"run", "build.go", "-local", "-os", "windows", "-arch", "arm64", "-version", *version}
	if !*skipUI {
		buildArgs = append(buildArgs, "-ui")
	}

	cmd := exec.Command("go", buildArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		fatal("build failed: %v", err)
	}
}

func downloadFile(url, dst string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		path := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path in zip: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(path, f.Mode())
			continue
		}
		os.MkdirAll(filepath.Dir(path), 0o755)
		outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}
		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return err
		}
	}
	return nil
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
