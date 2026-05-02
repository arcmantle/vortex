//go:build !windows

package main

import "arcmantle/vortex/internal/release"

// platformPostInstall configures PATH on non-Windows platforms.
func platformPostInstall(installDir string) error {
	_, err := release.EnsurePathEntry(installDir)
	return err
}
