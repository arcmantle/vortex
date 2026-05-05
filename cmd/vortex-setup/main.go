// vortex-setup is the unified installer/bootstrap for Vortex. On macOS it
// serves as the first-launch bootstrap inside Vortex.app; on Windows it is
// the GUI installer (built with -H=windowsgui to suppress the console).
//
// When vortex is already installed the behavior depends on the platform:
//   - macOS: silently launches the existing install (called on every .app open)
//   - Windows: shows a "reinstall or cancel" dialog
//
// Supports --uninstall mode (or invoked as uninstall.exe) on all platforms.
//
// Build:
//
//	go build -ldflags "-s -w -X main.Version=v1.0.0" ./cmd/vortex-setup
//	go build -ldflags "-s -w -X main.Version=v1.0.0 -H=windowsgui" ./cmd/vortex-setup  # Windows
package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
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
	if len(os.Args) > 1 && os.Args[1] == "--cleanup-self" {
		runCleanupHelper(os.Args[2:])
		return
	}

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

	installDir, err := release.ManagedInstallDir()
	if err != nil {
		showError(fmt.Sprintf("resolve install directory: %v", err))
		return
	}

	vortexHostBin := filepath.Join(installDir, release.ManagedHostBinaryName())
	vortexGUIBin := filepath.Join(installDir, release.ManagedGUIBinaryName())
	alreadyInstalled := fileExists(vortexHostBin) && fileExists(vortexGUIBin)

	// macOS: if already installed, skip the UI — just launch the GUI directly.
	// The GUI will spawn the host if needed. The launcher calls us on every
	// .app open, so this must be fast.
	if alreadyInstalled && runtime.GOOS == "darwin" {
		launchVortex(vortexGUIBin)
		return
	}

	runInstall(installDir, alreadyInstalled)
}

func runInstall(installDir string, alreadyInstalled bool) {
	localDir := resolveLocalDir()
	version := release.NormalizeVersion(Version)
	if localDir == "" && (version == "" || version == "dev") {
		showError("This binary was not built with a release version (set VORTEX_BOOTSTRAP_LOCAL to test locally).")
		return
	}

	// Start local HTTP server to serve the progress UI.
	mux := http.NewServeMux()
	progressCh := make(chan progressUpdate, 10)
	doneCh := make(chan error, 1)
	actionCh := make(chan string, 1)
	installCh := make(chan struct{}, 1)
	launched := false

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
		switch action {
		case "install":
			select {
			case installCh <- struct{}{}:
			default:
			}
		default:
			actionCh <- action
		}
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

	var controllerMu sync.Mutex
	var controller webview.Controller
	closeInstaller := func() {
		controllerMu.Lock()
		c := controller
		controllerMu.Unlock()
		if c != nil {
			c.Close()
			return
		}
		cancel()
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		waitForInstall := true

		if alreadyInstalled {
			// Wait for user's choice (Windows path — macOS skips this).
			action := <-actionCh
			switch action {
			case "reinstall":
				waitForInstall = false
			default:
				cancel()
				return
			}
		}

		// Wait for user to click "Install" or close the window.
		if waitForInstall {
			select {
			case <-installCh:
			case <-ctx.Done():
				return
			}
		}

		if localDir != "" {
			doneCh <- doLocalInstall(installDir, localDir, progressCh)
		} else {
			doneCh <- doInstall(installDir, version, progressCh)
		}
		// Wait for the user to click "Launch Vortex" or close the window.
		select {
		case action := <-actionCh:
			launched = action == "launch"
			closeInstaller()
		case <-ctx.Done():
		}
	}()

	url := fmt.Sprintf("http://%s/", addr)
	title := "Setting up Vortex"
	if runtime.GOOS == "windows" {
		title = "Install Vortex"
	}
	webview.OpenDialogWithContextAndReady(ctx, title, url, 460, 300, func(c webview.Controller) {
		controllerMu.Lock()
		controller = c
		controllerMu.Unlock()
	})
	cancel() // Window closed — unblock goroutine if it's waiting on ctx.Done().
	wg.Wait()

	// Launch only if the user explicitly clicked "Launch Vortex".
	if launched {
		select {
		case err := <-doneCh:
			if err != nil {
				showError(fmt.Sprintf("Installation failed: %v", err))
				return
			}
			vortexGUIBin := filepath.Join(installDir, release.ManagedGUIBinaryName())
			launchVortex(vortexGUIBin)
		default:
		}
	}
}

// ---------------------------------------------------------------------------
// Install logic
// ---------------------------------------------------------------------------

type progressUpdate struct {
	step     string
	progress int // 0-100
}

