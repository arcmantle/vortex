//go:build windows

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"

	"arcmantle/vortex/internal/release"
	"arcmantle/vortex/internal/uninstall"
	"arcmantle/vortex/internal/webview"
)

// runUninstall shows a webview uninstall confirmation dialog and removes
// binaries, shortcuts, and registry entries on Windows.
func runUninstall() {
	installDir, err := release.ManagedInstallDir()
	if err != nil {
		showError(fmt.Sprintf("resolve install directory: %v", err))
		return
	}

	guiInstallDir, _ := release.ManagedGUIInstallDir()

	// Check --silent flag for non-interactive uninstall.
	silent := false
	removeConfig := false
	for _, arg := range os.Args[1:] {
		if arg == "--silent" {
			silent = true
		}
		if arg == "--remove-config" {
			removeConfig = true
		}
	}

	opts := uninstall.Options{
		InstallDir:    installDir,
		GUIInstallDir: guiInstallDir,
		RemoveConfig:  removeConfig,
	}

	if silent {
		performUninstall(opts)
		return
	}

	// Show webview UI for interactive uninstall.
	actionCh := make(chan uninstallAction, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(uninstallHTML))
	})
	mux.HandleFunc("/action", func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")
		config := r.URL.Query().Get("config") == "true"
		actionCh <- uninstallAction{action: action, removeConfig: config}
		w.WriteHeader(http.StatusOK)
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		showError(fmt.Sprintf("listen: %v", err))
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
	closeDialog := func() {
		controllerMu.Lock()
		c := controller
		controllerMu.Unlock()
		if c != nil {
			c.Close()
			return
		}
		cancel()
	}

	go func() {
		ua := <-actionCh
		if ua.action == "confirm" {
			opts.RemoveConfig = ua.removeConfig
			performUninstall(opts)
		}
		closeDialog()
	}()

	url := fmt.Sprintf("http://%s/", addr)
	webview.OpenDialogWithContextAndReady(ctx, "Uninstall Vortex", url, 420, 240, func(c webview.Controller) {
		controllerMu.Lock()
		controller = c
		controllerMu.Unlock()
	})
}

type uninstallAction struct {
	action       string
	removeConfig bool
}

func performUninstall(opts uninstall.Options) {
	// Remove everything we can while running (binaries, registry, shortcuts, caches).
	uninstall.Remove(opts)

	// We can't delete ourselves (vortex-setup.exe) while running.
	// Spawn a detached cleanup helper to remove the remaining locked files.
	paths := uninstall.AllRemovalPaths(opts)
	// Also include webview data left by setup itself.
	paths = append(paths, uninstall.WebviewCachePaths()...)

	selfPath, _ := os.Executable()
	if selfPath != "" {
		paths = append(paths, selfPath)
	}

	if err := uninstall.SpawnCleanupHelper(paths); err != nil {
		fmt.Fprintf(os.Stderr, "warning: schedule self-delete failed: %v\n", err)
	}
}

const uninstallHTML = `<!DOCTYPE html>
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
  font-size: 1.2rem;
  font-weight: 600;
  margin-bottom: 1.5rem;
  color: #fff;
}
.option {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 1.5rem;
  font-size: 0.85rem;
  color: #a0a0c0;
}
.option input { accent-color: #6366f1; }
.buttons { display: flex; gap: 0.75rem; }
.btn {
  padding: 0.6rem 1.5rem;
  border: none;
  border-radius: 6px;
  font-size: 0.9rem;
  cursor: pointer;
}
.btn-danger { background: #dc2626; color: #fff; }
.btn-danger:hover { background: #b91c1c; }
.btn-secondary {
  background: #2a2a4a;
  color: #a0a0c0;
  border: 1px solid #3a3a5a;
}
.btn-secondary:hover { background: #3a3a5a; }
</style>
</head>
<body>
<h1>Uninstall Vortex?</h1>
<label class="option">
  <input type="checkbox" id="config">
  Also remove configuration and data
</label>
<div class="buttons">
  <button class="btn btn-danger" onclick="doUninstall()">Uninstall</button>
  <button class="btn btn-secondary" onclick="doCancel()">Cancel</button>
</div>
<script>
requestAnimationFrame(() => {
	window.vortexAppReady?.();
});

function doUninstall() {
  const config = document.getElementById('config').checked;
  fetch('/action?action=confirm&config=' + config).then(() => window.close());
}
function doCancel() {
  fetch('/action?action=cancel').then(() => window.close());
}
</script>
</body>
</html>`
