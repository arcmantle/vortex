//go:build linux && cgo

package windowicon

/*
#cgo pkg-config: gtk+-3.0

// Implemented in icon_linux.c
extern void setWindowIconFromData(void *window, const void *data, int len);
*/
import "C"

import "unsafe"

func set(window unsafe.Pointer, icon []byte) {
	if window == nil {
		return
	}
	C.setWindowIconFromData(window, unsafe.Pointer(&icon[0]), C.int(len(icon)))
}
