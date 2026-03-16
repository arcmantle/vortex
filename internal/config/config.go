// Package config handles loading and validating vortex YAML configuration files.
//
// Example vortex.yaml:
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

	"gopkg.in/yaml.v3"
)

// JobSpec describes a single job in the config.
type JobSpec struct {
	// ID is the unique job identifier (used in needs references and SSE URLs).
	ID string `yaml:"id"`
	// Label is the human-readable display name shown in the UI.
	Label string `yaml:"label"`
	// Command is the full shell command including args, space-separated.
	// Example: "go test -race ./..."
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

// Config is the top-level structure of a vortex.yaml file.
type Config struct {
	Jobs []JobSpec `yaml:"jobs"`
}

// Load reads and parses a YAML config file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if len(cfg.Jobs) == 0 {
		return nil, fmt.Errorf("config defines no jobs")
	}
	// Validate: all IDs non-empty and unique.
	seen := make(map[string]struct{}, len(cfg.Jobs))
	for _, j := range cfg.Jobs {
		if j.ID == "" {
			return nil, fmt.Errorf("a job is missing an id field")
		}
		if _, dup := seen[j.ID]; dup {
			return nil, fmt.Errorf("duplicate job id %q", j.ID)
		}
		seen[j.ID] = struct{}{}
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
