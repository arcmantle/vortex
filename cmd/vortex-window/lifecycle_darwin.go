//go:build darwin

package main

import (
	"fmt"
	"os"
	"sync"

	"arcmantle/vortex/internal/webview"
	"arcmantle/windowlifecycle"
)

// darwinLifecycle manages macOS-specific lifecycle behavior:
// - Hide-on-close instead of exit
// - Dock click to reopen
// - Cmd+Q sends CLOSE to parent
type darwinLifecycle struct {
	mu         sync.Mutex
	controller webview.Controller
	hidden     bool
}

func newPlatformLifecycle() *darwinLifecycle {
	return &darwinLifecycle{}
}

// beforeWebview installs AppKit delegates. Must be called before webview.New().
func (dl *darwinLifecycle) beforeWebview(stop func()) {
	events := windowlifecycle.Configure(windowlifecycle.Config{HideOnClose: true})
	dl.handleEvents(events, stop)
}

// onReady installs the window delegate and stores the controller.
// Must be called after webview.New() creates the window.
func (dl *darwinLifecycle) onReady(c webview.Controller) {
	windowlifecycle.InstallWindowDelegate(true)
	dl.mu.Lock()
	dl.controller = c
	dl.mu.Unlock()
}

// show makes the window visible again and signals the parent.
func (dl *darwinLifecycle) show() {
	dl.mu.Lock()
	c := dl.controller
	dl.hidden = false
	dl.mu.Unlock()

	if c != nil {
		c.Show()
	} else {
		windowlifecycle.ShowWindow()
	}
	fmt.Fprintln(os.Stdout, "READY")
}

// handleEvents processes appkit lifecycle events in a goroutine.
func (dl *darwinLifecycle) handleEvents(events <-chan windowlifecycle.Event, stop func()) {
	go func() {
		for ev := range events {
			switch ev {
			case windowlifecycle.WindowHidden:
				dl.mu.Lock()
				dl.hidden = true
				dl.mu.Unlock()
				fmt.Fprintln(os.Stdout, "HIDDEN")
			case windowlifecycle.ReopenRequest:
				dl.show()
			case windowlifecycle.QuitRequest:
				stop()
			}
		}
	}()
}
