//go:build cgo && !darwin && !linux && !windows

package windowfocus

import "unsafe"

func showApp() {}

func hideApp() {}

func focus(_ unsafe.Pointer) {}
