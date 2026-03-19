//go:build windows && cgo

package windowfocus

/*
#include <windows.h>

static void focusWindow(void *hwnd) {
	HWND window = (HWND)hwnd;
	if (!window) {
		return;
	}
	if (IsIconic(window)) {
		ShowWindow(window, SW_RESTORE);
	} else {
		ShowWindow(window, SW_SHOW);
	}
	BringWindowToTop(window);
	SetActiveWindow(window);
	SetForegroundWindow(window);
}
*/
import "C"

import "unsafe"

func focus(window unsafe.Pointer) {
	if window == nil {
		return
	}

	C.focusWindow(window)
}
