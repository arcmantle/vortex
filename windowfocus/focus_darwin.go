//go:build darwin && cgo

package windowfocus

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

// Implemented in focus_darwin.m
extern void showApplication(void);
extern void hideApplication(void);
extern void focusWindow(void *windowPtr);
extern void hideNativeWindow(void *windowPtr);
extern void showNativeWindow(void *windowPtr);
*/
import "C"

import "unsafe"

func showApp() {
	C.showApplication()
}

func hideApp() {
	C.hideApplication()
}

func focus(window unsafe.Pointer) {
	if window == nil {
		return
	}

	C.focusWindow(window)
}

func hideWindow(window unsafe.Pointer) {
	if window == nil {
		return
	}
	C.hideNativeWindow(window)
}

func showWindow(window unsafe.Pointer) {
	if window == nil {
		return
	}
	C.showNativeWindow(window)
}
