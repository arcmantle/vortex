package config

import (
	"reflect"
	"strings"
)

// NodeRuntimeSpec defines a shared Node.js runtime with imports, vars, and
// functions that are injected into all jobs that opt in with use: node.
type NodeRuntimeSpec struct {
	Typescript bool              `yaml:"typescript"`
	Sources    []string          `yaml:"sources"`
	Imports    []JSImportSpec    `yaml:"imports"`
	Vars       map[string]any    `yaml:"vars"`
	Functions  map[string]string `yaml:"functions"`
}

func (n NodeRuntimeSpec) Empty() bool {
	return !n.Typescript && len(n.Sources) == 0 && len(n.Imports) == 0 && len(n.Vars) == 0 && len(n.Functions) == 0
}

func (n NodeRuntimeSpec) Equal(other NodeRuntimeSpec) bool {
	return reflect.DeepEqual(n, other)
}

func (j JobSpec) UsesNodeRuntime() bool {
	return strings.TrimSpace(j.Use) == "node"
}

func (cfg Config) validateNodeRuntime() error {
	return validateJSRuntime(cfg.Node.Sources, cfg.Node.Imports, cfg.Node.Vars, cfg.Node.Functions, "node", cfg.WorkingDir)
}

func (cfg Config) prepareNodeJobCommand(job JobSpec) (string, []string, error) {
	ts := jsRuntimeUsesTypeScript(cfg.Node.Sources, cfg.Node.Typescript)
	return prepareJSRuntimeJobCommand(cfg, job, cfg.Node.Sources, cfg.Node.Imports, cfg.Node.Vars, cfg.Node.Functions, "node", "node-runtime", ts)
}
