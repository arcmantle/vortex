package main

import (
	"fmt"
	"runtime"

	"arcmantle/vortex/internal/release"
	"arcmantle/vortex/internal/uninstall"

	"github.com/spf13/cobra"
)

func uninstallCommand() *cobra.Command {
	var removeConfig bool

	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove Vortex from this system",
		Long: `Remove Vortex binaries, the app bundle (macOS), and optionally config files.

This removes:
  - The vortex-host binary and vortex alias from the install directory
  - The GUI binary from its install directory
  - /Applications/Vortex.app (macOS)
  - WebKit/webview caches

Use --remove-config to also delete ~/.config/vortex/ (settings, themes, etc).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUninstallFromHost(removeConfig)
		},
	}

	cmd.Flags().BoolVar(&removeConfig, "remove-config", false, "Also remove configuration files (~/.config/vortex/)")
	return cmd
}

func runUninstallFromHost(removeConfig bool) error {
	installDir, err := release.ManagedInstallDir()
	if err != nil {
		return fmt.Errorf("resolve install directory: %w", err)
	}

	guiInstallDir, err := release.ManagedGUIInstallDir()
	if err != nil {
		return fmt.Errorf("resolve GUI install directory: %w", err)
	}

	opts := uninstall.Options{
		InstallDir:    installDir,
		GUIInstallDir: guiInstallDir,
		RemoveConfig:  removeConfig,
	}

	// On Windows, the running binary can't delete itself. Remove what we can
	// (registry, shortcuts), then delegate file deletion to a detached helper.
	if runtime.GOOS == "windows" {
		return scheduleWindowsUninstall(opts)
	}

	// Unix: straightforward removal.
	uninstall.Remove(opts)
	fmt.Println("Vortex has been uninstalled.")
	return nil
}
