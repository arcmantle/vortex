//go:build !cgo

package webview

import (
	"context"
	"log"
)

func openWithContext(_ context.Context, _ string, _ string, _ int, _ int, _ func(Controller)) {
	log.Printf("native webview unavailable: built without cgo; rebuild with CGO_ENABLED=1")
}
