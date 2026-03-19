// Package webview provides a thin wrapper around a native webview window.
// In dev mode the webview is never opened; the browser is used instead.
package webview

import "context"

type Controller interface {
	Focus()
}

// Open opens a native webview window pointing at url.
// It blocks until the window is closed.
func Open(title, url string, width, height int) {
	openWithContext(context.Background(), title, url, width, height, nil)
}

// OpenWithContext opens a native webview window and terminates it when ctx is canceled.
// On macOS this must be called from the main thread.
func OpenWithContext(ctx context.Context, title, url string, width, height int) {
	OpenWithContextAndReady(ctx, title, url, width, height, nil)
}

// OpenWithContextAndReady opens a native webview window and reports a controller
// once the window exists so callers can bring it to the foreground later.
func OpenWithContextAndReady(ctx context.Context, title, url string, width, height int, onReady func(Controller)) {
	if ctx == nil {
		ctx = context.Background()
	}
	openWithContext(ctx, title, url, width, height, onReady)
}
