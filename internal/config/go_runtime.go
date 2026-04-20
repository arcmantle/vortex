package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

var goIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// GoImportSpec describes a Go module dependency.
type GoImportSpec struct {
	// Path is the module path (e.g. "github.com/fatih/color").
	Path string `yaml:"path"`
	// Version is the module version (e.g. "v1.16.0").
	Version string `yaml:"version"`
}

// GoRuntimeSpec defines a shared Go runtime with imports, vars, and functions
// that are injected into all jobs that opt in with use: go.
type GoRuntimeSpec struct {
	Module    string            `yaml:"module"`
	Sources   []string          `yaml:"sources"`
	Imports   []GoImportSpec    `yaml:"imports"`
	Vars      map[string]any    `yaml:"vars"`
	Functions map[string]string `yaml:"functions"`
}

func (g GoRuntimeSpec) Empty() bool {
	return len(g.Sources) == 0 && len(g.Imports) == 0 && len(g.Vars) == 0 && len(g.Functions) == 0
}

func (g GoRuntimeSpec) Equal(other GoRuntimeSpec) bool {
	return reflect.DeepEqual(g, other)
}

func (g GoRuntimeSpec) moduleName() string {
	if strings.TrimSpace(g.Module) != "" {
		return strings.TrimSpace(g.Module)
	}
	return "vortex/runtime"
}

func (j JobSpec) UsesGoRuntime() bool {
	return strings.TrimSpace(j.Use) == "go"
}

func (cfg Config) validateGoRuntime() error {
	if cfg.Go.Empty() {
		return nil
	}
	for i, src := range cfg.Go.Sources {
		if strings.TrimSpace(src) == "" {
			return fmt.Errorf("go.sources[%d]: path must not be empty", i)
		}
	}
	for i, imp := range cfg.Go.Imports {
		if strings.TrimSpace(imp.Path) == "" {
			return fmt.Errorf("go.imports[%d]: path is required", i)
		}
		if strings.TrimSpace(imp.Version) == "" {
			return fmt.Errorf("go.imports[%d] (%s): version is required", i, imp.Path)
		}
	}
	seen := make(map[string]struct{})
	for key := range cfg.Go.Vars {
		if !isValidGoIdentifier(key) {
			return fmt.Errorf("go.vars: %q is not a valid Go identifier", key)
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("go.vars: duplicate key %q", key)
		}
		seen[key] = struct{}{}
	}
	for key := range cfg.Go.Functions {
		if !isValidGoIdentifier(key) {
			return fmt.Errorf("go.functions: %q is not a valid Go identifier", key)
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("go.functions: %q conflicts with a var of the same name", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func isValidGoIdentifier(value string) bool {
	return goIdentifierPattern.MatchString(strings.TrimSpace(value))
}

// prepareGoJobCommand generates a Go project in the cache directory and
// returns "go" with args ["run", "."].
func (cfg Config) prepareGoJobCommand(job JobSpec) (string, []string, error) {
	runtimeDir, err := goRuntimeDir(cfg)
	if err != nil {
		return "", nil, err
	}
	projectDir := filepath.Join(runtimeDir, sanitizeFileComponent(job.ID)+"-"+hashString(job.ID)[:8])
	if err := os.MkdirAll(projectDir, 0o700); err != nil {
		return "", nil, fmt.Errorf("create go project directory: %w", err)
	}

	// Generate go.mod
	goMod := goModFile(cfg.Go)
	if err := writeFileIfChanged(filepath.Join(projectDir, "go.mod"), []byte(goMod), 0o600); err != nil {
		return "", nil, fmt.Errorf("write go.mod: %w", err)
	}

	// Generate shared.go (only when using shared runtime with vars/functions)
	if job.UsesGoRuntime() && !cfg.Go.Empty() && (len(cfg.Go.Vars) > 0 || len(cfg.Go.Functions) > 0) {
		shared := goSharedSource(cfg.Go)
		if err := writeFileIfChanged(filepath.Join(projectDir, "shared.go"), []byte(shared), 0o600); err != nil {
			return "", nil, fmt.Errorf("write shared.go: %w", err)
		}
	} else {
		os.Remove(filepath.Join(projectDir, "shared.go"))
	}

	// Generate main.go
	program := goProgramSource(cfg.Go, job)
	if err := writeFileIfChanged(filepath.Join(projectDir, "main.go"), []byte(program), 0o600); err != nil {
		return "", nil, fmt.Errorf("write main.go: %w", err)
	}

	// Copy source files into the project directory.
	if err := copyGoSources(cfg.Go.Sources, projectDir, cfg.WorkingDir); err != nil {
		return "", nil, err
	}

	return "go", []string{"run", "."}, nil
}

func goRuntimeDir(cfg Config) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	workDir := cfg.WorkingDir
	if strings.TrimSpace(workDir) == "" {
		workDir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve working directory for go runtime: %w", err)
		}
	}
	key := hashString(cfg.Name + "|" + workDir)
	return filepath.Join(base, "vortex", "go-runtime", key), nil
}

func goModFile(spec GoRuntimeSpec) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("module %s\n\ngo 1.21\n", spec.moduleName()))
	if len(spec.Imports) > 0 {
		b.WriteString("\nrequire (\n")
		for _, imp := range spec.Imports {
			b.WriteString(fmt.Sprintf("\t%s %s\n", strings.TrimSpace(imp.Path), strings.TrimSpace(imp.Version)))
		}
		b.WriteString(")\n")
	}
	return b.String()
}

