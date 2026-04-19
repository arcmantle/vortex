package upgrade

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"arcmantle/vortex/internal/instance"
)

const (
	repoOwner         = "arcmantle"
	repoName          = "vortex"
	checksumAssetName = "vortex-checksums.txt"
)

// Options controls upgrade behavior from the CLI entrypoint.
type Options struct {
	CurrentVersion string
}

type release struct {
	TagName string         `json:"tag_name"`
	HTMLURL string         `json:"html_url"`
	Assets  []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Run downloads and installs the newest release binary for the current platform.
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

	installDir, err := managedInstallDir()
	if err != nil {
		return err
	}
	if !*checkOnly {
		if err := os.MkdirAll(installDir, 0o755); err != nil {
			return fmt.Errorf("create install dir: %w", err)
		}
	}

	targetPath := filepath.Join(installDir, installedBinaryName())
	currentPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current executable: %w", err)
	}
	currentPath = cleanPath(currentPath)
	sameInstallPath := samePath(currentPath, targetPath)

	latest, asset, checksumAsset, err := latestReleaseForCurrentPlatform()
	if err != nil {
		return err
	}

	expectedChecksum, err := fetchExpectedChecksum(checksumAsset.BrowserDownloadURL, asset.Name)
	if err != nil {
		return err
	}

	currentVersion := normalizeVersion(opts.CurrentVersion)
	latestVersion := normalizeVersion(latest.TagName)
	upToDate := sameInstallPath && currentVersion != "" && currentVersion == latestVersion
	pathConfigured := pathContains(installDir, os.Getenv("PATH"))

	if *checkOnly {
		fmt.Printf("Current version: %s\n", displayVersion(opts.CurrentVersion))
		fmt.Printf("Latest version: %s\n", latest.TagName)
		fmt.Printf("Release page: %s\n", latest.HTMLURL)
		fmt.Printf("Managed install path: %s\n", targetPath)
		fmt.Printf("Current executable: %s\n", currentPath)
		fmt.Printf("Release asset: %s\n", asset.Name)
		fmt.Printf("Checksum asset: %s\n", checksumAsset.Name)
		fmt.Printf("Checksum verified from release metadata: %s\n", expectedChecksum)
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
		fmt.Printf("vortex %s is already installed at %s\n", latest.TagName, targetPath)
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

	tmpPath, actualChecksum, err := downloadAsset(asset.BrowserDownloadURL, installDir)
	if err != nil {
		return err
	}
	if actualChecksum != expectedChecksum {
		os.Remove(tmpPath)
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", asset.Name, expectedChecksum, actualChecksum)
	}

	fmt.Printf("Verified SHA-256 for %s\n", asset.Name)

	if runtime.GOOS == "windows" {
		if sameInstallPath {
			if err := scheduleWindowsReplacement(tmpPath, targetPath, os.Getpid()); err != nil {
				return err
			}
		} else {
			if err := copyFile(tmpPath, targetPath); err != nil {
				return err
			}
			if err := os.Remove(tmpPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				fmt.Fprintf(os.Stderr, "warning: could not remove temp file %s: %v\n", tmpPath, err)
			}
		}
	} else {
		if err := os.Chmod(tmpPath, 0o755); err != nil {
			return fmt.Errorf("make downloaded binary executable: %w", err)
		}
		if err := os.Rename(tmpPath, targetPath); err != nil {
			return fmt.Errorf("install binary: %w", err)
		}
		if err := finalizeUnixInstall(targetPath); err != nil {
			return err
		}
	}

	if changed, err := ensurePathEntry(installDir); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not update PATH automatically: %v\n", err)
	} else if changed {
		fmt.Printf("Added %s to your PATH. Open a new terminal to pick it up.\n", installDir)
	}

	if runtime.GOOS == "windows" && sameInstallPath {
		fmt.Printf("Downloading %s from %s\n", latest.TagName, latest.HTMLURL)
		fmt.Printf("Scheduled upgrade to %s after this process exits.\n", targetPath)
		return nil
	}

	fmt.Printf("Installed vortex %s to %s\n", latest.TagName, targetPath)
	if !sameInstallPath {
		fmt.Printf("Current executable is %s\n", currentPath)
	}
	return nil
}

