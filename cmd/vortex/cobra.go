package main

import (
	"fmt"
	"strings"

	"arcmantle/vortex/internal/instance"
	"arcmantle/vortex/internal/upgrade"

	"github.com/spf13/cobra"
)

func rootCommand() *cobra.Command {
	var versionFlag bool
	var bareOpts cliOptions

	cmd := &cobra.Command{
		Use:           "vortex",
		Short:         "Terminal manager and job runner",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return unknownCommandError(cmd, args[0])
			}
			if versionFlag {
				printVersion()
				return nil
			}
			return runBareMode(bareOpts)
		},
	}

	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.Flags().BoolVarP(&versionFlag, "version", "v", false, "Show the current version")
	cmd.Flags().BoolVar(&bareOpts.dev, "dev", false, "Development mode (no native webview)")
	cmd.Flags().BoolVar(&bareOpts.headless, "headless", false, "Run without opening the native webview")
	cmd.Flags().IntVar(&bareOpts.port, "port", 0, "Override the HTTP port (default 7370)")
	cmd.Flags().BoolVar(&bareOpts.forked, "forked", false, "internal")
	_ = cmd.Flags().MarkHidden("forked")

	cmd.AddCommand(runCommand())
	cmd.AddCommand(stopCommand())
	cmd.AddCommand(configCommand())
	cmd.AddCommand(instanceCommand())
	cmd.AddCommand(favoriteCommand())
	cmd.AddCommand(initCommand())
	cmd.AddCommand(versionCommand())
	cmd.AddCommand(docsCommand())
	cmd.AddCommand(upgradeCommand())

	return cmd
}

func runCommand() *cobra.Command {
	var opts cliOptions

	cmd := &cobra.Command{
		Use:   "run [config-file]",
		Short: "Run a named Vortex config",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.positionals = append([]string(nil), args...)
			return executeRunCommand(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.dev, "dev", false, "Development mode for browser/Vite workflow")
	cmd.Flags().BoolVar(&opts.headless, "headless", false, "Run without opening the native webview")
	cmd.Flags().StringVar(&opts.cwd, "cwd", "", "Working directory for all jobs (defaults to the config file directory)")
	cmd.Flags().StringVar(&opts.configFile, "config", "", "Path to a Vortex config file")
	cmd.Flags().IntVar(&opts.port, "port", 0, "Override the deterministic HTTP port for this instance")
	cmd.Flags().BoolVar(&opts.forked, "forked", false, "internal")
	_ = cmd.Flags().MarkHidden("forked")
	return cmd
}

func instanceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "instance",
		Short: "Inspect and control named Vortex instances",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return unknownCommandError(cmd, args[0])
			}
			return cmd.Help()
		},
	}

	cmd.AddCommand(instanceListCommand())
	cmd.AddCommand(instanceQuitCommand())
	cmd.AddCommand(instanceKillCommand())
	cmd.AddCommand(instanceShowUICommand())
	cmd.AddCommand(instanceHideUICommand())
	cmd.AddCommand(instanceRerunCommand())
	return cmd
}

func unknownCommandError(cmd *cobra.Command, arg string) error {
	return fmt.Errorf("unknown command %q for %q", arg, cmd.CommandPath())
}

func validateCommandPath(root *cobra.Command, args []string) error {
	current := root
	for _, arg := range args {
		if arg == "--" {
			return nil
		}
		if arg == "help" {
			return nil
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if !current.HasAvailableSubCommands() {
			return nil
		}
		next := findSubcommand(current, arg)
		if next == nil {
			return unknownCommandError(current, arg)
		}
		current = next
	}
	return nil
}

func findSubcommand(cmd *cobra.Command, name string) *cobra.Command {
	for _, child := range cmd.Commands() {
		if child.Name() == name || child.HasAlias(name) {
			return child
		}
	}
	return nil
}

func versionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show build version information",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			printVersion()
		},
	}
}

func initCommand() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init [path]",
		Short: "Create a new Vortex config template",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) == 1 {
				path = args[0]
			}
			return runInitCommand(path, force)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing config file")
	return cmd
}