func doInstall(installDir, version string, progressCh chan<- progressUpdate) error {
	defer close(progressCh)

	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return fmt.Errorf("create install directory: %w", err)
	}

	progressCh <- progressUpdate{"Fetching release info...", 5}

	rel, err := release.FetchRelease("v"+version, "vortex-setup")
	if err != nil {
		return err
	}

	archiveName := release.ArchiveName(runtime.GOOS, runtime.GOARCH)

	assets := map[string]*release.ReleaseAsset{}
	for i := range rel.Assets {
		assets[rel.Assets[i].Name] = &rel.Assets[i]
	}

	archiveAsset := assets[archiveName]
	checksumAsset := assets[release.ChecksumAssetName]

	if archiveAsset == nil {
		return fmt.Errorf("release %s does not include %s", rel.TagName, archiveName)
	}
	if checksumAsset == nil {
		return fmt.Errorf("release %s does not include %s", rel.TagName, release.ChecksumAssetName)
	}

	progressCh <- progressUpdate{"Verifying checksums...", 10}
	checksums, err := release.FetchChecksums(checksumAsset.BrowserDownloadURL, "vortex-setup")
	if err != nil {
		return err
	}

	archiveChecksum := checksums[archiveName]
	if archiveChecksum == "" {
		return fmt.Errorf("checksum file does not contain entry for %s", archiveName)
	}

	progressCh <- progressUpdate{"Downloading Vortex...", 30}
	tmpArchive, actualChecksum, err := release.DownloadAsset(archiveAsset.BrowserDownloadURL, installDir, "vortex-setup")
	if err != nil {
		return fmt.Errorf("download %s: %w", archiveName, err)
	}
	defer os.Remove(tmpArchive)

	if actualChecksum != archiveChecksum {
		return fmt.Errorf("checksum mismatch for %s", archiveName)
	}

	progressCh <- progressUpdate{"Extracting...", 60}
	binaries := []struct {
		name     string
		target   string
		progress int
	}{
		{"vortex-host" + exeSuffix(), filepath.Join(installDir, release.ManagedHostBinaryName()), 70},
		{"vortex" + exeSuffix(), filepath.Join(installDir, release.ManagedGUIBinaryName()), 80},
	}

	extractNames := make([]string, len(binaries))
	for i, b := range binaries {
		extractNames[i] = b.name
	}
	extracted, err := release.ExtractBinaries(tmpArchive, installDir, extractNames)
	if err != nil {
		return fmt.Errorf("extract binaries: %w", err)
	}

	for _, b := range binaries {
		tmpPath, ok := extracted[b.name]
		if !ok {
			return fmt.Errorf("archive did not contain %s", b.name)
		}
		progressCh <- progressUpdate{fmt.Sprintf("Installing %s...", b.name), b.progress}
		if err := installBinary(tmpPath, b.target); err != nil {
			return fmt.Errorf("install %s: %w", b.name, err)
		}
	}

	if err := installWindowsUninstaller(installDir); err != nil {
		return err
	}

	// Platform-specific post-install (shortcuts, registry, PATH).
	progressCh <- progressUpdate{"Configuring system...", 90}
	if err := platformPostInstall(installDir); err != nil {
		return fmt.Errorf("post-install: %w", err)
	}

	progressCh <- progressUpdate{"Done!", 100}
	return nil
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// doLocalInstall copies pre-built binaries from a local directory instead of
// downloading from GitHub. Used for testing and .app bundles with embedded binaries.
func doLocalInstall(installDir, localDir string, progressCh chan<- progressUpdate) error {
	defer close(progressCh)

	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return fmt.Errorf("create install directory: %w", err)
	}

	binaries := []struct {
		name     string
		target   string
		progress int
	}{
		{"vortex-host", filepath.Join(installDir, release.ManagedHostBinaryName()), 30},
		{"vortex", filepath.Join(installDir, release.ManagedGUIBinaryName()), 70},
	}

	for _, b := range binaries {
		src := filepath.Join(localDir, release.BinaryName(b.name))
		dst := b.target

		progressCh <- progressUpdate{fmt.Sprintf("Installing %s...", b.name), b.progress}

		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("local binary not found: %s", src)
		}

		// Brief delay so the progress UI is visible.
		time.Sleep(400 * time.Millisecond)

		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("copy %s: %w", b.name, err)
		}
		if err := release.FinalizeInstall(dst); err != nil {
			return fmt.Errorf("finalize %s: %w", b.name, err)
		}
	}

	if err := installWindowsUninstaller(installDir); err != nil {
		return err
	}

	progressCh <- progressUpdate{"Configuring system...", 90}
	if err := platformPostInstall(installDir); err != nil {
		return fmt.Errorf("post-install: %w", err)
	}

	progressCh <- progressUpdate{"Done!", 100}
	return nil
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

