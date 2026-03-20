package main

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"arcmantle/vortex/internal/settings"

	"github.com/spf13/cobra"
)

const (
	configKeyBrowser = "browser"
	configKeyEditor  = "editor"
)

func configCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Read and update Vortex user settings",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return unknownCommandError(cmd, args[0])
			}
			return cmd.Help()
		},
	}

	cmd.AddCommand(configGetCommand())
	cmd.AddCommand(configListCommand())
	cmd.AddCommand(configSetCommand())
	cmd.AddCommand(configUnsetCommand())
	return cmd
}

func configListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all Vortex user settings",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := settings.Load()
			if err != nil {
				return err
			}
			return printConfigValues(cfg)
		},
	}
	return cmd
}

func configGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [key]",
		Short: "Show Vortex user settings",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := settings.Load()
			if err != nil {
				return err
			}
			if len(args) == 0 {
				return printConfigValues(cfg)
			}

			value, err := configValue(cfg, args[0])
			if err != nil {
				return err
			}
			fmt.Println(value)
			return nil
		},
	}
	return cmd
}

func configSetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Persist a Vortex user setting",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := settings.Load()
			if err != nil {
				return err
			}
			if err := setConfigValue(&cfg, args[0], args[1]); err != nil {
				return err
			}
			if err := settings.Save(cfg); err != nil {
				return err
			}
			fmt.Printf("Set %s=%s\n", normalizeConfigKey(args[0]), args[1])
			return nil
		},
	}
	return cmd
}

func configUnsetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unset <key>",
		Short: "Clear a persisted Vortex user setting",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := settings.Load()
			if err != nil {
				return err
			}
			if err := unsetConfigValue(&cfg, args[0]); err != nil {
				return err
			}
			if err := settings.Save(cfg); err != nil {
				return err
			}
			fmt.Printf("Unset %s\n", normalizeConfigKey(args[0]))
			return nil
		},
	}
	return cmd
}

func configValue(cfg settings.Settings, key string) (string, error) {
	switch normalizeConfigKey(key) {
	case configKeyBrowser:
		return cfg.Browser, nil
	case configKeyEditor:
		return cfg.Editor, nil
	default:
		return "", fmt.Errorf("unsupported config key %q", key)
	}
}

func setConfigValue(cfg *settings.Settings, key, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("config value for %q must not be empty", normalizeConfigKey(key))
	}

	switch normalizeConfigKey(key) {
	case configKeyBrowser:
		cfg.Browser = trimmed
		return nil
	case configKeyEditor:
		cfg.Editor = trimmed
		return nil
	default:
		return fmt.Errorf("unsupported config key %q", key)
	}
}

func unsetConfigValue(cfg *settings.Settings, key string) error {
	switch normalizeConfigKey(key) {
	case configKeyBrowser:
		cfg.Browser = ""
		return nil
	case configKeyEditor:
		cfg.Editor = ""
		return nil
	default:
		return fmt.Errorf("unsupported config key %q", key)
	}
}

func normalizeConfigKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func printConfigValues(cfg settings.Settings) error {
	if err := settings.EnsureDir(); err != nil {
		return err
	}
	path, err := settings.Path()
	if err != nil {
		return err
	}
	fmt.Printf("dir=%s\n", filepath.Dir(path))
	fmt.Printf("%s=%s\n", configKeyBrowser, cfg.Browser)
	fmt.Printf("%s=%s\n", configKeyEditor, cfg.Editor)
	return nil
}

func terminalClickablePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	absolute := trimmed
	if !filepath.IsAbs(absolute) {
		resolved, err := filepath.Abs(absolute)
		if err == nil {
			absolute = resolved
		}
	}
	uri := (&url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}).String()
	return fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", uri, absolute)
}
