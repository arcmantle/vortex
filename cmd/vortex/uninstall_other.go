//go:build !windows

package main

import "arcmantle/vortex/internal/uninstall"

func scheduleWindowsUninstall(_ uninstall.Options) error { return nil }
func runUninstallCleanup(_ []string)                     {}
