//go:build !darwin

package main

import "arcmantle/vortex/internal/webview"

// platformLifecycle is a no-op on non-darwin platforms.
type platformLifecycle struct{}

func newPlatformLifecycle() *platformLifecycle {
	return &platformLifecycle{}
}

func (pl *platformLifecycle) beforeWebview(stop func())    {}
func (pl *platformLifecycle) onReady(c webview.Controller) {}
func (pl *platformLifecycle) show()                        {}
