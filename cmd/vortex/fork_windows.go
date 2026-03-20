//go:build windows

package main

import (
	"log"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

const (
	windowsDetachedProcess        = 0x00000008
	windowsCreateNewProcGroup     = 0x00000200
	windowsCreateBreakawayFromJob = 0x01000000
)

var (
	kernel32DLL          = windows.NewLazySystemDLL("kernel32.dll")
	procGetConsoleWindow = kernel32DLL.NewProc("GetConsoleWindow")
	getConsoleMode       = windows.GetConsoleMode
)

// maybeFork respawns the process detached from any attached console when
// needed. Proper release builds use -H=windowsgui and skip this path because
// they already run without a console.
func maybeFork() bool {
	if !hasAttachedConsole() {
		return false
	}

	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("cannot find own executable: %v", err)
	}

	args := append(os.Args[1:], "--forked")
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windowsDetachedProcess | windowsCreateNewProcGroup | windowsCreateBreakawayFromJob,
		HideWindow:    true,
	}
	if err := cmd.Start(); err != nil {
		log.Fatalf("failed to detach: %v", err)
	}
	return true
}

func hasAttachedConsole() bool {
	hwnd, _, _ := procGetConsoleWindow.Call()
	if hwnd != 0 {
		return true
	}

	return fileHasConsole(os.Stdin) || fileHasConsole(os.Stdout) || fileHasConsole(os.Stderr)
}

func fileHasConsole(file *os.File) bool {
	if file == nil {
		return false
	}
	var mode uint32
	return getConsoleMode(windows.Handle(file.Fd()), &mode) == nil
}
