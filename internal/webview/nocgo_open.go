//go:build !cgo

package webview

import "context"

func openWithContext(_ context.Context, _ string, _ string, _ int, _ int, _ func(Controller)) {}
