//go:build windows && cgo

package windowfocus

/*
// Implemented in focus_windows.c
extern void focusWindow(void *hwnd);
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
