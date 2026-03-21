//go:build windows

package main

import (
	"log"
	"os"

	"golang.org/x/sys/windows"
)

const attachParentProcess = ^uint32(0)

var procAttachConsole = kernel32DLL.NewProc("AttachConsole")

func prepareConsoleForCLI(args []string) {
	if !shouldAttachConsoleForCLI(args) || hasAttachedConsole() {
		return
	}
	if !attachParentConsole() {
		return
	}
	log.SetOutput(os.Stderr)
}

func attachParentConsole() bool {
	r1, _, _ := procAttachConsole.Call(uintptr(attachParentProcess))
	if r1 == 0 {
		return false
	}

	stdin, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err == nil {
		os.Stdin = stdin
	}

	stdout, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err == nil {
		os.Stdout = stdout
		_ = windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(stdout.Fd()))
	}

	stderr, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err == nil {
		os.Stderr = stderr
		_ = windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(stderr.Fd()))
	}

	return true
}