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
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

var supportedOSSelectors = map[string]struct{}{
	"darwin":  {},
	"linux":   {},
	"windows": {},
	"default": {},
}

// JobSpec describes a single job in the config.
type JobSpec struct {
	// ID is the unique job identifier (used in needs references and SSE URLs).
	ID string `yaml:"id"`
	// Label is the human-readable display name shown in the UI.
	Label string `yaml:"label"`
	// Use opts the job into a shared runtime declared at the top-level config.
	// Supports "node" (requires shell: node), "bun" (requires shell: bun),
	// "deno" (requires shell: deno), "csharp" (requires shell: csharp),
	// and "go" (requires shell: go).
	Use string `yaml:"use"`
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

type rawJobSpec struct {
	ID      string   `yaml:"id"`
	Label   string   `yaml:"label"`
	Use     string   `yaml:"use"`
	Group   string   `yaml:"group"`
	Needs   []string `yaml:"needs"`
	If      string   `yaml:"if"`
	Restart *bool    `yaml:"restart"`
}

// UnmarshalYAML accepts either a plain string or an OS-keyed object for the
// shell and command fields, then resolves the value for the current runtime OS.
func (j *JobSpec) UnmarshalYAML(node *yaml.Node) error {
	var raw rawJobSpec
	if err := node.Decode(&raw); err != nil {
		return err
	}

	command, err := resolveOSValue(mappingValueNode(node, "command"), "command")
	if err != nil {
		return err
	}
	shell, err := resolveOSValue(mappingValueNode(node, "shell"), "shell")
	if err != nil {
		return err
	}

	*j = JobSpec{
		ID:      raw.ID,
		Label:   raw.Label,
		Use:     raw.Use,
		Shell:   shell,
		Command: command,
		Group:   raw.Group,
		Needs:   raw.Needs,
		If:      raw.If,
		Restart: raw.Restart,
	}
	return nil
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
	case "csharp":
		// C# requires project generation; actual command is resolved by PrepareJobCommand.
		return "dotnet", []string{"run"}, nil
	case "go":
		// Go requires project generation; actual command is resolved by PrepareJobCommand.
		return "go", []string{"run", "."}, nil
	default:
		return "", nil, fmt.Errorf("unsupported shell %q; supported shells: bash, sh, zsh, fish, cmd, powershell, pwsh, python, python3, node, deno, bun, csharp, go", j.Shell)
	}
}

func normalizeShellName(shell string) string {
	return strings.ToLower(strings.TrimSpace(shell))
}

func mappingValueNode(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		if node.Content[index].Value == key {
			return node.Content[index+1]
		}
	}
	return nil
}

func resolveOSValue(node *yaml.Node, field string) (string, error) {
	if node == nil {
		return "", nil
	}

	switch node.Kind {
	case yaml.ScalarNode:
		var value string
		if err := node.Decode(&value); err != nil {
			return "", fmt.Errorf("%s must be a string", field)
		}
		return value, nil
	case yaml.MappingNode:
		var values map[string]string
		if err := node.Decode(&values); err != nil {
			return "", fmt.Errorf("%s OS selector must be an object of strings: %w", field, err)
		}
		if len(values) == 0 {
			return "", fmt.Errorf("%s OS selector must not be empty", field)
		}
		normalized := make(map[string]string, len(values))
		for key := range values {
			normalizedKey := strings.ToLower(strings.TrimSpace(key))
			if _, ok := supportedOSSelectors[normalizedKey]; !ok {
				return "", fmt.Errorf("%s uses unsupported OS key %q; supported keys: darwin, linux, windows, default", field, key)
			}
			if _, dup := normalized[normalizedKey]; dup {
				return "", fmt.Errorf("%s has duplicate OS key %q (after case normalization)", field, normalizedKey)
			}
			normalized[normalizedKey] = values[key]
		}
		if value, ok := normalized[runtime.GOOS]; ok {
			return value, nil
		}
		if value, ok := normalized["default"]; ok {
			return value, nil
		}
		return "", fmt.Errorf("%s does not define a value for %q and has no default", field, runtime.GOOS)
	default:
		return "", fmt.Errorf("%s must be either a string or an OS selector object", field)
	}
}

// Config is the top-level structure of a Vortex config file.
type Config struct {
	Name   string            `yaml:"name"`
	Node   NodeRuntimeSpec   `yaml:"node"`
	Bun    BunRuntimeSpec    `yaml:"bun"`
	Deno   DenoRuntimeSpec   `yaml:"deno"`
	CSharp CSharpRuntimeSpec `yaml:"csharp"`
	Go     GoRuntimeSpec     `yaml:"go"`
	Jobs   []JobSpec         `yaml:"jobs"`
	// WorkingDir is the runtime working directory used for job execution.
	// It is derived from CLI flags and the config path, not from YAML.
	WorkingDir string `yaml:"-"`
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
	if err := cfg.validateNodeRuntime(); err != nil {
		return nil, err
	}
	if err := cfg.validateBunRuntime(); err != nil {
		return nil, err
	}
	if err := cfg.validateDenoRuntime(); err != nil {
		return nil, err
	}
	if err := cfg.validateCSharpRuntime(); err != nil {
		return nil, err
	}
	if err := cfg.validateGoRuntime(); err != nil {
		return nil, err
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
		if err := cfg.validateJobSpec(*j); err != nil {
			return nil, fmt.Errorf("job %q: %w", j.ID, err)
		}
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
	// Validate: no dependency cycles.
	if cycle := detectCycle(cfg.Jobs); cycle != nil {
		return nil, fmt.Errorf("dependency cycle detected: %s", strings.Join(cycle, " -> "))
	}
	return &cfg, nil
}

// detectCycle performs a DFS-based topological sort on the job dependency
// graph and returns a cycle path if one exists, or nil if the graph is a DAG.
func detectCycle(jobs []JobSpec) []string {
	needs := make(map[string][]string, len(jobs))
	for _, j := range jobs {
		needs[j.ID] = j.Needs
	}

	const (
		white = 0 // not visited
		gray  = 1 // in current DFS path
		black = 2 // fully explored
	)

	color := make(map[string]int, len(jobs))
	parent := make(map[string]string, len(jobs))

	var dfs func(id string) []string
	dfs = func(id string) []string {
		color[id] = gray
		for _, dep := range needs[id] {
			switch color[dep] {
			case gray:
				// Back edge — reconstruct cycle.
				cycle := []string{dep, id}
				cur := id
				for cur != dep && cur != "" && len(cycle) <= len(jobs) {
					cur = parent[cur]
					if cur == "" {
						break
					}
					cycle = append(cycle, cur)
				}
				// Reverse to get natural order.
				for i, j := 0, len(cycle)-1; i < j; i, j = i+1, j-1 {
					cycle[i], cycle[j] = cycle[j], cycle[i]
				}
				return cycle
			case white:
				parent[dep] = id
				if cycle := dfs(dep); cycle != nil {
					return cycle
				}
			}
		}
		color[id] = black
		return nil
	}

	for _, j := range jobs {
		if color[j.ID] == white {
			if cycle := dfs(j.ID); cycle != nil {
				return cycle
			}
		}
	}
	return nil
}
