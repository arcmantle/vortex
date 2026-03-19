//go:build cgo && !darwin && !linux && !windows

package windowicon

import "unsafe"

func set(_ unsafe.Pointer, _ []byte) {}
