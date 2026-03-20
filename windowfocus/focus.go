// Package windowfocus provides best-effort foreground activation for existing
// native windows on supported platforms.
//
// Callers must supply a native window handle and invoke Focus on the correct
// UI thread for their toolkit.
package windowfocus

import "unsafe"

// ShowApp promotes the current process to a foreground app on supported
// platforms so native windows appear normally.
func ShowApp() {
	showApp()
}

// HideApp demotes the current process back to a background app on supported
// platforms after its native windows have been dismissed.
func HideApp() {
	hideApp()
}

// Focus brings a native window to the foreground on supported platforms.
// Unsupported platforms and !cgo builds are no-ops.
func Focus(window unsafe.Pointer) {
	showApp()
	focus(window)
}
