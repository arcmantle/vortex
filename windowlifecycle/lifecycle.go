// Package windowlifecycle provides a Go event-channel API over macOS AppKit
// lifecycle delegates. On non-darwin platforms, Configure returns a closed
// channel and all operations are no-ops.
//
// This package is designed to cooperate with the webview library's NSApplication
// run loop. Call Configure() after the webview/NSApp is created but before
// Run() starts the event loop.
package windowlifecycle

// Event represents a lifecycle event emitted by AppKit.
type Event int

const (
	// WindowHidden is emitted when the user clicks the red close button.
	// The window is hidden (not destroyed) and the process stays alive.
	WindowHidden Event = iota

	// ReopenRequest is emitted when the user clicks the dock icon while
	// all windows are hidden.
	ReopenRequest

	// QuitRequest is emitted when the user presses Cmd+Q or selects Quit
	// from the dock context menu.
	QuitRequest
)

// String returns a human-readable name for the event.
func (e Event) String() string {
	switch e {
	case WindowHidden:
		return "WindowHidden"
	case ReopenRequest:
		return "ReopenRequest"
	case QuitRequest:
		return "QuitRequest"
	default:
		return "Unknown"
	}
}

// Config controls which AppKit lifecycle behaviors are installed.
type Config struct {
	// HideOnClose makes the red X hide the window instead of closing it.
	// The process stays alive for dock-click reopen.
	HideOnClose bool
}

// Configure installs AppKit lifecycle delegates. Must be called on the main
// thread before the webview run loop starts.
//
// Call sequence:
//  1. windowlifecycle.Configure(cfg)             — installs app delegate
//  2. webview.New()                              — creates window
//  3. windowlifecycle.InstallWindowDelegate()    — replaces window delegate
//  4. webview.Run()                              — starts run loop
func Configure(cfg Config) <-chan Event {
	return configure(cfg)
}

// InstallWindowDelegate must be called AFTER the webview window exists
// (after webview.New()) but before Run(). It replaces the window delegate
// to intercept close events.
func InstallWindowDelegate(hideOnClose bool) {
	installWindowDelegate(hideOnClose)
}

// ShowWindow makes the main window visible after it has been hidden.
func ShowWindow() {
	showWindow()
}
