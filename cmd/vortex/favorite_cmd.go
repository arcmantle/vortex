package main

import (
	"fmt"
	"net/url"
	"path/filepath"

	"arcmantle/vortex/internal/favorites"

	"github.com/spf13/cobra"
)

func favoriteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "favorite",
		Aliases: []string{"fav"},
		Short:   "Save and manage favorite Vortex configs for quick access",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return unknownCommandError(cmd, args[0])
			}
			return cmd.Help()
		},
	}

	cmd.AddCommand(favoriteAddCommand())
	cmd.AddCommand(favoriteListCommand())
	cmd.AddCommand(favoriteRemoveCommand())
	return cmd
}

func favoriteAddCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "add <alias> <config-file>",
		Short: "Save a Vortex config as a favorite",
		Long:  "Save a Vortex config file under an alias so it can be run from anywhere with: vortex run @<alias>",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			alias, configPath := args[0], args[1]
			if err := favorites.Add(alias, configPath); err != nil {
				return err
			}
			abs, _ := filepath.Abs(configPath)
			fmt.Printf("Saved favorite %q → %s\n", alias, terminalClickablePath(abs))
			return nil
		},
	}
}

func favoriteListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all saved favorite configs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := favorites.List()
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Println("No favorites saved. Add one with: vortex favorite add <alias> <config-file>")
				return nil
			}
			// Find longest alias for alignment.
			maxLen := 0
			for _, e := range entries {
				if len(e.Alias) > maxLen {
					maxLen = len(e.Alias)
				}
			}
			for _, e := range entries {
				pathDisplay := e.Path
				uri := (&url.URL{Scheme: "file", Path: filepath.ToSlash(e.Path)}).String()
				link := fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", uri, pathDisplay)
				fmt.Printf("  @%-*s → %s\n", maxLen, e.Alias, link)
			}
			return nil
		},
	}
}

func favoriteRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "remove <alias>",
		Aliases: []string{"rm"},
		Short:   "Remove a saved favorite",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			alias := args[0]
			if err := favorites.Remove(alias); err != nil {
				return err
			}
			fmt.Printf("Removed favorite %q\n", alias)
			return nil
		},
	}
}
