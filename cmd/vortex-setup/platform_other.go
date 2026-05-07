//go:build !windows

package main

import (
	"os"
	"path/filepath"

	"arcmantle/vortex/internal/release"
)

// platformPostInstall configures PATH on non-Windows platforms.
func platformPostInstall(installDir string) error {
	_, err := release.EnsurePathEntry(installDir)
	return err
}

// ensureHostSymlink creates a "vortex" symlink in installDir pointing to the
// host binary ("vortex-host") so users can type "vortex" to invoke the host.
// On Unix the host binary is named "vortex-host" and the GUI binary is named
// "vortex" but installed to a separate directory; the symlink bridges the gap.
func ensureHostSymlink(installDir string) error {
	link := filepath.Join(installDir, release.ManagedHostSymlinkName())
	target := release.ManagedHostBinaryName()

	// Remove any existing file/symlink at the link path (could be an old GUI
	// binary or a stale symlink from a previous install).
	os.Remove(link)
	return os.Symlink(target, link)
}
