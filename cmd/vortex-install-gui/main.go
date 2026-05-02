// vortex-install-gui is a Windows GUI installer for Vortex. It displays a
// branded webview window with a progress bar, downloads the release binaries,
// verifies checksums, installs them, creates a Start Menu shortcut, and
// registers the app in Add/Remove Programs.
//
// Built with -H=windowsgui ldflags to suppress the console window.
//
// Also supports --uninstall mode (or when invoked as uninstall.exe) to
// remove binaries, shortcuts, and registry entries.
//
// Build:
//
//	go build -ldflags "-s -w -X main.Version=v1.0.0 -H=windowsgui" ./cmd/vortex-install-gui
package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"arcmantle/vortex/internal/release"
	"arcmantle/vortex/internal/webview"
)

var Version = "dev"

func main() {
	// Detect uninstall mode: --uninstall flag or binary named "uninstall".
	uninstallMode := false
	for _, arg := range os.Args[1:] {
		if arg == "--uninstall" {
			uninstallMode = true
			break
		}
	}
	if !uninstallMode {
		exe := filepath.Base(os.Args[0])
		exe = strings.TrimSuffix(exe, filepath.Ext(exe))
		if strings.EqualFold(exe, "uninstall") {
			uninstallMode = true
		}
	}

	if uninstallMode {
		runUninstall()
		return
	}

	runInstall()
}

func runInstall() {
	localDir := os.Getenv("VORTEX_BOOTSTRAP_LOCAL")
	version := release.NormalizeVersion(Version)
	if localDir == "" && (version == "" || version == "dev") {
		showError("This installer was not built with a release version (set VORTEX_BOOTSTRAP_LOCAL to test locally).")
		return
	}

	installDir, err := release.ManagedInstallDir()
	if err != nil {
		showError(fmt.Sprintf("Failed to resolve install directory: %v", err))
		return
	}

	// Check if already installed.
	vortexBin := filepath.Join(installDir, release.BinaryName("vortex"))
	alreadyInstalled := fileExists(vortexBin)

	// Start local HTTP server for the UI.
	mux := http.NewServeMux()
	progressCh := make(chan progressUpdate, 10)
	doneCh := make(chan error, 1)
	actionCh := make(chan string, 1)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if alreadyInstalled {
			w.Write([]byte(alreadyInstalledHTML))
		} else {
			w.Write([]byte(installerHTML))
		}
	})

	mux.HandleFunc("/action", func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")
		actionCh <- action
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/progress", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		for update := range progressCh {
			fmt.Fprintf(w, "data: {\"step\":%q,\"progress\":%d}\n\n", update.step, update.progress)
			flusher.Flush()
		}
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		showError(fmt.Sprintf("Failed to start UI server: %v", err))
		return
	}
	addr := listener.Addr().String()
	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	defer server.Shutdown(context.Background())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		if alreadyInstalled {
			// Wait for user's choice.
			action := <-actionCh
			switch action {
			case "reinstall":
				// Continue with install.
			case "cancel":
				cancel()
				return
			default:
				cancel()
				return
			}
		}

		if localDir != "" {
			doneCh <- doLocalInstall(installDir, localDir, progressCh)
		} else {
			doneCh <- doInstall(installDir, version, progressCh)
		}
	}()

	url := fmt.Sprintf("http://%s/", addr)
	webview.OpenWithContext(ctx, "Install Vortex", url, 460, 300)
	wg.Wait()

	// Check result.
	select {
	case err := <-doneCh:
		if err != nil {
			showError(fmt.Sprintf("Installation failed: %v", err))
		}
	default:
	}
}

type progressUpdate struct {
	step     string
	progress int
}

