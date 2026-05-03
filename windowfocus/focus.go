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

// HideWindow hides a native window without destroying it (orderOut on macOS).
// On platforms that don't support persistent hide, this is a no-op.
func HideWindow(window unsafe.Pointer) {
	hideWindow(window)
}

// ShowWindow makes a previously hidden native window visible and focused.
func ShowWindow(window unsafe.Pointer) {
	showWindow(window)
}