func docsCommand() *cobra.Command {
	var force bool
	var noOpen bool

	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Render local documentation",
		Long: `Render local Vortex documentation from the embedded README.

The generated file is versioned and written to:
~/Library/Caches/vortex/docs/<version>/index.html`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDocsCommand(force, noOpen)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Regenerate docs even if the file already exists")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "Generate docs but do not open the browser")
	return cmd
}

func upgradeCommand() *cobra.Command {
	var checkOnly bool

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Install or check for the latest Vortex release",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			upgradeArgs := []string{}
			if checkOnly {
				upgradeArgs = append(upgradeArgs, "--check")
			}
			return upgrade.Run(upgradeArgs, upgrade.Options{CurrentVersion: Version})
		},
	}

	cmd.Flags().BoolVar(&checkOnly, "check", false, "Show whether a newer release is available without installing it")
	return cmd
}

func stopCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop [name]",
		Short: "Stop a running Vortex instance",
		Long:  "Stop a running Vortex instance. With no name, stops the default singleton.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := bareInstanceName
			if len(args) == 1 {
				name = args[0]
			}
			identity, err := instance.NewIdentity(name)
			if err != nil {
				return err
			}
			return runQuitCommand(identity)
		},
	}
	return cmd
}

func instanceListCommand() *cobra.Command {
	var jsonOutput bool
	var prune bool

	cmd := &cobra.Command{
		Use:   "list [name]",
		Short: "List running Vortex instances",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := instancesCommandOptions{jsonOutput: jsonOutput, prune: prune}
			if len(args) == 1 {
				identity, err := instance.NewIdentity(args[0])
				if err != nil {
					return err
				}
				opts.filterName = identity.Name
			}
			return runInstancesCommandWithOptions(opts)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Write JSON output")
	cmd.Flags().BoolVar(&prune, "prune", false, "Remove stale instance entries")
	return cmd
}

func instanceQuitCommand() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "quit [name]",
		Short: "Ask a running Vortex instance to shut down",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			identity, err := resolveCommandIdentity(name, configPath, "usage: vortex instance quit <name>")
			if err != nil {
				return err
			}
			return runQuitCommand(identity)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "Resolve the target instance from a Vortex config file")
	return cmd
}

func instanceKillCommand() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "kill [name]",
		Short: "Terminate managed child processes for a running Vortex instance",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			identity, err := resolveCommandIdentity(name, configPath, "usage: vortex instance kill <name>")
			if err != nil {
				return err
			}
			return runKillCommand(identity)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "Resolve the target instance from a Vortex config file")
	return cmd
}

func instanceShowUICommand() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "show [name]",
		Short: "Show the native UI for a running Vortex instance",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			identity, err := resolveCommandIdentity(name, configPath, "usage: vortex instance show <name>")
			if err != nil {
				return err
			}
			return runShowUICommand(identity)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "Resolve the target instance from a Vortex config file")
	return cmd
}

func instanceHideUICommand() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "hide [name]",
		Short: "Hide the native UI for a running Vortex instance",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			identity, err := resolveCommandIdentity(name, configPath, "usage: vortex instance hide <name>")
			if err != nil {
				return err
			}
			return runHideUICommand(identity)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "Resolve the target instance from a Vortex config file")
	return cmd
}

func instanceRerunCommand() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "rerun <name> <job-id>",
		Short: "Rerun a job and its downstream dependents for a running Vortex instance",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			jobID := ""
			if configPath != "" {
				if len(args) != 1 {
					return cobra.MinimumNArgs(1)(cmd, args)
				}
				jobID = args[0]
			} else {
				if len(args) != 2 {
					return cobra.ExactArgs(2)(cmd, args)
				}
				name = args[0]
				jobID = args[1]
			}
			identity, err := resolveCommandIdentity(name, configPath, "usage: vortex instance rerun <name> <job-id>")
			if err != nil {
				return err
			}
			return runRerunCommand(identity, jobID)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "Resolve the target instance from a Vortex config file")
	return cmd
}
