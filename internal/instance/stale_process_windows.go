//go:build windows

package instance

import (
	"time"

	"golang.org/x/sys/windows"
)

// isStaleProcess reports whether pid likely belongs to the Vortex instance
// recorded at metaStartedAtMs.
//
// On Windows we can query the exact process creation time via
// GetProcessTimes. A process is considered stale (safe to kill) only when
// its creation time is no later than the recorded metadata start time plus
// a small tolerance. If we cannot open the process or read its times we
// return false to avoid killing an unrelated process.
func isStaleProcess(pid int, metaStartedAtMs int64) bool {
	if pid <= 0 {
		return false
	}

	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// Process does not exist or is inaccessible — nothing to kill.
		return false
	}
	defer windows.CloseHandle(handle)

	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return false
	}

	createdAt := time.Unix(0, creation.Nanoseconds())
	metaStartedAt := time.UnixMilli(metaStartedAtMs)

	// Allow a 5-second tolerance to account for clock skew between the
	// metadata write and the OS-reported process creation time.
	return createdAt.Before(metaStartedAt.Add(5 * time.Second))
}
