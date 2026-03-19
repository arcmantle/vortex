// Package windowfocus provides best-effort foreground activation for existing
// native windows on supported platforms.
//
// Callers must supply a native window handle and invoke Focus on the correct
// UI thread for their toolkit.
package windowfocus

import "unsafe"

// Focus brings a native window to the foreground on supported platforms.
// Unsupported platforms and !cgo builds are no-ops.
func Focus(window unsafe.Pointer) {
	focus(window)
}
