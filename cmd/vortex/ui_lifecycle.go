package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"arcmantle/vortex/internal/instance"
)

var (
	resolveExecutablePath = os.Executable
	lookupWindowBinary    = exec.LookPath
)

// uiLifecycle manages the native webview window lifecycle by spawning a
// separate vortex-window process. This keeps the host binary as a
// console-subsystem app, avoiding -H=windowsgui issues on Windows.
type uiLifecycle struct {
	mu           sync.Mutex
	open         bool
	suppressStop bool
	cancel       context.CancelFunc
	stdinPipe    io.WriteCloser // send commands to vortex-window
	identity     instance.Identity
	windowTitle  string
	windowURL    string
	onClose      func() // called when the window closes (nil-safe)
}

func newUILifecycle(identity instance.Identity, title, url string) *uiLifecycle {
	return &uiLifecycle{
		identity:    identity,
		windowTitle: title,
		windowURL:   url,
	}
}

// windowBinaryName returns the path to the vortex-window executable.
// It looks next to the current executable first, then falls back to PATH.
func windowBinaryName() string {
	self, err := resolveExecutablePath()
	if err == nil {
		dir := filepath.Dir(self)
		candidate := filepath.Join(dir, "vortex-window")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		candidate += ".exe"
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if candidate, err := lookupWindowBinary("vortex-window"); err == nil || errors.Is(err, exec.ErrDot) {
		if filepath.IsAbs(candidate) {
			return candidate
		}
		if abs, absErr := filepath.Abs(candidate); absErr == nil {
			return abs
		}
		return candidate
	}
	return "vortex-window"
}

// Open launches the vortex-window subprocess. Returns false if the window is
// already open. When stopOnClose is true, the provided stop function is called
// when the subprocess exits (unless the close was suppressed by Hide).
func (ui *uiLifecycle) Open(ctx context.Context, stop context.CancelFunc, stopOnClose bool) bool {
	ui.mu.Lock()
	if ui.open {
		ui.mu.Unlock()
		return false
	}
	uiCtx, uiCancel := context.WithCancel(ctx)
	ui.open = true
	ui.suppressStop = false
	ui.cancel = uiCancel
	ui.stdinPipe = nil
	if err := instance.SetUIState(ui.identity, "open"); err != nil {
		log.Printf("instance registry warning: %v", err)
	}
	ui.mu.Unlock()

	run := func() {
		ui.runWindowProcess(uiCtx, stop, stopOnClose)
	}

	// Always run in a goroutine — no more platform-specific main-thread
	// requirements since the webview is in a separate process.
	go run()
	return true
}

func (ui *uiLifecycle) runWindowProcess(ctx context.Context, stop context.CancelFunc, stopOnClose bool) {
	bin := windowBinaryName()
	cmd := exec.CommandContext(ctx, bin,
		"--title", ui.windowTitle,
		"--url", ui.windowURL,
		"--width", "1280",
		"--height", "800",
	)

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		log.Printf("vortex-window stdin pipe error: %v", err)
		ui.markClosed(ctx, stop, stopOnClose)
		return
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("vortex-window stdout pipe error: %v", err)
		ui.markClosed(ctx, stop, stopOnClose)
		return
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("vortex-window stderr pipe error: %v", err)
		ui.markClosed(ctx, stop, stopOnClose)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("vortex-window start error: %v", err)
		ui.markClosed(ctx, stop, stopOnClose)
		return
	}

	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			log.Printf("vortex-window stderr: %s", scanner.Text())
		}
	}()

	// Wait for the READY signal from the child.
	scanner := bufio.NewScanner(stdoutPipe)
	ready := false
	for scanner.Scan() {
		if scanner.Text() == "READY" {
			ready = true
			break
		}
	}
	if !ready {
		log.Printf("vortex-window: did not receive READY signal")
	}

	ui.mu.Lock()
	ui.stdinPipe = stdinPipe
	ui.mu.Unlock()

	// Wait for the child to exit.
	if err := cmd.Wait(); err != nil {
		// Context cancellation is expected when we close the window.
		if ctx.Err() == nil {
			log.Printf("vortex-window exited: %v", err)
		}
	}

	ui.markClosed(ctx, stop, stopOnClose)
}

func (ui *uiLifecycle) markClosed(ctx context.Context, stop context.CancelFunc, stopOnClose bool) {
	ui.mu.Lock()
	suppress := ui.suppressStop
	onClose := ui.onClose
	ui.open = false
	ui.suppressStop = false
	ui.cancel = nil
	if ui.stdinPipe != nil {
		ui.stdinPipe.Close()
		ui.stdinPipe = nil
	}
	ui.mu.Unlock()

	if suppress {
		if err := instance.SetUIState(ui.identity, "hidden"); err != nil {
			log.Printf("instance registry warning: %v", err)
		}
	}

	// Invoke onClose for non-suppressed window closes (real user action).
	if !suppress && onClose != nil {
		onClose()
	}

	if stopOnClose && !suppress && ctx.Err() == nil {
		stop()
	}
}

// Close sends CLOSE to the vortex-window process. When suppressStop is true,
// the window close will not trigger the stop function passed to Open (used for
// hide-ui). Returns false if the window is not open.
func (ui *uiLifecycle) Close(suppressStop bool) bool {
	ui.mu.Lock()
	if !ui.open {
		ui.mu.Unlock()
		return false
	}
	ui.suppressStop = suppressStop
	cancel := ui.cancel
	pipe := ui.stdinPipe
	ui.mu.Unlock()

	// Send CLOSE command; if that fails, cancel the context to force-kill.
	if pipe != nil {
		fmt.Fprintln(pipe, "CLOSE")
	}
	if cancel != nil {
		cancel()
	}
	return true
}

// Focus sends FOCUS to the vortex-window process. Returns false if the window
// is not open.
func (ui *uiLifecycle) Focus() bool {
	ui.mu.Lock()
	pipe := ui.stdinPipe
	isOpen := ui.open
	ui.mu.Unlock()
	if !isOpen || pipe == nil {
		return false
	}
	_, err := fmt.Fprintln(pipe, "FOCUS")
	return err == nil
}

// IsOpen reports whether the window is currently open.
func (ui *uiLifecycle) IsOpen() bool {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	return ui.open
}
