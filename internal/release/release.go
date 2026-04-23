// Package release provides shared utilities for downloading, verifying,
// and installing vortex release binaries from GitHub. It is used by both
// the upgrade command and the standalone installer.
package release

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	RepoOwner         = "arcmantle"
	RepoName          = "vortex"
	ChecksumAssetName = "vortex-checksums.txt"
)

// Release represents a GitHub release.
type Release struct {
	TagName string         `json:"tag_name"`
	HTMLURL string         `json:"html_url"`
	Assets  []ReleaseAsset `json:"assets"`
}

// ReleaseAsset represents a downloadable file attached to a release.
type ReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// --- GitHub API ---

// FetchLatestRelease fetches the latest published release from GitHub.
func FetchLatestRelease(userAgent string) (*Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", RepoOwner, RepoName)
	return fetchReleaseURL(url, userAgent)
}

// FetchRelease fetches a specific release by version tag from GitHub.
func FetchRelease(version, userAgent string) (*Release, error) {
	tag := version
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", RepoOwner, RepoName, tag)
	return fetchReleaseURL(url, userAgent)
}

func fetchReleaseURL(url, userAgent string) (*Release, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("fetch release: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var rel Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}
	return &rel, nil
}

// FetchChecksums downloads and parses a checksums file. The returned map
// is keyed by asset name with lowercase hex SHA-256 values.
func FetchChecksums(url, userAgent string) (map[string]string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build checksum request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("download checksums: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("download checksums: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	result := map[string]string{}
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
		if _, err := hex.DecodeString(sum); err == nil {
			result[name] = sum
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read checksums: %w", err)
	}
	return result, nil
}

// --- Asset naming ---

// AssetName returns the platform-specific release asset filename for a binary.
func AssetName(binary, goos, goarch string) string {
	name := fmt.Sprintf("%s-%s-%s", binary, goos, goarch)
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// BinaryName returns the installed binary filename for the current platform.
func BinaryName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

// --- Download ---

// DownloadAsset downloads a release asset to a temporary file in installDir.
// It returns the temporary file path and the lowercase hex SHA-256 checksum.
func DownloadAsset(url, installDir, userAgent string) (string, string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", "", fmt.Errorf("build download request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return "", "", fmt.Errorf("download release asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", "", fmt.Errorf("download release asset: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	tmp, err := os.CreateTemp(installDir, "vortex-*.download")
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

// --- Install helpers ---

// CopyFile copies src to dst, syncing to disk before returning.
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return fmt.Errorf("open target: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	return out.Close()
}

// --- Install location ---

// ManagedInstallDir returns the platform-specific managed install directory.
func ManagedInstallDir() (string, error) {
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

// --- PATH management ---

// EnsurePathEntry adds dir to the user's shell profile if it is not already
// in PATH. It returns true if a profile was modified.
func EnsurePathEntry(dir string) (bool, error) {
	if PathContains(dir, os.Getenv("PATH")) {
		return false, nil
	}
	if runtime.GOOS == "windows" {
		return true, ensureWindowsPath(dir)
	}
	return true, ensureUnixPath(dir)
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

	if _, err := fmt.Fprintf(f, "\n# Added by Vortex\n%s\n", line); err != nil {
		return fmt.Errorf("update shell profile %s: %w", profilePath, err)
	}
	return nil
}

func shellPathDir(dir, home string) string {
	if SamePath(dir, filepath.Join(home, ".local", "bin")) {
		return "$HOME/.local/bin"
	}
	return dir
}

func fishPathDir(dir, home string) string {
	if SamePath(dir, filepath.Join(home, ".local", "bin")) {
		return "~/.local/bin"
	}
	return dir
}

func ensureWindowsPath(dir string) error {
	powershell, err := FindPowerShell()
	if err != nil {
		return err
	}

	script := fmt.Sprintf(`$dir = '%s'
$current = [Environment]::GetEnvironmentVariable('Path', 'User')
$parts = @()
if ($current) { $parts = $current -split ';' | Where-Object { $_ } }
$exists = $false
foreach ($part in $parts) {
  if ($part.TrimEnd('\') -ieq $dir.TrimEnd('\')) { $exists = $true; break }
}
if (-not $exists) {
  if ([string]::IsNullOrWhiteSpace($current)) { $newPath = $dir } else { $newPath = "$current;$dir" }
  [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
}
`, EscapePowerShellSingleQuotes(dir))

	cmd := exec.Command(powershell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("update user PATH: %w", err)
	}
	return nil
}

// FindPowerShell returns the path to an available PowerShell executable.
func FindPowerShell() (string, error) {
	for _, candidate := range []string{"powershell", "pwsh"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", errors.New("could not find PowerShell")
}

// EscapePowerShellSingleQuotes doubles single quotes for safe embedding
// in PowerShell single-quoted strings.
func EscapePowerShellSingleQuotes(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// --- Path utilities ---

// PathContains reports whether dir appears in the PATH-style rawPath string.
func PathContains(dir, rawPath string) bool {
	cleanDir := CleanPath(dir)
	for _, entry := range filepath.SplitList(rawPath) {
		if SamePath(cleanDir, entry) {
			return true
		}
	}
	return false
}

// SamePath reports whether two paths refer to the same location.
func SamePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return CleanPath(a) == CleanPath(b)
}

// CleanPath resolves symlinks and returns an absolute, clean path.
func CleanPath(path string) string {
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

// --- Version ---

// NormalizeVersion strips whitespace and the leading "v" prefix.
func NormalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}