func doInstall(installDir, version string, progressCh chan<- progressUpdate) error {
	defer close(progressCh)

	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return fmt.Errorf("create install directory: %w", err)
	}

	progressCh <- progressUpdate{"Fetching release info...", 5}

	rel, err := release.FetchRelease("v"+version, "vortex-install-gui")
	if err != nil {
		return err
	}

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
		return fmt.Errorf("release does not include %s", hostAssetName)
	}
	if windowAsset == nil {
		return fmt.Errorf("release does not include %s", windowAssetName)
	}
	if checksumAsset == nil {
		return fmt.Errorf("release does not include checksums")
	}

	progressCh <- progressUpdate{"Verifying checksums...", 10}
	checksums, err := release.FetchChecksums(checksumAsset.BrowserDownloadURL, "vortex-install-gui")
	if err != nil {
		return err
	}

	binaries := []struct {
		name     string
		asset    *release.ReleaseAsset
		checksum string
		target   string
		progress int
	}{
		{"vortex", hostAsset, checksums[hostAssetName], filepath.Join(installDir, release.BinaryName("vortex")), 50},
		{"vortex-window", windowAsset, checksums[windowAssetName], filepath.Join(installDir, release.BinaryName("vortex-window")), 80},
	}

	for _, b := range binaries {
		progressCh <- progressUpdate{fmt.Sprintf("Downloading %s...", b.name), b.progress}

		tmpPath, actualChecksum, err := release.DownloadAsset(b.asset.BrowserDownloadURL, installDir, "vortex-install-gui")
		if err != nil {
			return fmt.Errorf("download %s: %w", b.name, err)
		}
		if actualChecksum != b.checksum {
			os.Remove(tmpPath)
			return fmt.Errorf("checksum mismatch for %s", b.name)
		}

		if err := installBinary(tmpPath, b.target); err != nil {
			return fmt.Errorf("install %s: %w", b.name, err)
		}
	}

	// Platform-specific post-install (shortcuts, registry, PATH).
	progressCh <- progressUpdate{"Configuring system...", 90}
	if err := platformPostInstall(installDir); err != nil {
		return fmt.Errorf("post-install: %w", err)
	}

	// Copy ourselves as uninstall.exe alongside the binaries.
	progressCh <- progressUpdate{"Finalizing...", 95}
	selfPath, err := os.Executable()
	if err == nil {
		uninstallPath := filepath.Join(installDir, "uninstall"+filepath.Ext(selfPath))
		release.CopyFile(selfPath, uninstallPath)
	}

	progressCh <- progressUpdate{"Installation complete!", 100}
	return nil
}

// doLocalInstall copies pre-built binaries from a local directory instead of
// downloading from GitHub. Activated by VORTEX_BOOTSTRAP_LOCAL=/path/to/dir.
func doLocalInstall(installDir, localDir string, progressCh chan<- progressUpdate) error {
	defer close(progressCh)

	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return fmt.Errorf("create install directory: %w", err)
	}

	binaries := []struct {
		name     string
		progress int
	}{
		{"vortex", 30},
		{"vortex-window", 70},
	}

	for _, b := range binaries {
		src := filepath.Join(localDir, release.BinaryName(b.name))
		dst := filepath.Join(installDir, release.BinaryName(b.name))

		progressCh <- progressUpdate{fmt.Sprintf("Installing %s...", b.name), b.progress}

		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("local binary not found: %s", src)
		}

		time.Sleep(400 * time.Millisecond)

		if err := copyLocalFile(src, dst); err != nil {
			return fmt.Errorf("copy %s: %w", b.name, err)
		}
		if err := release.FinalizeInstall(dst); err != nil {
			return fmt.Errorf("finalize %s: %w", b.name, err)
		}
	}

	progressCh <- progressUpdate{"Configuring system...", 90}
	if err := platformPostInstall(installDir); err != nil {
		return fmt.Errorf("post-install: %w", err)
	}

	progressCh <- progressUpdate{"Installation complete!", 100}
	return nil
}

func copyLocalFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func showError(msg string) {
	fmt.Fprintf(os.Stderr, "vortex-install-gui: %s\n", msg)
}

// launchVortex starts vortex in UI mode.
func launchVortex() {
	installDir, err := release.ManagedInstallDir()
	if err != nil {
		return
	}
	bin := filepath.Join(installDir, release.BinaryName("vortex"))
	cmd := exec.Command(bin)
	cmd.Start()
}

