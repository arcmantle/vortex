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

	"github.com/evanw/esbuild/pkg/api"
)

// --- Shared types for all JS runtimes (Node, Bun, Deno) ---

var jsIdentifierPattern = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

// JSImportSpec describes a single ESM import declaration.
type JSImportSpec struct {
	From      string              `yaml:"from"`
	Default   string              `yaml:"default"`
	Namespace string              `yaml:"namespace"`
	Names     []string            `yaml:"names"`
	Named     []JSNamedImportSpec `yaml:"named"`
}

// JSNamedImportSpec describes a named import with an optional alias.
type JSNamedImportSpec struct {
	Export string `yaml:"export"`
	As     string `yaml:"as"`
}

func (spec JSImportSpec) boundNames() []string {
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

// --- Shared validation ---

// validateJSRuntime validates a shared JS runtime spec (sources, imports, vars, functions).
// The prefix is used in error messages (e.g. "node", "bun", or "deno").
func validateJSRuntime(sources []string, imports []JSImportSpec, vars map[string]any, functions map[string]string, prefix, workDir string) error {
	if len(sources) == 0 && len(imports) == 0 && len(vars) == 0 && len(functions) == 0 {
		return nil
	}

	exported := make(map[string]string)
	register := func(name, source string) error {
		if !isValidJSIdentifier(name) {
			return fmt.Errorf("%s runtime name %q is not a valid JavaScript identifier", prefix, name)
		}
		if prev, exists := exported[name]; exists {
			return fmt.Errorf("%s runtime name %q from %s conflicts with %s", prefix, name, source, prev)
		}
		exported[name] = source
		return nil
	}

	for index, src := range sources {
		src = strings.TrimSpace(src)
		if src == "" {
			return fmt.Errorf("%s.sources[%d]: path must not be empty", prefix, index)
		}
		ns := fileNameToNamespace(src)
		if ns == "" {
			return fmt.Errorf("%s.sources[%d]: cannot derive namespace from %q", prefix, index, src)
		}
		if err := register(ns, fmt.Sprintf("%s.sources[%d]", prefix, index)); err != nil {
			return err
		}
	}

	for index, spec := range imports {
		if err := validateJSImportSpec(spec); err != nil {
			return fmt.Errorf("%s.imports[%d]: %w", prefix, index, err)
		}
		for _, name := range spec.boundNames() {
			if err := register(name, fmt.Sprintf("%s.imports[%d]", prefix, index)); err != nil {
				return err
			}
		}
	}

	varKeys := make([]string, 0, len(vars))
	for key := range vars {
		varKeys = append(varKeys, key)
	}
	sort.Strings(varKeys)
	for _, key := range varKeys {
		if _, err := json.Marshal(vars[key]); err != nil {
			return fmt.Errorf("%s.vars.%s: value is not JSON-serializable: %w", prefix, key, err)
		}
		if err := register(key, prefix+".vars"); err != nil {
			return err
		}
	}

	functionKeys := make([]string, 0, len(functions))
	for key := range functions {
		functionKeys = append(functionKeys, key)
	}
	sort.Strings(functionKeys)
	for _, key := range functionKeys {
		body := strings.TrimSpace(functions[key])
		if body == "" {
			return fmt.Errorf("%s.functions.%s must not be empty", prefix, key)
		}
		pattern, compileErr := regexp.Compile(`(?m)export\s+(async\s+)?function\s+` + regexp.QuoteMeta(key) + `\b`)
		if compileErr != nil {
			return fmt.Errorf("internal: compile export pattern for %q: %w", key, compileErr)
		}
		if !pattern.MatchString(body) {
			return fmt.Errorf("%s.functions.%s must export function %q", prefix, key, key)
		}
		if err := register(key, prefix+".functions"); err != nil {
			return err
		}
	}

	return nil
}

func validateJSImportSpec(spec JSImportSpec) error {
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

// --- Shared code generation ---

// prepareJSRuntimeJobCommand generates shared+wrapper files and returns the binary + args.
func prepareJSRuntimeJobCommand(cfg Config, job JobSpec, sources []string, imports []JSImportSpec, vars map[string]any, functions map[string]string, binary, cacheDirName string, typescript bool) (string, []string, error) {
	return prepareJSRuntimeJobCommandWithArgs(cfg, job, sources, imports, vars, functions, binary, cacheDirName, nil, typescript)
}

// prepareJSRuntimeJobCommandWithArgs is like prepareJSRuntimeJobCommand but
// allows extra args to be inserted before the wrapper path (e.g. "run --allow-all" for deno).
func prepareJSRuntimeJobCommandWithArgs(cfg Config, job JobSpec, sources []string, imports []JSImportSpec, vars map[string]any, functions map[string]string, binary, cacheDirName string, extraArgs []string, typescript bool) (string, []string, error) {
	runtimeDir, err := jsRuntimeDir(cfg, cacheDirName)
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		return "", nil, fmt.Errorf("create %s runtime directory: %w", binary, err)
	}

	ext := ".mjs"
	if typescript {
		ext = ".mts"
	}

	sharedPath := filepath.Join(runtimeDir, "shared"+ext)
	sharedSource, err := jsSharedModuleSource(sources, imports, vars, functions, cfg.WorkingDir)
	if err != nil {
		return "", nil, err
	}
	if err := atomicWriteFile(sharedPath, []byte(sharedSource), 0o600); err != nil {
		return "", nil, fmt.Errorf("write shared %s runtime: %w", binary, err)
	}

	wrapperName := sanitizeFileComponent(job.ID) + "-" + hashString(job.ID)[:8]
	wrapperPath := filepath.Join(runtimeDir, wrapperName+ext)
	wrapperSource, err := jsJobWrapperSource(sources, imports, vars, functions, job, sharedPath, cfg.WorkingDir)
	if err != nil {
		return "", nil, err
	}
	if err := atomicWriteFile(wrapperPath, []byte(wrapperSource), 0o600); err != nil {
		return "", nil, fmt.Errorf("write %s job wrapper: %w", binary, err)
	}

	runPath := wrapperPath
	// Node requires esbuild bundling for TypeScript; Bun and Deno run .mts natively.
	if typescript && binary == "node" {
		bundledPath := filepath.Join(runtimeDir, wrapperName+".mjs")
		if err := esbuildBundleForNode(wrapperPath, bundledPath); err != nil {
			return "", nil, err
		}
		runPath = bundledPath
	}

	args := append(append([]string{}, extraArgs...), runPath)
	return binary, args, nil
}

func jsRuntimeDir(cfg Config, cacheDirName string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	workDir := cfg.WorkingDir
	if strings.TrimSpace(workDir) == "" {
		workDir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve working directory for runtime: %w", err)
		}
	}
	key := hashString(cfg.Name + "|" + workDir)
	return filepath.Join(base, "vortex", cacheDirName, key), nil
}

