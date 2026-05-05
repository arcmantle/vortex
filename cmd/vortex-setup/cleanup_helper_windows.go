//go:build windows

package main

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
	cleanupDetachedProcess       = 0x00000008
	cleanupCreateNewProcessGroup = 0x00000200
	cleanupCreateNoWindow        = 0x08000000
	cleanupHelperName            = "vortex-uninstall-cleanup.exe"
	cleanupDeleteAttempts        = 40
	cleanupDeleteDelay           = 250 * time.Millisecond
)

func launchCleanupHelper(selfPath, installDir string) error {
	helperDir, err := os.MkdirTemp("", "vortex-uninstall-*")
	if err != nil {
		return fmt.Errorf("create cleanup helper dir: %w", err)
	}

	helperPath := filepath.Join(helperDir, cleanupHelperName)
	if err := release.CopyFile(selfPath, helperPath); err != nil {
		return fmt.Errorf("copy cleanup helper: %w", err)
	}

	cmd := exec.Command(helperPath, "--cleanup-self", strconv.Itoa(os.Getpid()), selfPath, installDir)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: cleanupDetachedProcess | cleanupCreateNewProcessGroup | cleanupCreateNoWindow,
		HideWindow:    true,
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start cleanup helper: %w", err)
	}
	return nil
}

func runCleanupHelper(args []string) {
	if len(args) != 3 {
		return
	}

	parentPID, err := strconv.Atoi(args[0])
	if err != nil {
		return
	}
	installedUninstaller := args[1]
	installDir := args[2]

	_ = waitForProcessExit(parentPID)
	_ = removePathWithRetry(installedUninstaller)
	_ = removePathWithRetry(installDir)

	selfPath, err := os.Executable()
	if err != nil {
		return
	}
	_ = scheduleDeleteOnReboot(selfPath)
	_ = scheduleDeleteOnReboot(filepath.Dir(selfPath))
}

func waitForProcessExit(pid int) error {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(handle)

	result, err := windows.WaitForSingleObject(handle, windows.INFINITE)
	if err != nil {
		return err
	}
	if result != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("wait for uninstall process: %d", result)
	}
	return nil
}

func removePathWithRetry(path string) error {
	var lastErr error
	for range cleanupDeleteAttempts {
		lastErr = os.Remove(path)
		if lastErr == nil || os.IsNotExist(lastErr) {
			return nil
		}
		time.Sleep(cleanupDeleteDelay)
	}
	return lastErr
}