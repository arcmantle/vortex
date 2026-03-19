// Package webview provides a thin wrapper around a native webview window.
// In dev mode the webview is never opened; the browser is used instead.
package webview

import "context"

var openWithContextImpl = func(context.Context, string, string, int, int) {}

// Open opens a native webview window pointing at url.
// It blocks until the window is closed.
func Open(title, url string, width, height int) {
	openWithContextImpl(context.Background(), title, url, width, height)
}

// OpenWithContext opens a native webview window and terminates it when ctx is canceled.
// On macOS this must be called from the main thread.
func OpenWithContext(ctx context.Context, title, url string, width, height int) {
	if ctx == nil {
		ctx = context.Background()
	}
	openWithContextImpl(ctx, title, url, width, height)
}
