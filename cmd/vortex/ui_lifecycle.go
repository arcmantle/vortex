package main

import (
	"context"
	"log"
	"runtime"
	"sync"

	"arcmantle/vortex/internal/instance"
	"arcmantle/vortex/internal/webview"
	"arcmantle/windowfocus"
)

// uiLifecycle manages the native webview window lifecycle. It replaces the
// deeply-nested closures that previously lived in runWithOptions, making the
// open/close/focus logic testable and easier to follow.
type uiLifecycle struct {
	mu              sync.Mutex
	open            bool
	suppressStop    bool
	cancel          context.CancelFunc
	focusController func() bool
	uiThread        *uiThreadRunner
	identity        instance.Identity
	windowTitle     string
	windowURL       string
}

func newUILifecycle(identity instance.Identity, title, url string, uiThread *uiThreadRunner) *uiLifecycle {
	return &uiLifecycle{
		identity:        identity,
		windowTitle:     title,
		windowURL:       url,
		uiThread:        uiThread,
		focusController: func() bool { return false },
	}
}

// Open launches the native webview window. Returns false if the window is
// already open. When stopOnClose is true, the provided stop function is called
// when the user closes the window (unless the close was suppressed by Hide).
//
// On macOS the webview must run on the main thread, so this call blocks until
// the window is closed. On other platforms the work is dispatched to the
// dedicated UI thread and Open returns immediately.
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
	ui.focusController = func() bool { return false }
	if err := instance.SetUIState(ui.identity, "open"); err != nil {
		log.Printf("instance registry warning: %v", err)
	}
	ui.mu.Unlock()

	run := func() {
		webview.OpenWithContextAndReady(uiCtx, ui.windowTitle, ui.windowURL, 1280, 800, func(controller webview.Controller) {
			if controller == nil {
				return
			}
			ui.mu.Lock()
			if ui.open {
				ui.focusController = func() bool {
					ui.mu.Lock()
					defer ui.mu.Unlock()
					if !ui.open {
						return false
					}
					controller.Focus()
					return true
				}
			}
			ui.mu.Unlock()
		})

		ui.mu.Lock()
		suppress := ui.suppressStop
		ui.open = false
		ui.suppressStop = false
		ui.cancel = nil
		ui.focusController = func() bool { return false }
		ui.mu.Unlock()

		if suppress {
			windowfocus.HideApp()
			if err := instance.SetUIState(ui.identity, "hidden"); err != nil {
				log.Printf("instance registry warning: %v", err)
			}
		}

		if stopOnClose && !suppress && ctx.Err() == nil {
			stop()
		}
	}

	if runtime.GOOS == "darwin" {
		run()
		return true
	}
	ui.uiThread.Post(run)
	return true
}

// Close cancels the webview context, causing the window to close. When
// suppressStop is true, the window close will not trigger the stop function
// passed to Open (used for hide-ui). Returns false if the window is not open.
func (ui *uiLifecycle) Close(suppressStop bool) bool {
	ui.mu.Lock()
	if !ui.open {
		ui.mu.Unlock()
		return false
	}
	ui.suppressStop = suppressStop
	cancel := ui.cancel
	ui.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return true
}

// Focus brings an already-open window to the foreground. Returns false if the
// window is not open or does not yet have a controller.
func (ui *uiLifecycle) Focus() bool {
	ui.mu.Lock()
	fn := ui.focusController
	ui.mu.Unlock()
	return fn()
}

// IsOpen reports whether the window is currently open.
func (ui *uiLifecycle) IsOpen() bool {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	return ui.open
}
