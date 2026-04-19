//go:build !windows

package instance

import (
	"os"
	"testing"
	"time"
)

func TestIsStaleProcessReturnsTrueForOwnPID(t *testing.T) {
	// Our own process should be reported as stale (alive and recent).
	pid := os.Getpid()
	startedAt := time.Now().UnixMilli()
	if !isStaleProcess(pid, startedAt) {
		t.Fatalf("isStaleProcess(%d, %d) = false, want true for own process", pid, startedAt)
	}
}

func TestIsStaleProcessReturnsFalseForInvalidPID(t *testing.T) {
	if isStaleProcess(0, time.Now().UnixMilli()) {
		t.Fatal("isStaleProcess(0, ...) = true, want false")
	}
	if isStaleProcess(-1, time.Now().UnixMilli()) {
		t.Fatal("isStaleProcess(-1, ...) = true, want false")
	}
}

func TestIsStaleProcessReturnsFalseForAncientMetadata(t *testing.T) {
	// Metadata older than 7 days should never allow a kill, even for our own PID.
	pid := os.Getpid()
	ancient := time.Now().Add(-8 * 24 * time.Hour).UnixMilli()
	if isStaleProcess(pid, ancient) {
		t.Fatalf("isStaleProcess(%d, ancient) = true, want false for old metadata", pid)
	}
}

func TestIsStaleProcessReturnsFalseForNonexistentPID(t *testing.T) {
	// Use an extremely high PID that almost certainly does not exist.
	fakePID := 4_000_000
	if isStaleProcess(fakePID, time.Now().UnixMilli()) {
		t.Fatalf("isStaleProcess(%d, now) = true, want false for nonexistent PID", fakePID)
	}
}
