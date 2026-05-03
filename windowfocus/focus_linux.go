//go:build linux && cgo

package windowfocus

/*
#cgo pkg-config: gtk+-3.0

// Implemented in focus_linux.c
extern void focusWindow(void *window);
*/
import "C"

import "unsafe"

func showApp() {}

func hideApp() {}

func focus(window unsafe.Pointer) {
	if window == nil {
		return
	}

	C.focusWindow(window)
}

func hideWindow(_ unsafe.Pointer) {}

func showWindow(_ unsafe.Pointer) {}
