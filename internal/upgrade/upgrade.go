package upgrade

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"arcmantle/vortex/internal/instance"
	"arcmantle/vortex/internal/release"
)

// Options controls upgrade behavior from the CLI entrypoint.
type Options struct {
	CurrentVersion string
}

// Run downloads and installs the newest release binaries for the current platform.
func Run(args []string, opts Options) error {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	checkOnly := fs.Bool("check", false, "show whether a newer release is available without downloading or installing it")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: vortex upgrade [--check]")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: vortex upgrade [--check]")
	}

	installDir, err := release.ManagedInstallDir()
	if err != nil {
		return err
	}
	if !*checkOnly {
		if err := os.MkdirAll(installDir, 0o755); err != nil {
			return fmt.Errorf("create install dir: %w", err)
		}
	}

	hostTargetPath := filepath.Join(installDir, release.BinaryName("vortex"))
	currentPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current executable: %w", err)
	}
	currentPath = release.CleanPath(currentPath)
	sameInstallPath := release.SamePath(currentPath, hostTargetPath)

	latest, err := release.FetchLatestRelease("vortex-upgrade")
	if err != nil {
		return err
	}

	// Resolve both binary assets.
	type binaryInfo struct {
		name      string
		assetName string
		asset     *release.ReleaseAsset
		checksum  string
		target    string
	}
	binaries := []binaryInfo{
		{name: "vortex", assetName: release.AssetName("vortex", runtime.GOOS, runtime.GOARCH), target: hostTargetPath},
		{name: "vortex-window", assetName: release.AssetName("vortex-window", runtime.GOOS, runtime.GOARCH), target: filepath.Join(installDir, release.BinaryName("vortex-window"))},
	}

	assetMap := map[string]*release.ReleaseAsset{}
	for i := range latest.Assets {
		assetMap[latest.Assets[i].Name] = &latest.Assets[i]
	}
	for i := range binaries {
		binaries[i].asset = assetMap[binaries[i].assetName]
		if binaries[i].asset == nil {
			var available []string
			for _, a := range latest.Assets {
				available = append(available, a.Name)
			}
			return fmt.Errorf("latest release %s does not include %s; available: %s", latest.TagName, binaries[i].assetName, strings.Join(available, ", "))
		}
	}

	checksumAsset := assetMap[release.ChecksumAssetName]
	if checksumAsset == nil {
		return fmt.Errorf("latest release %s does not include %s", latest.TagName, release.ChecksumAssetName)
	}

	// Fetch checksums for all binaries.
	checksums, err := release.FetchChecksums(checksumAsset.BrowserDownloadURL, "vortex-upgrade")
	if err != nil {
		return err
	}
	for i := range binaries {
		sum, ok := checksums[binaries[i].assetName]
		if !ok {
			return fmt.Errorf("checksum file does not contain entry for %s", binaries[i].assetName)
		}
		binaries[i].checksum = sum
	}

	currentVersion := release.NormalizeVersion(opts.CurrentVersion)
	latestVersion := release.NormalizeVersion(latest.TagName)
	upToDate := sameInstallPath && currentVersion != "" && currentVersion == latestVersion
	pathConfigured := release.PathContains(installDir, os.Getenv("PATH"))

	if *checkOnly {
		fmt.Printf("Current version: %s\n", displayVersion(opts.CurrentVersion))
		fmt.Printf("Latest version: %s\n", latest.TagName)
		fmt.Printf("Release page: %s\n", latest.HTMLURL)
		fmt.Printf("Install directory: %s\n", installDir)
		fmt.Printf("Current executable: %s\n", currentPath)
		for _, b := range binaries {
			fmt.Printf("Release asset: %s (checksum: %s)\n", b.assetName, b.checksum)
		}
		fmt.Printf("Managed install location already in use: %t\n", sameInstallPath)
		fmt.Printf("PATH already contains install dir: %t\n", pathConfigured)
		if upToDate {
			fmt.Println("Status: up to date")
		} else {
			fmt.Println("Status: upgrade available")
		}
		return nil
	}

	if upToDate {
		fmt.Printf("vortex %s is already installed at %s\n", latest.TagName, installDir)
		if changed, err := ensurePathEntry(installDir); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not update PATH automatically: %v\n", err)
		} else if changed {
			fmt.Printf("Added %s to your PATH. Open a new terminal to pick it up.\n", installDir)
		}
		return nil
	}

	if err := stopRunningInstance(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not stop active vortex instance: %v\n", err)
	}

	// Download and install each binary.
	for _, b := range binaries {
		fmt.Printf("Downloading %s...\n", b.assetName)
		tmpPath, actualChecksum, err := release.DownloadAsset(b.asset.BrowserDownloadURL, installDir, "vortex-upgrade")
		if err != nil {
			return err
		}
		if actualChecksum != b.checksum {
			os.Remove(tmpPath)
			return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", b.assetName, b.checksum, actualChecksum)
		}
		fmt.Printf("Verified SHA-256 for %s\n", b.assetName)

		isHostBinary := b.name == "vortex"
		if runtime.GOOS == "windows" {
			if isHostBinary && sameInstallPath {
				if err := scheduleWindowsReplacement(tmpPath, b.target, os.Getpid()); err != nil {
					return err
				}
			} else {
				if err := release.CopyFile(tmpPath, b.target); err != nil {
					return err
				}
				if err := os.Remove(tmpPath); err != nil && !errors.Is(err, os.ErrNotExist) {
					fmt.Fprintf(os.Stderr, "warning: could not remove temp file %s: %v\n", tmpPath, err)
				}
			}
		} else {
			if err := os.Rename(tmpPath, b.target); err != nil {
				return fmt.Errorf("install %s: %w", b.name, err)
			}
			if err := release.FinalizeInstall(b.target); err != nil {
				return err
			}
		}
		fmt.Printf("Installed %s\n", b.target)
	}

	if changed, err := ensurePathEntry(installDir); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not update PATH automatically: %v\n", err)
	} else if changed {
		fmt.Printf("Added %s to your PATH. Open a new terminal to pick it up.\n", installDir)
	}

	if runtime.GOOS == "windows" && sameInstallPath {
		fmt.Printf("The host binary will be replaced after this process exits.\n")
		fmt.Printf("Upgraded vortex-window to %s; host binary scheduled for replacement.\n", latest.TagName)
	} else {
		fmt.Printf("Upgraded to vortex %s\n", latest.TagName)
	}
	return nil
}

func displayVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || version == "unknown" {
		return "unknown"
	}
	return version
}

func stopRunningInstance() error {
	instances, err := instance.ListMetadata()
	if err != nil {
		return err
	}
	for _, meta := range instances {
		identity, err := instance.NewIdentity(meta.Name)
		if err != nil {
			continue
		}
		l, first, err := instance.TryLock(identity)
		if err != nil {
			return err
		}
		if first {
			_ = l.Close()
			_ = instance.CleanupInactiveMetadata(meta)
			continue
		}
		if err := instance.Quit(identity); err != nil {
			return err
		}

		deadline := time.Now().Add(10 * time.Second)
		stopped := false
		for time.Now().Before(deadline) {
			l, first, err = instance.TryLock(identity)
			if err != nil {
				return err
			}
			if first {
				_ = l.Close()
				_ = instance.CleanupInactiveMetadata(meta)
				stopped = true
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		if !stopped {
			return fmt.Errorf("timed out waiting for the running vortex instance %q to stop", identity.DisplayName)
		}
	}
	return nil
}

// ensurePathEntry updates the user's shell profile and also updates the
// running process's PATH so subsequent operations see the new directory.
func ensurePathEntry(dir string) (bool, error) {
	changed, err := release.EnsurePathEntry(dir)
	if err != nil {
		return false, err
	}
	if changed {
		pathEnv := os.Getenv("PATH")
		if pathEnv == "" {
			os.Setenv("PATH", dir)
		} else if !release.PathContains(dir, pathEnv) {
			os.Setenv("PATH", dir+string(os.PathListSeparator)+pathEnv)
		}
	}
	return changed, nil
}

func scheduleWindowsReplacement(src string, dst string, waitPID int) error {
	powershell, err := release.FindPowerShell()
	if err != nil {
		return err
	}

	scriptFile, err := os.CreateTemp(filepath.Dir(dst), "vortex-upgrade-*.ps1")
	if err != nil {
		return fmt.Errorf("create upgrade script: %w", err)
	}
	defer scriptFile.Close()

	script := `param(
  [int]$WaitPid,
  [string]$Source,
  [string]$Target,
  [string]$ScriptPath
)

$logFile = "$env:TEMP\vortex-upgrade.log"

try {
  Wait-Process -Id $WaitPid -ErrorAction SilentlyContinue
  $targetDir = Split-Path -Parent $Target
  if (-not (Test-Path $targetDir)) {
    New-Item -ItemType Directory -Path $targetDir -Force | Out-Null
  }

  $succeeded = $false
  for ($i = 0; $i -lt 120; $i++) {
    try {
      Copy-Item -LiteralPath $Source -Destination $Target -Force
      $succeeded = $true
      Remove-Item -LiteralPath $Source -Force -ErrorAction SilentlyContinue
      break
    } catch {
      Start-Sleep -Milliseconds 250
    }
  }

  if (-not $succeeded) {
    $msg = "$(Get-Date -Format o) FAILED: could not replace $Target after 30s retries. Source kept at $Source"
    Add-Content -LiteralPath $logFile -Value $msg -ErrorAction SilentlyContinue
  } else {
    $msg = "$(Get-Date -Format o) OK: upgraded $Target"
    Add-Content -LiteralPath $logFile -Value $msg -ErrorAction SilentlyContinue
  }
} catch {
  $msg = "$(Get-Date -Format o) ERROR: $($_.Exception.Message)"
  Add-Content -LiteralPath $logFile -Value $msg -ErrorAction SilentlyContinue
} finally {
  Remove-Item -LiteralPath $ScriptPath -Force -ErrorAction SilentlyContinue
}
`

	if _, err := scriptFile.WriteString(script); err != nil {
		return fmt.Errorf("write upgrade script: %w", err)
	}

	cmd := exec.Command(
		powershell,
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy",
		"Bypass",
		"-File",
		scriptFile.Name(),
		"-WaitPid",
		fmt.Sprintf("%d", waitPID),
		"-Source",
		src,
		"-Target",
		dst,
		"-ScriptPath",
		scriptFile.Name(),
	)
	if err := cmd.Start(); err != nil {
		_ = os.Remove(scriptFile.Name())
		return fmt.Errorf("launch upgrade script: %w", err)
	}

	return nil
}