func latestReleaseForCurrentPlatform() (*release, *releaseAsset, *releaseAsset, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repoOwner, repoName)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("build release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "vortex-upgrade")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, nil, nil, fmt.Errorf("fetch latest release: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var latest release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&latest); err != nil {
		return nil, nil, nil, fmt.Errorf("decode latest release: %w", err)
	}

	assetName := releaseAssetName(runtime.GOOS, runtime.GOARCH)
	var binaryAsset *releaseAsset
	var checksumAsset *releaseAsset
	for _, asset := range latest.Assets {
		if asset.Name == assetName {
			assetCopy := asset
			binaryAsset = &assetCopy
		}
		if asset.Name == checksumAssetName {
			assetCopy := asset
			checksumAsset = &assetCopy
		}
	}
	if binaryAsset != nil && checksumAsset != nil {
		return &latest, binaryAsset, checksumAsset, nil
	}

	var available []string
	for _, asset := range latest.Assets {
		available = append(available, asset.Name)
	}
	if binaryAsset == nil {
		return nil, nil, nil, fmt.Errorf("latest release %s does not include %s; available assets: %s", latest.TagName, assetName, strings.Join(available, ", "))
	}
	return nil, nil, nil, fmt.Errorf("latest release %s does not include %s; available assets: %s", latest.TagName, checksumAssetName, strings.Join(available, ", "))
}

func releaseAssetName(goos, goarch string) string {
	name := fmt.Sprintf("vortex-%s-%s", goos, goarch)
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

func installedBinaryName() string {
	if runtime.GOOS == "windows" {
		return "vortex.exe"
	}
	return "vortex"
}

func managedInstallDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	if runtime.GOOS == "windows" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(base, "Programs", "Vortex"), nil
	}
	return filepath.Join(home, ".local", "bin"), nil
}

func downloadAsset(url string, installDir string) (string, string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", "", fmt.Errorf("build download request: %w", err)
	}
	req.Header.Set("User-Agent", "vortex-upgrade")

	resp, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
	if err != nil {
		return "", "", fmt.Errorf("download release asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", "", fmt.Errorf("download release asset: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	tmp, err := os.CreateTemp(installDir, installedBinaryName()+".*.download")
	if err != nil {
		return "", "", fmt.Errorf("create temp file: %w", err)
	}

	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, hasher), resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", "", fmt.Errorf("write release asset: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", "", fmt.Errorf("close downloaded asset: %w", err)
	}

	return tmp.Name(), hex.EncodeToString(hasher.Sum(nil)), nil
}

func fetchExpectedChecksum(url string, assetName string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build checksum request: %w", err)
	}
	req.Header.Set("User-Agent", "vortex-upgrade")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("download checksum asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("download checksum asset: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		sum := strings.ToLower(fields[0])
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if name == assetName {
			if _, err := hex.DecodeString(sum); err != nil {
				return "", fmt.Errorf("invalid checksum for %s in %s", assetName, checksumAssetName)
			}
			return sum, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read checksum asset: %w", err)
	}
	return "", fmt.Errorf("checksum asset %s does not contain an entry for %s", checksumAssetName, assetName)
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

func ensurePathEntry(dir string) (bool, error) {
	if pathContains(dir, os.Getenv("PATH")) {
		return false, nil
	}

	if runtime.GOOS == "windows" {
		if err := ensureWindowsPath(dir); err != nil {
			return false, err
		}
	} else {
		if err := ensureUnixPath(dir); err != nil {
			return false, err
		}
	}

	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		os.Setenv("PATH", dir)
	} else if !pathContains(dir, pathEnv) {
		os.Setenv("PATH", dir+string(os.PathListSeparator)+pathEnv)
	}

	return true, nil
}

