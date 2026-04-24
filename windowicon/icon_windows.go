//go:build windows && cgo

package windowicon

/*
#cgo LDFLAGS: -lgdiplus -lole32

#include "icon_windows_helper.h"
*/
import "C"

import "unsafe"

func set(window unsafe.Pointer, icon []byte) {
	if window == nil {
		return
	}
	C.setWindowIconFromData(window, unsafe.Pointer(&icon[0]), C.int(len(icon)))
}
