//go:build darwin && cgo

package windowicon

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#include <Cocoa/Cocoa.h>

static void setWindowIconFromData(const void *data, int len) {
	NSData *imgData = [NSData dataWithBytes:data length:len];
	NSImage *img = [[NSImage alloc] initWithData:imgData];
	if (img) {
		[NSApp setApplicationIconImage:img];
	}
}
*/
import "C"

import "unsafe"

func set(_ unsafe.Pointer, icon []byte) {
	C.setWindowIconFromData(unsafe.Pointer(&icon[0]), C.int(len(icon)))
}