func installWindowsUninstaller(installDir string) error {
	if runtime.GOOS != "windows" {
		return nil
	}

	selfPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate installer executable: %w", err)
	}

	uninstallPath := filepath.Join(installDir, "uninstall"+filepath.Ext(selfPath))
	if err := release.CopyFile(selfPath, uninstallPath); err != nil {
		return fmt.Errorf("install uninstaller: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// resolveLocalDir checks for a local binaries directory to install from
// instead of downloading from GitHub. Checks in order:
//  1. VORTEX_BOOTSTRAP_LOCAL env var
//  2. ../Resources/local-binaries/ relative to this executable (for macOS .app bundles)
func resolveLocalDir() string {
	if dir := os.Getenv("VORTEX_BOOTSTRAP_LOCAL"); dir != "" {
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}

	// Check bundle Resources directory (Contents/MacOS/../Resources/local-binaries/).
	if selfPath, err := os.Executable(); err == nil {
		bundled := filepath.Join(filepath.Dir(selfPath), "..", "Resources", "local-binaries")
		if _, err := os.Stat(bundled); err == nil {
			return bundled
		}
	}
	return ""
}

func copyFile(src, dst string) error {
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func showError(msg string) {
	fmt.Fprintf(os.Stderr, "vortex-setup: %s\n", msg)
}

// ---------------------------------------------------------------------------
// HTML templates
// ---------------------------------------------------------------------------

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
  -webkit-user-select: none;
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
  display: none;
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
  display: none;
}
.btn {
  padding: 0.6rem 1.5rem;
  border: none;
  border-radius: 6px;
  font-size: 0.9rem;
  cursor: pointer;
  margin-top: 1rem;
}
.btn-primary {
  background: #6366f1;
  color: #fff;
}
.btn-primary:hover { background: #5558e6; }
</style>
</head>
<body>
<h1>Install Vortex</h1>
<p class="subtitle" id="version"></p>
<div class="progress-container" id="progress-container">
  <div class="progress-bar" id="bar"></div>
</div>
<p class="status" id="status">Preparing...</p>
<button class="btn btn-primary" id="action-btn" onclick="doInstall()">Install</button>
<script>
requestAnimationFrame(() => {
	window.vortexAppReady?.();
});

const bar = document.getElementById('bar');
const status = document.getElementById('status');
const actionBtn = document.getElementById('action-btn');
const progressContainer = document.getElementById('progress-container');

function doInstall() {
  actionBtn.style.display = 'none';
  progressContainer.style.display = 'block';
  status.style.display = 'block';
  fetch('/action?action=install');
  const evtSource = new EventSource('/progress');
  evtSource.onmessage = (e) => {
    const data = JSON.parse(e.data);
    status.textContent = data.step;
    if (data.progress >= 0) {
      bar.style.width = data.progress + '%';
    }
    if (data.progress >= 100) {
      evtSource.close();
      actionBtn.textContent = 'Launch Vortex';
      actionBtn.onclick = doLaunch;
      actionBtn.style.display = 'inline-block';
      status.style.display = 'none';
    }
  };
  evtSource.onerror = () => {
    evtSource.close();
  };
}

function doLaunch() {
	fetch('/action?action=launch').then(() => {
		window.close();
	});
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
.progress-container {
	width: 100%;
	max-width: 320px;
	background: #2a2a4a;
	border-radius: 6px;
	overflow: hidden;
	height: 8px;
	margin-bottom: 1rem;
	display: none;
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
	display: none;
	margin-bottom: 1rem;
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
<div class="progress-container" id="progress-container">
	<div class="progress-bar" id="bar"></div>
</div>
<p class="status" id="status">Preparing...</p>
<div class="buttons" id="buttons">
	<button class="btn btn-primary" id="reinstall-btn" onclick="doReinstall()">Reinstall</button>
	<button class="btn btn-secondary" id="cancel-btn" onclick="doCancel()">Cancel</button>
</div>
<button class="btn btn-primary" id="launch-btn" onclick="doLaunch()" style="display:none;">Launch Vortex</button>
<script>
requestAnimationFrame(() => {
	window.vortexAppReady?.();
});

const bar = document.getElementById('bar');
const status = document.getElementById('status');
const buttons = document.getElementById('buttons');
const launchBtn = document.getElementById('launch-btn');
const reinstallBtn = document.getElementById('reinstall-btn');
const cancelBtn = document.getElementById('cancel-btn');
const progressContainer = document.getElementById('progress-container');

function doReinstall() {
	reinstallBtn.disabled = true;
	cancelBtn.disabled = true;
	buttons.style.display = 'none';
	progressContainer.style.display = 'block';
	status.style.display = 'block';
	fetch('/action?action=reinstall');

	const evtSource = new EventSource('/progress');
	evtSource.onmessage = (e) => {
		const data = JSON.parse(e.data);
		status.textContent = data.step;
		if (data.progress >= 0) {
			bar.style.width = data.progress + '%';
		}
		if (data.progress >= 100) {
			evtSource.close();
			status.style.display = 'none';
			launchBtn.style.display = 'inline-block';
		}
	};
	evtSource.onerror = () => {
		evtSource.close();
	};
}

function doLaunch() {
	fetch('/action?action=launch').then(() => {
		window.close();
	});
}

function doCancel() {
	fetch('/action?action=cancel').then(() => {
		window.close();
	});
}
</script>
</body>
</html>`
