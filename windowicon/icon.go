// Package windowicon provides best-effort icon assignment for existing native
// windows on supported platforms.
//
// Callers must supply a native window handle and invoke Set on the correct UI
// thread for their toolkit.
package windowicon

import "unsafe"

// Set makes a best-effort attempt to apply icon data to a native window on
// supported platforms. Unsupported platforms, !cgo builds, and empty icon data
// are no-ops.
func Set(window unsafe.Pointer, icon []byte) {
	if len(icon) == 0 {
		return
	}
	set(window, icon)
}
