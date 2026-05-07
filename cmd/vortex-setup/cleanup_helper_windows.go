//go:build windows

package main

import "arcmantle/vortex/internal/uninstall"

func runCleanupHelper(args []string) {
	uninstall.RunCleanupHelper(args)
}
