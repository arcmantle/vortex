//go:build !cgo

package webview

import "context"

func init() {
	openWithContextImpl = func(_ context.Context, _ string, _ string, _ int, _ int) {}
}
