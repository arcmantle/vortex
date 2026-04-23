// vortex-install is a standalone installer for Vortex. It downloads both
// the vortex host binary and the vortex-window GUI binary for the current
// platform from the GitHub release matching its pinned version, verifies
// checksums, places them in the managed install directory, and configures
// the user's PATH.
//
// The version is embedded at build time via ldflags:
//
//	go build -ldflags "-X main.Version=v1.0.0" ./cmd/vortex-install
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"arcmantle/vortex/internal/release"
)

var Version = "dev"

func main() {
	version := release.NormalizeVersion(Version)
	if version == "" || version == "dev" {
		fatal("this installer was not built with a release version; use -ldflags \"-X main.Version=vX.Y.Z\"")
	}

	fmt.Printf("Vortex installer %s (%s/%s)\n\n", Version, runtime.GOOS, runtime.GOARCH)

	installDir, err := release.ManagedInstallDir()
	if err != nil {
		fatal("resolve install directory: %v", err)
	}
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		fatal("create install directory: %v", err)
	}

	fmt.Printf("Install directory: %s\n", installDir)

	// Fetch the release matching our pinned version.
	rel, err := release.FetchRelease(Version, "vortex-install")
	if err != nil {
		fatal("%v", err)
	}
	fmt.Printf("Release: %s (%s)\n", rel.TagName, rel.HTMLURL)

	// Resolve assets for this platform.
	hostAssetName := release.AssetName("vortex", runtime.GOOS, runtime.GOARCH)
	windowAssetName := release.AssetName("vortex-window", runtime.GOOS, runtime.GOARCH)

	assets := map[string]*release.ReleaseAsset{}
	for i := range rel.Assets {
		assets[rel.Assets[i].Name] = &rel.Assets[i]
	}

	hostAsset := assets[hostAssetName]
	windowAsset := assets[windowAssetName]
	checksumAsset := assets[release.ChecksumAssetName]

	if hostAsset == nil {
		fatal("release %s does not include %s", rel.TagName, hostAssetName)
	}
	if windowAsset == nil {
		fatal("release %s does not include %s", rel.TagName, windowAssetName)
	}
	if checksumAsset == nil {
		fatal("release %s does not include %s", rel.TagName, release.ChecksumAssetName)
	}

	// Fetch checksums.
	checksums, err := release.FetchChecksums(checksumAsset.BrowserDownloadURL, "vortex-install")
	if err != nil {
		fatal("%v", err)
	}
	hostChecksum, ok := checksums[hostAssetName]
	if !ok {
		fatal("checksum file does not contain entry for %s", hostAssetName)
	}
	windowChecksum, ok := checksums[windowAssetName]
	if !ok {
		fatal("checksum file does not contain entry for %s", windowAssetName)
	}

	// Download and install both binaries.
	binaries := []struct {
		name     string
		asset    *release.ReleaseAsset
		checksum string
		target   string
	}{
		{name: "vortex", asset: hostAsset, checksum: hostChecksum, target: filepath.Join(installDir, release.BinaryName("vortex"))},
		{name: "vortex-window", asset: windowAsset, checksum: windowChecksum, target: filepath.Join(installDir, release.BinaryName("vortex-window"))},
	}

	fmt.Println()
	for _, b := range binaries {
		fmt.Printf("Downloading %s...\n", b.asset.Name)
		tmpPath, actualChecksum, err := release.DownloadAsset(b.asset.BrowserDownloadURL, installDir, "vortex-install")
		if err != nil {
			fatal("download %s: %v", b.name, err)
		}
		if actualChecksum != b.checksum {
			os.Remove(tmpPath)
			fatal("checksum mismatch for %s: expected %s, got %s", b.asset.Name, b.checksum, actualChecksum)
		}
		fmt.Printf("  Verified SHA-256: %s\n", b.checksum)

		if err := installBinary(tmpPath, b.target); err != nil {
			fatal("install %s: %v", b.name, err)
		}
		fmt.Printf("  Installed: %s\n", b.target)
	}

	// Configure PATH.
	fmt.Println()
	if changed, err := release.EnsurePathEntry(installDir); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not update PATH automatically: %v\n", err)
	} else if changed {
		fmt.Printf("Added %s to your PATH. Open a new terminal to pick it up.\n", installDir)
	} else {
		fmt.Printf("PATH already contains %s\n", installDir)
	}

	fmt.Printf("\nVortex %s installed successfully.\n", rel.TagName)
}

func installBinary(tmpPath, targetPath string) error {
	if runtime.GOOS == "windows" {
		if err := release.CopyFile(tmpPath, targetPath); err != nil {
			return err
		}
		return os.Remove(tmpPath)
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return release.FinalizeInstall(targetPath)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
