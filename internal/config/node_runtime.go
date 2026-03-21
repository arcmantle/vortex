package config

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var jsIdentifierPattern = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

type NodeRuntimeSpec struct {
	Imports   []NodeImportSpec  `yaml:"imports"`
	Vars      map[string]any    `yaml:"vars"`
	Functions map[string]string `yaml:"functions"`
}

type NodeImportSpec struct {
	From      string                `yaml:"from"`
	Default   string                `yaml:"default"`
	Namespace string                `yaml:"namespace"`
	Names     []string              `yaml:"names"`
	Named     []NodeNamedImportSpec `yaml:"named"`
}

type NodeNamedImportSpec struct {
	Export string `yaml:"export"`
	As     string `yaml:"as"`
}

func (n NodeRuntimeSpec) Empty() bool {
	return len(n.Imports) == 0 && len(n.Vars) == 0 && len(n.Functions) == 0
}

func (n NodeRuntimeSpec) Equal(other NodeRuntimeSpec) bool {
	left, err := json.Marshal(n)
	if err != nil {
		return false
	}
	right, err := json.Marshal(other)
	if err != nil {
		return false
	}
	return string(left) == string(right)
}

func (j JobSpec) UsesNodeRuntime() bool {
	return strings.TrimSpace(j.Use) == "node"
}

func (cfg Config) PrepareJobCommand(job JobSpec) (string, []string, error) {
	if strings.TrimSpace(job.Use) == "" {
		return job.CommandLine()
	}
	if strings.TrimSpace(job.Use) != "node" {
		return "", nil, fmt.Errorf("unsupported runtime %q", job.Use)
	}
	if normalizeShellName(job.Shell) != "node" {
		return "", nil, fmt.Errorf("use: node requires shell: node")
	}
	if cfg.Node.Empty() {
		return "", nil, fmt.Errorf("use: node requires a top-level node block")
	}
	return cfg.prepareNodeJobCommand(job)
}

func (cfg Config) validateJobSpec(job JobSpec) error {
	use := strings.TrimSpace(job.Use)
	if use == "" {
		return nil
	}
	if use != "node" {
		return fmt.Errorf("use must be \"node\", got %q", job.Use)
	}
	if normalizeShellName(job.Shell) != "node" {
		return fmt.Errorf("use: node requires shell: node")
	}
	if cfg.Node.Empty() {
		return fmt.Errorf("use: node requires a top-level node block")
	}
	return nil
}

func (cfg Config) validateNodeRuntime() error {
	if cfg.Node.Empty() {
		return nil
	}

	exported := make(map[string]string)
	register := func(name, source string) error {
		if !isValidJSIdentifier(name) {
			return fmt.Errorf("node runtime name %q is not a valid JavaScript identifier", name)
		}
		if prev, exists := exported[name]; exists {
			return fmt.Errorf("node runtime name %q from %s conflicts with %s", name, source, prev)
		}
		exported[name] = source
		return nil
	}

	for index, spec := range cfg.Node.Imports {
		if err := validateNodeImportSpec(spec); err != nil {
			return fmt.Errorf("node.imports[%d]: %w", index, err)
		}
		for _, name := range spec.boundNames() {
			if err := register(name, fmt.Sprintf("node.imports[%d]", index)); err != nil {
				return err
			}
		}
	}

	varKeys := make([]string, 0, len(cfg.Node.Vars))
	for key := range cfg.Node.Vars {
		varKeys = append(varKeys, key)
	}
	sort.Strings(varKeys)
	for _, key := range varKeys {
		if _, err := json.Marshal(cfg.Node.Vars[key]); err != nil {
			return fmt.Errorf("node.vars.%s: value is not JSON-serializable: %w", key, err)
		}
		if err := register(key, "node.vars"); err != nil {
			return err
		}
	}

	functionKeys := make([]string, 0, len(cfg.Node.Functions))
	for key := range cfg.Node.Functions {
		functionKeys = append(functionKeys, key)
	}
	sort.Strings(functionKeys)
	for _, key := range functionKeys {
		body := strings.TrimSpace(cfg.Node.Functions[key])
		if body == "" {
			return fmt.Errorf("node.functions.%s must not be empty", key)
		}
		pattern := regexp.MustCompile(`(?m)export\s+(async\s+)?function\s+` + regexp.QuoteMeta(key) + `\b`)
		if !pattern.MatchString(body) {
			return fmt.Errorf("node.functions.%s must export function %q", key, key)
		}
		if err := register(key, "node.functions"); err != nil {
			return err
		}
	}

	return nil
}

