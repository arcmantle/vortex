//go:build !windows

package uninstall

// removePlatformArtifacts is a no-op on non-Windows (no registry/shortcuts).
func removePlatformArtifacts(_ Options) {}
