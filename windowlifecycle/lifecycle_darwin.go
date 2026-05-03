//go:build darwin && cgo

package windowlifecycle

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

// Implemented in lifecycle_darwin.m
extern void vortexInstallAppDelegate(void);
extern void vortexInstallWindowDelegate(int hideOnClose);
extern void vortexShowMainWindow(void);
*/
import "C"

var eventChan chan Event

func configure(cfg Config) <-chan Event {
	eventChan = make(chan Event, 8)
	C.vortexInstallAppDelegate()
	_ = cfg // stored for InstallWindowDelegate
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
