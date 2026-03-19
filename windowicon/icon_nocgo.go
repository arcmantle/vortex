//go:build !cgo

package windowicon

import "unsafe"

func set(_ unsafe.Pointer, _ []byte) {}
