// Package webview provides a thin wrapper around a native webview window.
// In dev mode the webview is never opened; the browser is used instead.
package webview

// Open opens a native webview window pointing at url.
// It blocks until the window is closed.
func Open(title, url string, width, height int) {
	open(title, url, width, height)
}
