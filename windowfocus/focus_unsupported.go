//go:build cgo && !darwin && !linux && !windows

package windowfocus

import "unsafe"

func focus(_ unsafe.Pointer) {}
