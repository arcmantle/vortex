// Package config handles loading and validating Vortex configuration files.
//
// Example dev.vortex:
//
//	jobs:
//	  - id: build
//	    label: Build
//	    command: go build ./...
//	    group: ci
//
//	  - id: test
//	    label: Test
//	    command: go test ./...
//	    group: ci
//	    needs: [build]
//
//	  - id: deploy
//	    label: Deploy
//	    command: ./deploy.sh
//	    group: deploy
//	    needs: [test]
//	    if: success   # "success" (default) | "failure" | "always"
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// JobSpec describes a single job in the config.
type JobSpec struct {
	// ID is the unique job identifier (used in needs references and SSE URLs).
	ID string `yaml:"id"`
	// Label is the human-readable display name shown in the UI.
	Label string `yaml:"label"`
	// Shell optionally selects an interpreter for command script blocks.
	// Examples: "bash", "zsh", "pwsh", "cmd", "python", "node".
	Shell string `yaml:"shell"`
	// Command is the full shell command including args, space-separated.
	// When Shell is unset this is split into argv directly.
	// When Shell is set this is passed as a script string to that interpreter.
	Command string `yaml:"command"`
	// Group optionally places this job under a named group in the UI.
	Group string `yaml:"group"`
	// Needs lists IDs of jobs that must complete before this one starts.
	Needs []string `yaml:"needs"`
	// If controls when this job runs relative to its needs.
	// "success" (default): run only if all needs succeeded.
	// "failure": run only if any need failed.
	// "always": run regardless of need outcomes.
	If string `yaml:"if"`
	// Restart controls whether the job is killed and re-launched on restart.
	// Defaults to true. Set to false for long-running processes (e.g. dev
	// servers) that should survive across config reloads.
	Restart *bool `yaml:"restart"`
}

// ShouldRestart returns whether this job should be killed and re-launched on
// restart. Defaults to true when the field is not set.
func (j JobSpec) ShouldRestart() bool {
	return j.Restart == nil || *j.Restart
}

// DisplayCommand returns the human-readable command shown in the UI.
func (j JobSpec) DisplayCommand() string {
	if strings.TrimSpace(j.Shell) == "" {
		return j.Command
	}
	return strings.TrimSpace(j.Shell) + ": " + j.Command
}

// CommandLine resolves the executable and argv used to launch the job.
func (j JobSpec) CommandLine() (string, []string, error) {
	script := strings.TrimSpace(j.Command)
	if script == "" {
		return "", nil, fmt.Errorf("command is required")
	}

	shell := normalizeShellName(j.Shell)
	if shell == "" {
		parts := strings.Fields(script)
		if len(parts) == 0 {
			return "", nil, fmt.Errorf("command is required")
		}
		return parts[0], parts[1:], nil
	}

	switch shell {
	case "bash":
		return "bash", []string{"-lc", script}, nil
	case "sh":
		return "sh", []string{"-c", script}, nil
	case "zsh":
		return "zsh", []string{"-lc", script}, nil
	case "fish":
		return "fish", []string{"-c", script}, nil
	case "cmd":
		return "cmd", []string{"/C", script}, nil
	case "powershell":
		return "powershell", []string{"-Command", script}, nil
	case "pwsh":
		return "pwsh", []string{"-Command", script}, nil
	case "python", "python3":
		return shell, []string{"-c", script}, nil
	case "node":
		return "node", []string{"-e", script}, nil
	case "deno":
		return "deno", []string{"eval", script}, nil
	case "bun":
		return "bun", []string{"-e", script}, nil
	default:
		return "", nil, fmt.Errorf("unsupported shell %q; supported shells: bash, sh, zsh, fish, cmd, powershell, pwsh, python, python3, node, deno, bun", j.Shell)
	}
}

func normalizeShellName(shell string) string {
	return strings.ToLower(strings.TrimSpace(shell))
}

// Config is the top-level structure of a Vortex config file.
type Config struct {
	Name string    `yaml:"name"`
	Jobs []JobSpec `yaml:"jobs"`
}

// Load reads and parses a Vortex config file stored in YAML syntax.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if strings.TrimSpace(cfg.Name) == "" {
		return nil, fmt.Errorf("config is missing a required top-level name field")
	}
	if len(cfg.Jobs) == 0 {
		return nil, fmt.Errorf("config defines no jobs")
	}
	// Validate: all IDs non-empty and unique.
	seen := make(map[string]struct{}, len(cfg.Jobs))
	for i := range cfg.Jobs {
		j := &cfg.Jobs[i]
		if j.ID == "" {
			return nil, fmt.Errorf("a job is missing an id field")
		}
		if _, dup := seen[j.ID]; dup {
			return nil, fmt.Errorf("duplicate job id %q", j.ID)
		}
		seen[j.ID] = struct{}{}
		if _, _, err := j.CommandLine(); err != nil {
			return nil, fmt.Errorf("job %q: %w", j.ID, err)
		}
		// Validate labels fall back to ID if not set.
		if j.Label == "" {
			j.Label = j.ID
		}
	}
	// Validate: needs references must exist.
	for _, j := range cfg.Jobs {
		for _, need := range j.Needs {
			if _, ok := seen[need]; !ok {
				return nil, fmt.Errorf("job %q needs unknown job %q", j.ID, need)
			}
		}
		if j.If != "" && j.If != "success" && j.If != "failure" && j.If != "always" {
			return nil, fmt.Errorf("job %q: if must be \"success\", \"failure\", or \"always\", got %q", j.ID, j.If)
		}
	}
	return &cfg, nil
}
