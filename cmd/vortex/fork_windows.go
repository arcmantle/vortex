//go:build windows

package main

// maybeFork is a no-op on Windows. The binary is built with -H=windowsgui
// which makes it a GUI subsystem application — no console is allocated and
// the launching terminal returns immediately.
// Returns false because no fork occurred.
func maybeFork() bool { return false }
