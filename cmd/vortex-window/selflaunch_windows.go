//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

const (
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
	stillActiveExitCode   = 259
)

// spawnHostDetached starts vortex-host without creating a console window.
// Runs headless since the GUI is the window.
func spawnHostDetached(bin string) error {
	cmd := exec.Command(bin, "--headless")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: detachedProcess | createNewProcessGroup | createNoWindow,
		HideWindow:    true,
	}
	return cmd.Start()
}

// processAlive checks if a process handle is still valid.
func processAlive(proc *os.Process) bool {
	if proc == nil || proc.Pid <= 0 {
		return false
	}

	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(proc.Pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)

	var exitCode uint32
	if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false
	}
	return exitCode == stillActiveExitCode
}
