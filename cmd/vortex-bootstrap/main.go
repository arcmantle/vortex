// vortex-bootstrap is a macOS-only binary bundled inside Vortex.app that
// handles first-launch setup. When the vortex binary isn't yet installed in
// ~/.local/bin/, it shows a branded progress UI, downloads the release binaries,
// verifies checksums, installs them, and then launches the main app.
//
// The version is embedded at build time:
//
//	go build -ldflags "-X main.Version=v1.0.0" ./cmd/vortex-bootstrap
package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"arcmantle/vortex/internal/release"
	"arcmantle/vortex/internal/webview"
)

var Version = "dev"

func main() {
	installDir, err := release.ManagedInstallDir()
	if err != nil {
		fatal("resolve install directory: %v", err)
	}

	vortexBin := filepath.Join(installDir, release.BinaryName("vortex"))

	// If already installed, just launch directly — skip bootstrap.
	if fileExists(vortexBin) {
		launchVortex(vortexBin)
		return
	}

	// First launch: show progress UI and install.
	if err := bootstrap(installDir); err != nil {
		fatal("bootstrap failed: %v", err)
	}

	// Launch vortex after successful install.
	launchVortex(vortexBin)
}

func bootstrap(installDir string) error {
	localDir := resolveLocalDir()
	if localDir == "" {
		version := release.NormalizeVersion(Version)
		if version == "" || version == "dev" {
			return fmt.Errorf("this bootstrap binary was not built with a release version")
		}
	}

	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return fmt.Errorf("create install directory: %w", err)
	}

	// Start a local HTTP server to serve the progress UI.
	mux := http.NewServeMux()
	progressCh := make(chan progressUpdate, 10)
	doneCh := make(chan error, 1)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(bootstrapHTML))
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
		return fmt.Errorf("listen: %w", err)
	}
	addr := listener.Addr().String()
	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	defer server.Shutdown(context.Background())

	// Start install in background.
	go func() {
		if localDir != "" {
			doneCh <- doLocalInstall(installDir, localDir, progressCh)
		} else {
			doneCh <- doInstall(installDir, release.NormalizeVersion(Version), progressCh)
		}
	}()

	// Open webview — blocks until window is closed.
	// Use a goroutine to close the window when install completes.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := <-doneCh
		if err != nil {
			// On error, the webview stays open showing the error (handled in JS).
			return
		}
		// Success — close the window after a brief pause for the user to see completion.
		cancel()
	}()

	url := fmt.Sprintf("http://%s/", addr)
	webview.OpenWithContext(ctx, "Setting up Vortex", url, 420, 260)
	wg.Wait()

	return nil
}

type progressUpdate struct {
	step     string
	progress int // 0-100
}

func doInstall(installDir, version string, progressCh chan<- progressUpdate) error {
	defer close(progressCh)

	progressCh <- progressUpdate{"Fetching release info...", 5}

	rel, err := release.FetchRelease("v"+version, "vortex-bootstrap")
	if err != nil {
		progressCh <- progressUpdate{fmt.Sprintf("Error: %v", err), -1}
		return err
	}

	// Resolve assets.
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
		return fmt.Errorf("release %s does not include %s", rel.TagName, hostAssetName)
	}
	if windowAsset == nil {
		return fmt.Errorf("release %s does not include %s", rel.TagName, windowAssetName)
	}
	if checksumAsset == nil {
		return fmt.Errorf("release %s does not include %s", rel.TagName, release.ChecksumAssetName)
	}

	progressCh <- progressUpdate{"Verifying checksums...", 10}
	checksums, err := release.FetchChecksums(checksumAsset.BrowserDownloadURL, "vortex-bootstrap")
	if err != nil {
		return err
	}

	// Download and install both binaries.
	binaries := []struct {
		name     string
		asset    *release.ReleaseAsset
		checksum string
		target   string
		progress int
	}{
		{"vortex", hostAsset, checksums[hostAssetName], filepath.Join(installDir, release.BinaryName("vortex")), 50},
		{"vortex-window", windowAsset, checksums[windowAssetName], filepath.Join(installDir, release.BinaryName("vortex-window")), 85},
	}

	for _, b := range binaries {
		progressCh <- progressUpdate{fmt.Sprintf("Downloading %s...", b.name), b.progress}

		tmpPath, actualChecksum, err := release.DownloadAsset(b.asset.BrowserDownloadURL, installDir, "vortex-bootstrap")
		if err != nil {
			return fmt.Errorf("download %s: %w", b.name, err)
		}
		if actualChecksum != b.checksum {
			os.Remove(tmpPath)
			return fmt.Errorf("checksum mismatch for %s", b.name)
		}

		if err := os.Rename(tmpPath, b.target); err != nil {
			return fmt.Errorf("install %s: %w", b.name, err)
		}
		if err := release.FinalizeInstall(b.target); err != nil {
			return fmt.Errorf("finalize %s: %w", b.name, err)
		}
	}

	// Configure PATH.
	progressCh <- progressUpdate{"Configuring PATH...", 95}
	release.EnsurePathEntry(installDir)

	progressCh <- progressUpdate{"Done!", 100}
	return nil
}

// doLocalInstall copies pre-built binaries from a local directory instead of
// downloading from GitHub. Activated by VORTEX_BOOTSTRAP_LOCAL=/path/to/dir.
func doLocalInstall(installDir, localDir string, progressCh chan<- progressUpdate) error {
	defer close(progressCh)

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

		// Simulate a brief delay so the progress UI is visible.
		time.Sleep(400 * time.Millisecond)

		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("copy %s: %w", b.name, err)
		}
		if err := release.FinalizeInstall(dst); err != nil {
			return fmt.Errorf("finalize %s: %w", b.name, err)
		}
	}

	progressCh <- progressUpdate{"Configuring PATH...", 95}
	release.EnsurePathEntry(installDir)

	progressCh <- progressUpdate{"Done!", 100}
	return nil
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

	h := sha256.New()
	if _, err := io.Copy(out, io.TeeReader(in, h)); err != nil {
		return err
	}
	return out.Close()
}

func launchVortex(bin string) {
	cmd := exec.Command(bin)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Start()
	// Don't wait — let the launcher exit once vortex is running.
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "vortex-bootstrap: "+format+"\n", args...)
	os.Exit(1)
}

// resolveLocalDir checks for a local binaries directory to install from
// instead of downloading from GitHub. Checks in order:
//  1. VORTEX_BOOTSTRAP_LOCAL env var
//  2. ../Resources/local-binaries/ relative to this executable (for .app bundles)
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

const bootstrapHTML = `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
  font-family: -apple-system, BlinkMacSystemFont, sans-serif;
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
  font-size: 1.2rem;
  font-weight: 600;
  margin-bottom: 1.5rem;
  color: #fff;
}
.progress-container {
  width: 100%;
  max-width: 300px;
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
}
</style>
</head>
<body>
<h1>Setting up Vortex</h1>
<div class="progress-container">
  <div class="progress-bar" id="bar"></div>
</div>
<p class="status" id="status">Preparing...</p>
<script>
const bar = document.getElementById('bar');
const status = document.getElementById('status');
const evtSource = new EventSource('/progress');
evtSource.onmessage = (e) => {
  const data = JSON.parse(e.data);
  status.textContent = data.step;
  if (data.progress >= 0) {
    bar.style.width = data.progress + '%';
  }
};
evtSource.onerror = () => {
  evtSource.close();
};
</script>
</body>
</html>`
