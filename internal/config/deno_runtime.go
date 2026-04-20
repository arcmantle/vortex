package config

import (
	"reflect"
	"strings"
)

// DenoRuntimeSpec defines a shared Deno runtime with imports, vars, and functions
// that are injected into all jobs that opt in with use: deno.
type DenoRuntimeSpec struct {
	Typescript bool              `yaml:"typescript"`
	Sources    []string          `yaml:"sources"`
	Imports    []JSImportSpec    `yaml:"imports"`
	Vars       map[string]any    `yaml:"vars"`
	Functions  map[string]string `yaml:"functions"`
}

func (d DenoRuntimeSpec) Empty() bool {
	return !d.Typescript && len(d.Sources) == 0 && len(d.Imports) == 0 && len(d.Vars) == 0 && len(d.Functions) == 0
}

func (d DenoRuntimeSpec) Equal(other DenoRuntimeSpec) bool {
	return reflect.DeepEqual(d, other)
}

func (j JobSpec) UsesDenoRuntime() bool {
	return strings.TrimSpace(j.Use) == "deno"
}

func (cfg Config) validateDenoRuntime() error {
	return validateJSRuntime(cfg.Deno.Sources, cfg.Deno.Imports, cfg.Deno.Vars, cfg.Deno.Functions, "deno", cfg.WorkingDir)
}

func (cfg Config) prepareDenoJobCommand(job JobSpec) (string, []string, error) {
	ts := jsRuntimeUsesTypeScript(cfg.Deno.Sources, cfg.Deno.Typescript)
	return prepareJSRuntimeJobCommandWithArgs(cfg, job, cfg.Deno.Sources, cfg.Deno.Imports, cfg.Deno.Vars, cfg.Deno.Functions, "deno", "deno-runtime", []string{"run", "--allow-all"}, ts)
}