func jsSharedModuleSource(sources []string, imports []JSImportSpec, vars map[string]any, functions map[string]string, workDir string) (string, error) {
	var lines []string
	if strings.TrimSpace(workDir) == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve working directory for runtime: %w", err)
		}
	}

	// Sources become namespace imports + re-exports.
	for _, src := range sources {
		specifier, err := resolveJSModuleSpecifier(strings.TrimSpace(src), workDir)
		if err != nil {
			return "", err
		}
		ns := fileNameToNamespace(src)
		lines = append(lines, fmt.Sprintf("import * as %s from %q;", ns, specifier))
	}
	if len(sources) > 0 {
		namespaces := make([]string, 0, len(sources))
		for _, src := range sources {
			namespaces = append(namespaces, fileNameToNamespace(src))
		}
		lines = append(lines, fmt.Sprintf("export { %s };", strings.Join(namespaces, ", ")))
		lines = append(lines, "")
	}

	for _, spec := range imports {
		specifier, err := resolveJSModuleSpecifier(spec.From, workDir)
		if err != nil {
			return "", err
		}
		lines = append(lines, renderJSImportLine(spec, specifier))
	}
	importBindings := jsImportedNames(imports)
	if len(importBindings) > 0 {
		lines = append(lines, fmt.Sprintf("export { %s };", strings.Join(importBindings, ", ")))
	}
	if len(lines) > 0 {
		lines = append(lines, "")
	}

	varKeys := make([]string, 0, len(vars))
	for key := range vars {
		varKeys = append(varKeys, key)
	}
	sort.Strings(varKeys)
	for _, key := range varKeys {
		data, err := json.Marshal(vars[key])
		if err != nil {
			return "", fmt.Errorf("marshal var %q: %w", key, err)
		}
		lines = append(lines, fmt.Sprintf("export const %s = %s;", key, string(data)))
	}
	if len(varKeys) > 0 && len(functions) > 0 {
		lines = append(lines, "")
	}

	functionKeys := make([]string, 0, len(functions))
	for key := range functions {
		functionKeys = append(functionKeys, key)
	}
	sort.Strings(functionKeys)
	for index, key := range functionKeys {
		lines = append(lines, strings.TrimSpace(functions[key]))
		if index != len(functionKeys)-1 {
			lines = append(lines, "")
		}
	}

	return strings.Join(lines, "\n") + "\n", nil
}