const installerHTML = `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
  background: #1a1a2e;
  color: #e0e0e0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100vh;
  padding: 2rem;
  user-select: none;
}
h1 {
  font-size: 1.3rem;
  font-weight: 600;
  margin-bottom: 0.5rem;
  color: #fff;
}
.subtitle {
  font-size: 0.85rem;
  color: #a0a0c0;
  margin-bottom: 2rem;
}
.progress-container {
  width: 100%;
  max-width: 320px;
  background: #2a2a4a;
  border-radius: 6px;
  overflow: hidden;
  height: 8px;
  margin-bottom: 1rem;
}
.progress-bar {
  height: 100%;
  background: linear-gradient(90deg, #6366f1, #8b5cf6);
  width: 0%;
  transition: width 0.3s ease;
  border-radius: 6px;
}
.status {
  font-size: 0.85rem;
  color: #a0a0c0;
  text-align: center;
  margin-bottom: 1.5rem;
}
.btn {
  padding: 0.6rem 1.5rem;
  border: none;
  border-radius: 6px;
  font-size: 0.9rem;
  cursor: pointer;
  background: #6366f1;
  color: #fff;
  display: none;
}
.btn:hover { background: #5558e6; }
</style>
</head>
<body>
<h1>Installing Vortex</h1>
<p class="subtitle" id="version"></p>
<div class="progress-container">
  <div class="progress-bar" id="bar"></div>
</div>
<p class="status" id="status">Preparing...</p>
<button class="btn" id="launch" onclick="launchApp()">Launch Vortex</button>
<script>
const bar = document.getElementById('bar');
const status = document.getElementById('status');
const launchBtn = document.getElementById('launch');
const evtSource = new EventSource('/progress');
evtSource.onmessage = (e) => {
  const data = JSON.parse(e.data);
  status.textContent = data.step;
  if (data.progress >= 0) {
    bar.style.width = data.progress + '%';
  }
  if (data.progress === 100) {
    evtSource.close();
    launchBtn.style.display = 'inline-block';
  }
};
evtSource.onerror = () => { evtSource.close(); };
function launchApp() {
  fetch('/action?action=launch');
  window.close();
}
</script>
</body>
</html>`

const alreadyInstalledHTML = `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
  background: #1a1a2e;
  color: #e0e0e0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100vh;
  padding: 2rem;
  user-select: none;
}
h1 {
  font-size: 1.3rem;
  font-weight: 600;
  margin-bottom: 0.5rem;
  color: #fff;
}
.subtitle {
  font-size: 0.85rem;
  color: #a0a0c0;
  margin-bottom: 2rem;
}
.buttons {
  display: flex;
  gap: 0.75rem;
}
.btn {
  padding: 0.6rem 1.5rem;
  border: none;
  border-radius: 6px;
  font-size: 0.9rem;
  cursor: pointer;
}
.btn-primary {
  background: #6366f1;
  color: #fff;
}
.btn-primary:hover { background: #5558e6; }
.btn-secondary {
  background: #2a2a4a;
  color: #a0a0c0;
  border: 1px solid #3a3a5a;
}
.btn-secondary:hover { background: #3a3a5a; }
</style>
</head>
<body>
<h1>Vortex is already installed</h1>
<p class="subtitle">Would you like to reinstall?</p>
<div class="buttons">
  <button class="btn btn-primary" onclick="doAction('reinstall')">Reinstall</button>
  <button class="btn btn-secondary" onclick="doAction('cancel')">Cancel</button>
</div>
<script>
function doAction(action) {
  fetch('/action?action=' + action).then(() => {
    if (action === 'reinstall') {
      document.body.innerHTML = '<h1>Reinstalling...</h1><p class="subtitle">Please wait.</p>';
      // Redirect to progress view.
      setTimeout(() => { window.location.reload(); }, 500);
    } else {
      window.close();
    }
  });
}
</script>
</body>
</html>`
