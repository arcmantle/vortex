//go:build darwin && cgo

package windowicon

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

// Implemented in icon_darwin.m
extern void setWindowIconFromData(const void *data, int len);
*/
import "C"

import "unsafe"

func set(_ unsafe.Pointer, icon []byte) {
	C.setWindowIconFromData(unsafe.Pointer(&icon[0]), C.int(len(icon)))
}