func ensureUnixPath(dir string) error {
	if strings.ContainsAny(dir, "\"'`$;&|\n") {
		return fmt.Errorf("unsafe characters in path %q", dir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	profilePath := filepath.Join(home, ".profile")
	line := fmt.Sprintf("export PATH=\"%s:$PATH\"", shellPathDir(dir, home))

	switch filepath.Base(os.Getenv("SHELL")) {
	case "zsh":
		profilePath = filepath.Join(home, ".zshrc")
	case "bash":
		profilePath = filepath.Join(home, ".bashrc")
	case "fish":
		profilePath = filepath.Join(home, ".config", "fish", "config.fish")
		line = fmt.Sprintf("fish_add_path -m %s", fishPathDir(dir, home))
	}

	contents, err := os.ReadFile(profilePath)
	if err == nil && (strings.Contains(string(contents), dir) || strings.Contains(string(contents), line)) {
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read shell profile %s: %w", profilePath, err)
	}

	if err := os.MkdirAll(filepath.Dir(profilePath), 0o755); err != nil {
		return fmt.Errorf("create shell profile directory: %w", err)
	}

	f, err := os.OpenFile(profilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open shell profile %s: %w", profilePath, err)
	}
	defer f.Close()

	if _, err := fmt.Fprintf(f, "\n# Added by vortex upgrade\n%s\n", line); err != nil {
		return fmt.Errorf("update shell profile %s: %w", profilePath, err)
	}

	return nil
}

func shellPathDir(dir string, home string) string {
	homeLocal := filepath.Join(home, ".local", "bin")
	if samePath(dir, homeLocal) {
		return "$HOME/.local/bin"
	}
	return dir
}

func fishPathDir(dir string, home string) string {
	homeLocal := filepath.Join(home, ".local", "bin")
	if samePath(dir, homeLocal) {
		return "~/.local/bin"
	}
	return dir
}

func ensureWindowsPath(dir string) error {
	powershell, err := findPowerShell()
	if err != nil {
		return err
	}

	script := fmt.Sprintf(`$dir = '%s'
$current = [Environment]::GetEnvironmentVariable('Path', 'User')
$parts = @()
if ($current) { $parts = $current -split ';' | Where-Object { $_ } }
$exists = $false
foreach ($part in $parts) {
  if ($part.TrimEnd('\\') -ieq $dir.TrimEnd('\\')) { $exists = $true; break }
}
if (-not $exists) {
  if ([string]::IsNullOrWhiteSpace($current)) { $newPath = $dir } else { $newPath = "$current;$dir" }
  [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
}
`, escapePowerShellSingleQuotes(dir))

	cmd := exec.Command(powershell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("update user PATH: %w", err)
	}

	return nil
}

func scheduleWindowsReplacement(src string, dst string, waitPID int) error {
	powershell, err := findPowerShell()
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

func findPowerShell() (string, error) {
	for _, candidate := range []string{"powershell", "pwsh"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", errors.New("could not find PowerShell to update PATH or replace the installed binary")
}

func escapePowerShellSingleQuotes(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func pathContains(dir string, rawPath string) bool {
	cleanDir := cleanPath(dir)
	for _, entry := range filepath.SplitList(rawPath) {
		if samePath(cleanDir, entry) {
			return true
		}
	}
	return false
}

func samePath(a string, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return cleanPath(a) == cleanPath(b)
}

func cleanPath(path string) string {
	if path == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

func normalizeVersion(version string) string {
	return strings.TrimPrefix(strings.TrimSpace(version), "v")
}

func displayVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || version == "unknown" {
		return "unknown"
	}
	return version
}

func copyFile(src string, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source binary: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return fmt.Errorf("open install target: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy binary: %w", err)
	}

	if err := out.Sync(); err != nil {
		return fmt.Errorf("sync install target: %w", err)
	}

	if err := out.Close(); err != nil {
		return fmt.Errorf("close install target: %w", err)
	}

	return nil
}
