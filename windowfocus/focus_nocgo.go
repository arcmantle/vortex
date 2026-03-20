//go:build !cgo

package windowfocus

import "unsafe"

func showApp() {}

func hideApp() {}

func focus(_ unsafe.Pointer) {}
