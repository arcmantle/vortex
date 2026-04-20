package config

import (
	"reflect"
	"strings"
)

// BunRuntimeSpec defines a shared Bun runtime with imports, vars, and functions
// that are injected into all jobs that opt in with use: bun.
type BunRuntimeSpec struct {
	Typescript bool              `yaml:"typescript"`
	Sources    []string          `yaml:"sources"`
	Imports    []JSImportSpec    `yaml:"imports"`
	Vars       map[string]any    `yaml:"vars"`
	Functions  map[string]string `yaml:"functions"`
}

func (b BunRuntimeSpec) Empty() bool {
	return !b.Typescript && len(b.Sources) == 0 && len(b.Imports) == 0 && len(b.Vars) == 0 && len(b.Functions) == 0
}

func (b BunRuntimeSpec) Equal(other BunRuntimeSpec) bool {
	return reflect.DeepEqual(b, other)
}

func (j JobSpec) UsesBunRuntime() bool {
	return strings.TrimSpace(j.Use) == "bun"
}

func (cfg Config) validateBunRuntime() error {
	return validateJSRuntime(cfg.Bun.Sources, cfg.Bun.Imports, cfg.Bun.Vars, cfg.Bun.Functions, "bun", cfg.WorkingDir)
}

func (cfg Config) prepareBunJobCommand(job JobSpec) (string, []string, error) {
	ts := jsRuntimeUsesTypeScript(cfg.Bun.Sources, cfg.Bun.Typescript)
	return prepareJSRuntimeJobCommand(cfg, job, cfg.Bun.Sources, cfg.Bun.Imports, cfg.Bun.Vars, cfg.Bun.Functions, "bun", "bun-runtime", ts)
}