func jsJobWrapperSource(sources []string, imports []JSImportSpec, vars map[string]any, functions map[string]string, job JobSpec, sharedPath, workDir string) (string, error) {
	var lines []string
	bindings := jsExportedNames(sources, imports, vars, functions)
	if len(bindings) > 0 {
		sharedSpecifier, err := resolveJSModuleSpecifier(sharedPath, "")
		if err != nil {
			return "", err
		}
		lines = append(lines, fmt.Sprintf("import { %s } from %q;", strings.Join(bindings, ", "), sharedSpecifier))
		lines = append(lines, "")
	}
	lines = append(lines, strings.TrimSpace(job.Command))
	return strings.Join(lines, "\n") + "\n", nil
}

func jsExportedNames(sources []string, imports []JSImportSpec, vars map[string]any, functions map[string]string) []string {
	var names []string
	for _, src := range sources {
		names = append(names, fileNameToNamespace(src))
	}
	names = append(names, jsImportedNames(imports)...)
	for key := range vars {
		names = append(names, key)
	}
	for key := range functions {
		names = append(names, key)
	}
	sort.Strings(names)
	return names
}

func jsImportedNames(imports []JSImportSpec) []string {
	var names []string
	for _, spec := range imports {
		names = append(names, spec.boundNames()...)
	}
	sort.Strings(names)
	return names
}

// --- Shared import resolution ---

func renderJSImportLine(spec JSImportSpec, specifier string) string {
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

func resolveJSModuleSpecifier(specifier, workDir string) (string, error) {
	specifier = strings.TrimSpace(specifier)
	if specifier == "" {
		return "", fmt.Errorf("import specifier must not be empty")
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
			return "", fmt.Errorf("resolve import %q: %w", specifier, err)
		}
		return fileURL(absPath), nil
	}
	return specifier, nil
}

func isLocalModuleSpecifier(specifier string) bool {
	cleaned := filepath.ToSlash(specifier)
	return strings.HasPrefix(cleaned, "./") || strings.HasPrefix(cleaned, "../") || filepath.IsAbs(specifier)
}

// --- Shared utilities ---

func fileURL(path string) string {
	slashPath := filepath.ToSlash(path)
	volume := filepath.VolumeName(path)
	if volume == "" {
		return (&url.URL{Scheme: "file", Path: slashPath}).String()
	}
	if len(volume) > 2 {
		return "file:" + slashPath
	}
	return (&url.URL{Scheme: "file", Path: "/" + slashPath}).String()
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
		if r == '/' || r == '\\' {
			b.WriteString("--")
			continue
		}
		b.WriteByte('_')
	}
	return b.String()
}