func validateNodeImportSpec(spec NodeImportSpec) error {
	if strings.TrimSpace(spec.From) == "" {
		return fmt.Errorf("from is required")
	}
	modes := 0
	if strings.TrimSpace(spec.Default) != "" {
		modes++
		if !isValidJSIdentifier(spec.Default) {
			return fmt.Errorf("default alias %q is not a valid JavaScript identifier", spec.Default)
		}
	}
	if strings.TrimSpace(spec.Namespace) != "" {
		modes++
		if !isValidJSIdentifier(spec.Namespace) {
			return fmt.Errorf("namespace alias %q is not a valid JavaScript identifier", spec.Namespace)
		}
	}
	if len(spec.Names) > 0 {
		modes++
		seen := make(map[string]struct{}, len(spec.Names))
		for _, name := range spec.Names {
			trimmed := strings.TrimSpace(name)
			if !isValidJSIdentifier(trimmed) {
				return fmt.Errorf("named import %q is not a valid JavaScript identifier", name)
			}
			if _, exists := seen[trimmed]; exists {
				return fmt.Errorf("duplicate named import %q", trimmed)
			}
			seen[trimmed] = struct{}{}
		}
	}
	if len(spec.Named) > 0 {
		modes++
		seen := make(map[string]struct{}, len(spec.Named))
		for _, named := range spec.Named {
			if strings.TrimSpace(named.Export) == "" {
				return fmt.Errorf("named import export is required")
			}
			alias := named.As
			if strings.TrimSpace(alias) == "" {
				alias = named.Export
			}
			if !isValidJSIdentifier(alias) {
				return fmt.Errorf("named import alias %q is not a valid JavaScript identifier", alias)
			}
			if _, exists := seen[alias]; exists {
				return fmt.Errorf("duplicate named import alias %q", alias)
			}
			seen[alias] = struct{}{}
		}
	}
	if modes != 1 {
		return fmt.Errorf("must define exactly one of default, namespace, names, or named")
	}
	return nil
}

func (spec NodeImportSpec) boundNames() []string {
	if strings.TrimSpace(spec.Default) != "" {
		return []string{strings.TrimSpace(spec.Default)}
	}
	if strings.TrimSpace(spec.Namespace) != "" {
		return []string{strings.TrimSpace(spec.Namespace)}
	}
	if len(spec.Names) > 0 {
		out := make([]string, 0, len(spec.Names))
		for _, name := range spec.Names {
			out = append(out, strings.TrimSpace(name))
		}
		return out
	}
	out := make([]string, 0, len(spec.Named))
	for _, named := range spec.Named {
		alias := strings.TrimSpace(named.As)
		if alias == "" {
			alias = strings.TrimSpace(named.Export)
		}
		out = append(out, alias)
	}
	return out
}

func (cfg Config) prepareNodeJobCommand(job JobSpec) (string, []string, error) {
	runtimeDir, err := cfg.nodeRuntimeDir()
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return "", nil, fmt.Errorf("create node runtime directory: %w", err)
	}

	sharedPath := filepath.Join(runtimeDir, "shared.mjs")
	sharedSource, err := cfg.nodeSharedModuleSource()
	if err != nil {
		return "", nil, err
	}
	if err := os.WriteFile(sharedPath, []byte(sharedSource), 0o644); err != nil {
		return "", nil, fmt.Errorf("write shared node runtime: %w", err)
	}

	wrapperPath := filepath.Join(runtimeDir, sanitizeFileComponent(job.ID)+".mjs")
	wrapperSource, err := cfg.nodeJobWrapperSource(job, sharedPath)
	if err != nil {
		return "", nil, err
	}
	if err := os.WriteFile(wrapperPath, []byte(wrapperSource), 0o644); err != nil {
		return "", nil, fmt.Errorf("write node job wrapper: %w", err)
	}

	return "node", []string{wrapperPath}, nil
}

func (cfg Config) nodeRuntimeDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	workDir := cfg.WorkingDir
	if strings.TrimSpace(workDir) == "" {
		workDir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve working directory for node runtime: %w", err)
		}
	}
	key := hashString(cfg.Name + "|" + workDir)
	return filepath.Join(base, "vortex", "node-runtime", key), nil
}

func (cfg Config) nodeSharedModuleSource() (string, error) {
	var lines []string
	workDir := cfg.WorkingDir
	if strings.TrimSpace(workDir) == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve working directory for node runtime: %w", err)
		}
	}

	for _, spec := range cfg.Node.Imports {
		specifier, err := resolveNodeModuleSpecifier(spec.From, workDir)
		if err != nil {
			return "", err
		}
		lines = append(lines, renderNodeImportLine(spec, specifier))
	}
	importBindings := cfg.nodeImportedNames()
	if len(importBindings) > 0 {
		lines = append(lines, fmt.Sprintf("export { %s };", strings.Join(importBindings, ", ")))
	}
	if len(lines) > 0 {
		lines = append(lines, "")
	}

	varKeys := make([]string, 0, len(cfg.Node.Vars))
	for key := range cfg.Node.Vars {
		varKeys = append(varKeys, key)
	}
	sort.Strings(varKeys)
	for _, key := range varKeys {
		data, err := json.Marshal(cfg.Node.Vars[key])
		if err != nil {
			return "", fmt.Errorf("marshal node var %q: %w", key, err)
		}
		lines = append(lines, fmt.Sprintf("export const %s = %s;", key, string(data)))
	}
	if len(varKeys) > 0 && len(cfg.Node.Functions) > 0 {
		lines = append(lines, "")
	}

	functionKeys := make([]string, 0, len(cfg.Node.Functions))
	for key := range cfg.Node.Functions {
		functionKeys = append(functionKeys, key)
	}
	sort.Strings(functionKeys)
	for index, key := range functionKeys {
		lines = append(lines, strings.TrimSpace(cfg.Node.Functions[key]))
		if index != len(functionKeys)-1 {
			lines = append(lines, "")
		}
	}

	return strings.Join(lines, "\n") + "\n", nil
}

