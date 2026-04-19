//go:build windows

package main

import (
	"log"
	"os"

	"golang.org/x/sys/windows"
)

const attachParentProcess = ^uint32(0)

var (
	procAttachConsole = kernel32DLL.NewProc("AttachConsole")

	// consoleHandles tracks file handles opened by attachParentConsole so
	// they can be closed at exit to avoid leaking OS handles.
	consoleHandles []*os.File
)

func prepareConsoleForCLI(args []string) {
	if !shouldAttachConsoleForCLI(args) || hasAttachedConsole() {
		return
	}
	if !attachParentConsole() {
		return
	}
	log.SetOutput(os.Stderr)
}

// CloseConsoleHandles closes the file handles opened for the attached parent
// console. It should be called before the process exits.
func CloseConsoleHandles() {
	for _, f := range consoleHandles {
		if f != nil {
			_ = f.Close()
		}
	}
	consoleHandles = nil
}

func attachParentConsole() bool {
	r1, _, _ := procAttachConsole.Call(uintptr(attachParentProcess))
	if r1 == 0 {
		return false
	}

	stdin, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err == nil {
		os.Stdin = stdin
		consoleHandles = append(consoleHandles, stdin)
	}

	stdout, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err == nil {
		os.Stdout = stdout
		_ = windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(stdout.Fd()))
		consoleHandles = append(consoleHandles, stdout)
	}

	stderr, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err == nil {
		os.Stderr = stderr
		_ = windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(stderr.Fd()))
		consoleHandles = append(consoleHandles, stderr)
	}

	return true
}

func cleanupConsole() {
	CloseConsoleHandles()
}
