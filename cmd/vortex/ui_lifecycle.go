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
	"runtime"
	"sync"

	"arcmantle/vortex/internal/instance"
)

var (
	resolveExecutablePath = os.Executable
	lookupWindowBinary    = exec.LookPath
)

// uiLifecycle manages the native webview window lifecycle by spawning a
// separate vortex gui process. This keeps the host binary as a
// console-subsystem app, avoiding -H=windowsgui issues on Windows.
type uiLifecycle struct {
	mu           sync.Mutex
	open         bool
	hidden       bool // child alive but window not visible (darwin hide-on-close)
	suppressStop bool
	cancel       context.CancelFunc
	stdinPipe    io.WriteCloser // send commands to vortex gui
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

// windowBinaryName returns the path to the vortex GUI executable.
// It looks next to the current executable first, then falls back to PATH.
func windowBinaryName() string {
	candidates := []string{"vortex"}
	if runtime.GOOS == "windows" {
		candidates = []string{"vortex-window", "vortex"}
	}

	self, err := resolveExecutablePath()
	if err == nil {
		dir := filepath.Dir(self)
		for _, base := range candidates {
			candidate := filepath.Join(dir, base)
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
			candidate += ".exe"
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}
	for _, base := range candidates {
		if candidate, err := lookupWindowBinary(base); err == nil || errors.Is(err, exec.ErrDot) {
			if filepath.IsAbs(candidate) {
				return candidate
			}
			if abs, absErr := filepath.Abs(candidate); absErr == nil {
				return abs
			}
			return candidate
		}
	}
	return candidates[0]
}

// Open launches the vortex GUI subprocess. Returns false if the window is
// already open. When stopOnClose is true, the provided stop function is called
// when the subprocess exits (unless the close was suppressed by Hide).
func (ui *uiLifecycle) Open(ctx context.Context, stop context.CancelFunc, stopOnClose bool) bool {
	ui.mu.Lock()
	if ui.open {
		// If the window is hidden (child alive, window not visible), unhide it.
		if ui.hidden && ui.stdinPipe != nil {
			pipe := ui.stdinPipe
			ui.mu.Unlock()
			fmt.Fprintln(pipe, "SHOW")
			return true
		}
		ui.mu.Unlock()
		return false
	}
	uiCtx, uiCancel := context.WithCancel(ctx)
	ui.open = true
	ui.hidden = false
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
		log.Printf("vortex gui stdin pipe error: %v", err)
		ui.markClosed(ctx, stop, stopOnClose)
		return
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("vortex gui stdout pipe error: %v", err)
		ui.markClosed(ctx, stop, stopOnClose)
		return
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("vortex gui stderr pipe error: %v", err)
		ui.markClosed(ctx, stop, stopOnClose)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("vortex gui start error: %v", err)
		ui.markClosed(ctx, stop, stopOnClose)
		return
	}

	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			log.Printf("vortex gui stderr: %s", scanner.Text())
		}
	}()

	// Read stdout continuously — the child sends lifecycle messages.
	readyCh := make(chan struct{}, 1)
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			msg := scanner.Text()
			switch msg {
			case "READY":
				ui.mu.Lock()
				ui.hidden = false
				ui.mu.Unlock()
				if err := instance.SetUIState(ui.identity, "open"); err != nil {
					log.Printf("instance registry warning: %v", err)
				}
				select {
				case readyCh <- struct{}{}:
				default:
				}
			case "HIDDEN":
				ui.handleChildHidden(ctx, stop, stopOnClose)
			default:
				log.Printf("vortex gui stdout: %s", msg)
			}
		}
	}()

	// Wait for READY (or child exit).
	select {
	case <-readyCh:
		// good
	case <-ctx.Done():
		log.Printf("vortex gui: context cancelled before READY")
	}

	ui.mu.Lock()
	ui.stdinPipe = stdinPipe
	ui.mu.Unlock()

	// Wait for the child to exit.
	if err := cmd.Wait(); err != nil {
		// Context cancellation is expected when we close the window.
		if ctx.Err() == nil {
			log.Printf("vortex gui exited: %v", err)
		}
	}

	ui.markClosed(ctx, stop, stopOnClose)
}

// handleChildHidden is called when the child sends HIDDEN (window was hidden,
// process is still alive). Transitions the UI to the hidden state.
func (ui *uiLifecycle) handleChildHidden(ctx context.Context, stop context.CancelFunc, stopOnClose bool) {
	ui.mu.Lock()
	ui.hidden = true
	ui.mu.Unlock()

	if err := instance.SetUIState(ui.identity, "hidden"); err != nil {
		log.Printf("instance registry warning: %v", err)
	}
}

func (ui *uiLifecycle) markClosed(ctx context.Context, stop context.CancelFunc, stopOnClose bool) {
	ui.mu.Lock()
	suppress := ui.suppressStop
	onClose := ui.onClose
	ui.open = false
	ui.hidden = false
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

// Close sends CLOSE to the vortex gui process. When suppressStop is true,
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

// Focus sends FOCUS to the vortex gui process. If the window is hidden,
// sends SHOW instead to unhide it. Returns false if the window is not open.
func (ui *uiLifecycle) Focus() bool {
	ui.mu.Lock()
	pipe := ui.stdinPipe
	isOpen := ui.open
	isHidden := ui.hidden
	ui.mu.Unlock()
	if !isOpen || pipe == nil {
		return false
	}
	if isHidden {
		_, err := fmt.Fprintln(pipe, "SHOW")
		return err == nil
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
