//go:build darwin && cgo

package windowlifecycle

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#include <stdlib.h>

// Implemented in lifecycle_darwin.m
extern void vortexInstallAppDelegate(void);
extern void vortexSetAppName(const char *name);
extern void vortexInstallWindowDelegate(int hideOnClose);
extern void vortexShowMainWindow(void);
*/
import "C"

import "unsafe"

var eventChan chan Event

func configure(cfg Config) <-chan Event {
	eventChan = make(chan Event, 8)
	C.vortexInstallAppDelegate()
	if cfg.AppName != "" {
		cName := C.CString(cfg.AppName)
		C.vortexSetAppName(cName)
		C.free(unsafe.Pointer(cName))
	}
	return eventChan
}

func installWindowDelegate(hideOnClose bool) {
	v := C.int(0)
	if hideOnClose {
		v = C.int(1)
	}
	C.vortexInstallWindowDelegate(v)
}

func showWindow() {
	C.vortexShowMainWindow()
}

//export goAppkitWindowHidden
func goAppkitWindowHidden() {
	if eventChan != nil {
		select {
		case eventChan <- WindowHidden:
		default:
		}
	}
}

//export goAppkitReopenRequest
func goAppkitReopenRequest() {
	if eventChan != nil {
		select {
		case eventChan <- ReopenRequest:
		default:
		}
	}
}

//export goAppkitQuitRequest
func goAppkitQuitRequest() {
	if eventChan != nil {
		select {
		case eventChan <- QuitRequest:
		default:
		}
	}
}