func hashString(value string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(value))
	return fmt.Sprintf("%016x", h.Sum64())
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".vortex-tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if err := f.Chmod(perm); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// jsRuntimeUsesTypeScript returns true if TypeScript mode should be enabled.
// TypeScript is enabled explicitly via the typescript field, or implicitly when
// any source file has a .ts, .mts, or .cts extension.
func jsRuntimeUsesTypeScript(sources []string, typescript bool) bool {
	if typescript {
		return true
	}
	for _, src := range sources {
		ext := strings.ToLower(filepath.Ext(strings.TrimSpace(src)))
		if ext == ".ts" || ext == ".mts" || ext == ".cts" {
			return true
		}
	}
	return false
}

// esbuildBundleForNode uses esbuild's Go API to bundle a TypeScript entry point
// into a single ESM .mjs file that Node can run. Local file:// imports and
// relative imports are resolved and inlined; bare specifiers (npm packages) and
// node: builtins are kept external.
func esbuildBundleForNode(entryPoint, outFile string) error {
	result := api.Build(api.BuildOptions{
		EntryPoints: []string{entryPoint},
		Outfile:     outFile,
		Bundle:      true,
		Format:      api.FormatESModule,
		Platform:    api.PlatformNode,
		Target:      api.ESNext,
		Write:       true,
		LogLevel:    api.LogLevelSilent,
		Plugins: []api.Plugin{{
			Name: "externalize-packages",
			Setup: func(build api.PluginBuild) {
				build.OnResolve(api.OnResolveOptions{Filter: ".*"}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
					path := args.Path
					if args.Kind != api.ResolveJSImportStatement && args.Kind != api.ResolveJSDynamicImport {
						return api.OnResolveResult{}, nil
					}
					// node: builtins are external.
					if strings.HasPrefix(path, "node:") {
						return api.OnResolveResult{Path: path, External: true}, nil
					}
					// Convert file:// URLs to local paths for esbuild resolution.
					if strings.HasPrefix(path, "file://") {
						parsed, err := url.Parse(path)
						if err != nil {
							return api.OnResolveResult{}, nil
						}
						return api.OnResolveResult{Path: parsed.Path, Namespace: "file"}, nil
					}
					// Relative and absolute paths are resolved by esbuild.
					if strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../") || filepath.IsAbs(path) {
						return api.OnResolveResult{}, nil
					}
					// Bare specifier — keep external.
					return api.OnResolveResult{Path: path, External: true}, nil
				})
			},
		}},
	})
	if len(result.Errors) > 0 {
		msg := result.Errors[0].Text
		if result.Errors[0].Location != nil {
			msg = fmt.Sprintf("%s:%d:%d: %s", result.Errors[0].Location.File, result.Errors[0].Location.Line, result.Errors[0].Location.Column, msg)
		}
		return fmt.Errorf("esbuild: %s", msg)
	}
	return nil
}

func isValidJSIdentifier(value string) bool {
	return jsIdentifierPattern.MatchString(strings.TrimSpace(value))
}

// fileNameToNamespace derives a camelCase namespace identifier from a file path.
// Example: "./run-helper.mjs" → "runHelper", "../utils/api.js" → "api".
func fileNameToNamespace(path string) string {
	base := filepath.Base(strings.TrimSpace(path))
	// Strip all extensions (e.g. ".d.ts", ".mjs", ".js").
	for ext := filepath.Ext(base); ext != ""; ext = filepath.Ext(base) {
		base = strings.TrimSuffix(base, ext)
	}
	if base == "" {
		return ""
	}
	// Split on - _ . and camelCase the parts.
	parts := strings.FieldsFunc(base, func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	})
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	for i, part := range parts {
		if part == "" {
			continue
		}
		if i == 0 {
			// First part: lowercase initial.
			b.WriteByte(toLower(part[0]))
			b.WriteString(part[1:])
		} else {
			// Subsequent parts: uppercase initial.
			b.WriteByte(toUpper(part[0]))
			b.WriteString(part[1:])
		}
	}
	result := b.String()
	if !isValidJSIdentifier(result) {
		return ""
	}
	return result
}

func toLower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 32
	}
	return c
}

func toUpper(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - 32
	}
	return c
}

