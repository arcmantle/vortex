//go:build !windows

package uninstall

// SpawnCleanupHelper is not needed on Unix (the process can delete itself).
func SpawnCleanupHelper(_ []string) error { return nil }

// RunCleanupHelper is a no-op on Unix.
func RunCleanupHelper(_ []string) {}

// ScheduleRebootDelete is a no-op on Unix.
func ScheduleRebootDelete(_ string) {}
