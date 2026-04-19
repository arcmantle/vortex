//go:build !windows

package instance

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// isStaleProcess reports whether pid likely belongs to the Vortex instance
// recorded at metaStartedAtMs. It returns false (don't kill) when the
// process doesn't exist or when its identity cannot be verified.
//
// On Unix/Darwin we can only probe existence via signal 0. To reduce the
// risk of killing a reused PID we check that the metadata is recent enough
// (within the last 7 days). Ancient metadata almost certainly describes a
// long-gone process whose PID has been recycled.
func isStaleProcess(pid int, metaStartedAtMs int64) bool {
	if pid <= 0 {
		return false
	}

	// If the metadata is older than 7 days the recorded PIDs are almost
	// certainly stale and possibly reused — skip the kill.
	if metaStartedAtMs > 0 {
		age := time.Since(time.UnixMilli(metaStartedAtMs))
		if age > 7*24*time.Hour {
			return false
		}
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Signal 0 checks existence without affecting the process.
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true // process exists
	}

	// EPERM means the process exists but we lack permission — still a real
	// process, but killing it would also fail, so skip.
	if errors.Is(err, syscall.EPERM) {
		return false
	}

	// ESRCH or other error means the process is gone.
	return false
}