func trimmedValues(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strings.TrimSpace(value))
	}
	return out
}

// --- Shared job spec routing ---

// PrepareJobCommand resolves the executable and args for a job, applying shared
// runtime wrapper generation when use is set.
func (cfg Config) PrepareJobCommand(job JobSpec) (string, []string, error) {
	use := strings.TrimSpace(job.Use)
	shell := normalizeShellName(job.Shell)

	// C# always requires project generation, even without use: csharp.
	if shell == "csharp" || use == "csharp" {
		if use == "csharp" && shell != "csharp" {
			return "", nil, fmt.Errorf("use: csharp requires shell: csharp")
		}
		return cfg.prepareCSharpJobCommand(job)
	}

	// Go always requires project generation, even without use: go.
	if shell == "go" || use == "go" {
		if use == "go" && shell != "go" {
			return "", nil, fmt.Errorf("use: go requires shell: go")
		}
		return cfg.prepareGoJobCommand(job)
	}

	if use == "" {
		return job.CommandLine()
	}
	switch use {
	case "node":
		if normalizeShellName(job.Shell) != "node" {
			return "", nil, fmt.Errorf("use: node requires shell: node")
		}
		if cfg.Node.Empty() {
			return "", nil, fmt.Errorf("use: node requires a top-level node block")
		}
		return cfg.prepareNodeJobCommand(job)
	case "bun":
		if normalizeShellName(job.Shell) != "bun" {
			return "", nil, fmt.Errorf("use: bun requires shell: bun")
		}
		if cfg.Bun.Empty() {
			return "", nil, fmt.Errorf("use: bun requires a top-level bun block")
		}
		return cfg.prepareBunJobCommand(job)
	case "deno":
		if normalizeShellName(job.Shell) != "deno" {
			return "", nil, fmt.Errorf("use: deno requires shell: deno")
		}
		if cfg.Deno.Empty() {
			return "", nil, fmt.Errorf("use: deno requires a top-level deno block")
		}
		return cfg.prepareDenoJobCommand(job)
	default:
		return "", nil, fmt.Errorf("unsupported runtime %q; supported runtimes: node, bun, deno, csharp, go", use)
	}
}

func (cfg Config) validateJobSpec(job JobSpec) error {
	use := strings.TrimSpace(job.Use)
	shell := normalizeShellName(job.Shell)

	// C# shell always requires project generation — validate shell/use consistency.
	if shell == "csharp" || use == "csharp" {
		if use == "csharp" && shell != "csharp" {
			return fmt.Errorf("use: csharp requires shell: csharp")
		}
		if use == "csharp" && cfg.CSharp.Empty() {
			return fmt.Errorf("use: csharp requires a top-level csharp block")
		}
		return nil
	}

	// Go shell always requires project generation — validate shell/use consistency.
	if shell == "go" || use == "go" {
		if use == "go" && shell != "go" {
			return fmt.Errorf("use: go requires shell: go")
		}
		if use == "go" && cfg.Go.Empty() {
			return fmt.Errorf("use: go requires a top-level go block")
		}
		return nil
	}

	if use == "" {
		return nil
	}
	switch use {
	case "node":
		if normalizeShellName(job.Shell) != "node" {
			return fmt.Errorf("use: node requires shell: node")
		}
		if cfg.Node.Empty() {
			return fmt.Errorf("use: node requires a top-level node block")
		}
	case "bun":
		if normalizeShellName(job.Shell) != "bun" {
			return fmt.Errorf("use: bun requires shell: bun")
		}
		if cfg.Bun.Empty() {
			return fmt.Errorf("use: bun requires a top-level bun block")
		}
	case "deno":
		if normalizeShellName(job.Shell) != "deno" {
			return fmt.Errorf("use: deno requires shell: deno")
		}
		if cfg.Deno.Empty() {
			return fmt.Errorf("use: deno requires a top-level deno block")
		}
	default:
		return fmt.Errorf("use must be \"node\", \"bun\", \"deno\", \"csharp\", or \"go\", got %q", job.Use)
	}
	return nil
}
