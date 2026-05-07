//go:build windows

package uninstall

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"arcmantle/vortex/internal/release"

	"golang.org/x/sys/windows"
)

const (
	cleanupHelperName = "vortex-uninstall-cleanup.exe"
	cleanupRetries    = 40
	cleanupRetryDelay = 250 * time.Millisecond
	cleanupPIDTimeout = 30000 // milliseconds
)

// SpawnCleanupHelper copies the calling binary to a temp directory and spawns
// it as a detached process that will wait for this process to exit, then remove
// the given paths. The spawned process is invoked with:
//
//	--uninstall-cleanup <pid> <path1> <path2> ...
//
// The caller must intercept --uninstall-cleanup in its main() before normal
// argument parsing and call RunCleanupHelper(os.Args[2:]).
func SpawnCleanupHelper(paths []string) error {
	selfPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current executable: %w", err)
	}

	helperDir, err := os.MkdirTemp("", "vortex-uninstall-*")
	if err != nil {
		return fmt.Errorf("create cleanup helper dir: %w", err)
	}

	helperPath := filepath.Join(helperDir, cleanupHelperName)
	if err := release.CopyFile(selfPath, helperPath); err != nil {
		return fmt.Errorf("copy cleanup helper: %w", err)
	}

	args := append([]string{"--uninstall-cleanup", strconv.Itoa(os.Getpid())}, paths...)
	cmd := exec.Command(helperPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x00000008 | 0x08000000, // DETACHED | CREATE_NO_WINDOW
		HideWindow:    true,
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start cleanup helper: %w", err)
	}
	return nil
}

// RunCleanupHelper is the entry point for the detached helper process.
// args should be os.Args[2:] after the --uninstall-cleanup flag, i.e.:
//
//	<pid> <path1> <path2> ...
func RunCleanupHelper(args []string) {
	if len(args) < 1 {
		return
	}

	pid, err := strconv.Atoi(args[0])
	if err != nil {
		return
	}
	paths := args[1:]

	waitForProcessExit(pid)

	for _, path := range paths {
		retryRemove(path)
	}

	// Schedule self-deletion on reboot.
	selfPath, _ := os.Executable()
	if selfPath != "" {
		ScheduleRebootDelete(selfPath)
		ScheduleRebootDelete(filepath.Dir(selfPath))
	}
}

// ScheduleRebootDelete marks a path for deletion on next Windows reboot.
func ScheduleRebootDelete(path string) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	windows.MoveFileEx(p, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
}

func waitForProcessExit(pid int) {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		time.Sleep(2 * time.Second)
		return
	}
	defer windows.CloseHandle(handle)
	windows.WaitForSingleObject(handle, uint32(cleanupPIDTimeout))
}

func retryRemove(path string) {
	if path == "" {
		return
	}
	for range cleanupRetries {
		if err := os.RemoveAll(path); err == nil || os.IsNotExist(err) {
			return
		}
		time.Sleep(cleanupRetryDelay)
	}
}