func goSharedSource(spec GoRuntimeSpec) string {
	var b strings.Builder
	b.WriteString("// Auto-generated by Vortex — do not edit.\n")
	b.WriteString("package main\n")

	// Detect standard library imports needed by function bodies.
	imports := detectGoImports(spec)
	if len(imports) > 0 {
		b.WriteString("\nimport (\n")
		for _, imp := range imports {
			fmt.Fprintf(&b, "\t%q\n", imp)
		}
		b.WriteString(")\n")
	}

	b.WriteByte('\n')

	// Vars as top-level variables.
	varKeys := make([]string, 0, len(spec.Vars))
	for key := range spec.Vars {
		varKeys = append(varKeys, key)
	}
	sort.Strings(varKeys)
	if len(varKeys) > 0 {
		b.WriteString("var (\n")
		for _, key := range varKeys {
			typeName, literal := goLiteral(spec.Vars[key])
			_ = typeName
			fmt.Fprintf(&b, "\t%s = %s\n", key, literal)
		}
		b.WriteString(")\n")
	}
	if len(varKeys) > 0 && len(spec.Functions) > 0 {
		b.WriteByte('\n')
	}

	// Functions as top-level functions.
	funcKeys := make([]string, 0, len(spec.Functions))
	for key := range spec.Functions {
		funcKeys = append(funcKeys, key)
	}
	sort.Strings(funcKeys)
	for i, key := range funcKeys {
		b.WriteString(strings.TrimSpace(spec.Functions[key]))
		b.WriteByte('\n')
		if i < len(funcKeys)-1 {
			b.WriteByte('\n')
		}
	}

	return b.String()
}

// detectGoImports scans function bodies for references to standard library
// packages and returns those that should be imported in shared.go.
func detectGoImports(spec GoRuntimeSpec) []string {
	// Combine all function bodies for scanning.
	var combined strings.Builder
	for _, body := range spec.Functions {
		combined.WriteString(body)
		combined.WriteByte('\n')
	}
	return detectGoImportsFromText(combined.String())
}

// detectGoImportsFromText scans text for references to common standard library
// packages and returns matching import paths.
func detectGoImportsFromText(text string) []string {
	// Common standard library packages detected by prefix usage.
	candidates := []struct {
		prefix string
		pkg    string
	}{
		{"fmt.", "fmt"},
		{"os.", "os"},
		{"strings.", "strings"},
		{"strconv.", "strconv"},
		{"filepath.", "path/filepath"},
		{"path.", "path"},
		{"time.", "time"},
		{"io.", "io"},
		{"log.", "log"},
		{"math.", "math"},
		{"http.", "net/http"},
		{"json.", "encoding/json"},
		{"bytes.", "bytes"},
		{"bufio.", "bufio"},
		{"sort.", "sort"},
		{"sync.", "sync"},
		{"context.", "context"},
		{"errors.", "errors"},
		{"regexp.", "regexp"},
	}

	seen := make(map[string]struct{})
	var result []string
	for _, c := range candidates {
		if strings.Contains(text, c.prefix) {
			if _, exists := seen[c.pkg]; !exists {
				seen[c.pkg] = struct{}{}
				result = append(result, c.pkg)
			}
		}
	}
	sort.Strings(result)
	return result
}

func goProgramSource(spec GoRuntimeSpec, job JobSpec) string {
	var b strings.Builder
	b.WriteString("// Auto-generated by Vortex — do not edit.\n")
	b.WriteString("package main\n")

	cmd := strings.TrimSpace(job.Command)

	// If the command already contains "func main()", use it as-is (the user
	// manages their own imports and main function).
	if strings.Contains(cmd, "func main()") {
		b.WriteByte('\n')
		b.WriteString(cmd)
		b.WriteByte('\n')
		return b.String()
	}

	// Detect imports needed by the command text.
	imports := detectGoImportsFromText(cmd)
	if len(imports) > 0 {
		b.WriteString("\nimport (\n")
		for _, imp := range imports {
			fmt.Fprintf(&b, "\t%q\n", imp)
		}
		b.WriteString(")\n")
	}

	// Wrap the command in func main().
	b.WriteString("\nfunc main() {\n")
	lines := strings.Split(cmd, "\n")
	for _, line := range lines {
		b.WriteString("\t" + line + "\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func goLiteral(value any) (typeName, literal string) {
	switch v := value.(type) {
	case string:
		data, _ := json.Marshal(v)
		return "string", string(data)
	case bool:
		if v {
			return "bool", "true"
		}
		return "bool", "false"
	case int:
		return "int", fmt.Sprintf("%d", v)
	case int64:
		return "int64", fmt.Sprintf("int64(%d)", v)
	case float64:
		if v == float64(int64(v)) {
			return "int", fmt.Sprintf("%d", int64(v))
		}
		return "float64", fmt.Sprintf("%g", v)
	default:
		data, _ := json.Marshal(v)
		return "string", string(data)
	}
}

// copyGoSources copies the listed source files into the project directory.
func copyGoSources(sources []string, projectDir, workDir string) error {
	for i, src := range sources {
		src = strings.TrimSpace(src)
		absPath := src
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(workDir, absPath)
		}
		data, err := os.ReadFile(absPath)
		if err != nil {
			return fmt.Errorf("go.sources[%d]: %w", i, err)
		}
		destName := filepath.Base(absPath)
		if err := writeFileIfChanged(filepath.Join(projectDir, destName), data, 0o600); err != nil {
			return fmt.Errorf("go.sources[%d]: write %s: %w", i, destName, err)
		}
	}
	return nil
}