func (cfg Config) nodeJobWrapperSource(job JobSpec, sharedPath string) (string, error) {
	var lines []string
	bindings := cfg.nodeExportedNames()
	if len(bindings) > 0 {
		sharedSpecifier, err := resolveNodeModuleSpecifier(sharedPath, "")
		if err != nil {
			return "", err
		}
		lines = append(lines, fmt.Sprintf("import { %s } from %q;", strings.Join(bindings, ", "), sharedSpecifier))
		lines = append(lines, "")
	}
	lines = append(lines, strings.TrimSpace(job.Command))
	return strings.Join(lines, "\n") + "\n", nil
}

func (cfg Config) nodeExportedNames() []string {
	var names []string
	names = append(names, cfg.nodeImportedNames()...)
	for key := range cfg.Node.Vars {
		names = append(names, key)
	}
	for key := range cfg.Node.Functions {
		names = append(names, key)
	}
	sort.Strings(names)
	return names
}

func (cfg Config) nodeImportedNames() []string {
	var names []string
	for _, spec := range cfg.Node.Imports {
		names = append(names, spec.boundNames()...)
	}
	sort.Strings(names)
	return names
}

func renderNodeImportLine(spec NodeImportSpec, specifier string) string {
	if strings.TrimSpace(spec.Default) != "" {
		return fmt.Sprintf("import %s from %q;", strings.TrimSpace(spec.Default), specifier)
	}
	if strings.TrimSpace(spec.Namespace) != "" {
		return fmt.Sprintf("import * as %s from %q;", strings.TrimSpace(spec.Namespace), specifier)
	}
	if len(spec.Names) > 0 {
		return fmt.Sprintf("import { %s } from %q;", strings.Join(trimmedValues(spec.Names), ", "), specifier)
	}
	parts := make([]string, 0, len(spec.Named))
	for _, named := range spec.Named {
		exported := strings.TrimSpace(named.Export)
		alias := strings.TrimSpace(named.As)
		if alias == "" || alias == exported {
			parts = append(parts, exported)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s as %s", exported, alias))
	}
	return fmt.Sprintf("import { %s } from %q;", strings.Join(parts, ", "), specifier)
}

func resolveNodeModuleSpecifier(specifier, workDir string) (string, error) {
	specifier = strings.TrimSpace(specifier)
	if specifier == "" {
		return "", fmt.Errorf("node import specifier must not be empty")
	}
	if strings.HasPrefix(specifier, "node:") || strings.HasPrefix(specifier, "file://") {
		return specifier, nil
	}
	if isLocalModuleSpecifier(specifier) {
		absPath := specifier
		if !filepath.IsAbs(absPath) {
			base := workDir
			if strings.TrimSpace(base) == "" {
				var err error
				base, err = os.Getwd()
				if err != nil {
					return "", fmt.Errorf("resolve working directory for import %q: %w", specifier, err)
				}
			}
			absPath = filepath.Join(base, absPath)
		}
		absPath, err := filepath.Abs(absPath)
		if err != nil {
			return "", fmt.Errorf("resolve node import %q: %w", specifier, err)
		}
		return fileURL(absPath), nil
	}
	return specifier, nil
}

func isLocalModuleSpecifier(specifier string) bool {
	return strings.HasPrefix(specifier, "./") || strings.HasPrefix(specifier, "../") || filepath.IsAbs(specifier)
}

func fileURL(path string) string {
	slashPath := filepath.ToSlash(path)
	if volume := filepath.VolumeName(path); volume != "" {
		slashPath = "/" + slashPath
	}
	return (&url.URL{Scheme: "file", Path: slashPath}).String()
}

func sanitizeFileComponent(value string) string {
	if strings.TrimSpace(value) == "" {
		return "job"
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	return b.String()
}

func hashString(value string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(value))
	return fmt.Sprintf("%x", h.Sum64())
}

func isValidJSIdentifier(value string) bool {
	return jsIdentifierPattern.MatchString(strings.TrimSpace(value))
}

func trimmedValues(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strings.TrimSpace(value))
	}
	return out
}
