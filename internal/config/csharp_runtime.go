package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

var csharpIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// CSharpPackageSpec describes a NuGet package reference.
type CSharpPackageSpec struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

// CSharpRuntimeSpec defines a shared C# runtime with usings, packages, vars,
// and functions that are injected into all jobs that opt in with use: csharp.
type CSharpRuntimeSpec struct {
	Framework string              `yaml:"framework"`
	Sources   []string            `yaml:"sources"`
	Usings    []string            `yaml:"usings"`
	Packages  []CSharpPackageSpec `yaml:"packages"`
	Vars      map[string]any      `yaml:"vars"`
	Functions map[string]string   `yaml:"functions"`
}

func (c CSharpRuntimeSpec) Empty() bool {
	return len(c.Sources) == 0 && len(c.Usings) == 0 && len(c.Packages) == 0 && len(c.Vars) == 0 && len(c.Functions) == 0
}

func (c CSharpRuntimeSpec) Equal(other CSharpRuntimeSpec) bool {
	return reflect.DeepEqual(c, other)
}

func (c CSharpRuntimeSpec) framework() string {
	if strings.TrimSpace(c.Framework) != "" {
		return strings.TrimSpace(c.Framework)
	}
	return "net8.0"
}

func (j JobSpec) UsesCSharpRuntime() bool {
	return strings.TrimSpace(j.Use) == "csharp"
}

func (cfg Config) validateCSharpRuntime() error {
	if cfg.CSharp.Empty() {
		return nil
	}
	for i, src := range cfg.CSharp.Sources {
		if strings.TrimSpace(src) == "" {
			return fmt.Errorf("csharp.sources[%d]: path must not be empty", i)
		}
	}
	for _, using := range cfg.CSharp.Usings {
		if strings.TrimSpace(using) == "" {
			return fmt.Errorf("csharp.usings: entry must not be empty")
		}
	}
	for i, pkg := range cfg.CSharp.Packages {
		if strings.TrimSpace(pkg.Name) == "" {
			return fmt.Errorf("csharp.packages[%d]: name is required", i)
		}
		if strings.TrimSpace(pkg.Version) == "" {
			return fmt.Errorf("csharp.packages[%d] (%s): version is required", i, pkg.Name)
		}
	}
	seen := make(map[string]struct{})
	for key := range cfg.CSharp.Vars {
		if !isValidCSharpIdentifier(key) {
			return fmt.Errorf("csharp.vars: %q is not a valid C# identifier", key)
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("csharp.vars: duplicate key %q", key)
		}
		seen[key] = struct{}{}
	}
	for key := range cfg.CSharp.Functions {
		if !isValidCSharpIdentifier(key) {
			return fmt.Errorf("csharp.functions: %q is not a valid C# identifier", key)
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("csharp.functions: %q conflicts with a var of the same name", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func isValidCSharpIdentifier(value string) bool {
	return csharpIdentifierPattern.MatchString(strings.TrimSpace(value))
}

// prepareCSharpJobCommand generates a .NET project in the cache directory and
// returns "dotnet" with args ["run", "--project", "<dir>"].
func (cfg Config) prepareCSharpJobCommand(job JobSpec) (string, []string, error) {
	runtimeDir, err := csharpRuntimeDir(cfg)
	if err != nil {
		return "", nil, err
	}
	projectDir := filepath.Join(runtimeDir, sanitizeFileComponent(job.ID)+"-"+hashString(job.ID)[:8])
	if err := os.MkdirAll(projectDir, 0o700); err != nil {
		return "", nil, fmt.Errorf("create csharp project directory: %w", err)
	}

	// Generate .csproj
	csproj := csharpProjectFile(cfg.CSharp)
	if err := writeFileIfChanged(filepath.Join(projectDir, "project.csproj"), []byte(csproj), 0o600); err != nil {
		return "", nil, fmt.Errorf("write csharp project file: %w", err)
	}

	// Generate Shared.cs (only when using shared runtime)
	if job.UsesCSharpRuntime() && !cfg.CSharp.Empty() {
		shared := csharpSharedSource(cfg.CSharp)
		if err := writeFileIfChanged(filepath.Join(projectDir, "Shared.cs"), []byte(shared), 0o600); err != nil {
			return "", nil, fmt.Errorf("write csharp shared source: %w", err)
		}
	} else {
		// Remove Shared.cs if it exists from a previous run with use: csharp
		os.Remove(filepath.Join(projectDir, "Shared.cs"))
	}

	// Generate Program.cs
	program := csharpProgramSource(cfg.CSharp, job)
	if err := writeFileIfChanged(filepath.Join(projectDir, "Program.cs"), []byte(program), 0o600); err != nil {
		return "", nil, fmt.Errorf("write csharp program source: %w", err)
	}

	// Copy source files into the project directory.
	if err := copyCSharpSources(cfg.CSharp.Sources, projectDir, cfg.WorkingDir); err != nil {
		return "", nil, err
	}

	return "dotnet", []string{"run", "--project", projectDir}, nil
}

func csharpRuntimeDir(cfg Config) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	workDir := cfg.WorkingDir
	if strings.TrimSpace(workDir) == "" {
		workDir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve working directory for csharp runtime: %w", err)
		}
	}
	key := hashString(cfg.Name + "|" + workDir)
	return filepath.Join(base, "vortex", "csharp-runtime", key), nil
}

func csharpProjectFile(spec CSharpRuntimeSpec) string {
	var b strings.Builder
	b.WriteString(`<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <OutputType>Exe</OutputType>
    <TargetFramework>`)
	b.WriteString(spec.framework())
	b.WriteString(`</TargetFramework>
    <ImplicitUsings>enable</ImplicitUsings>
  </PropertyGroup>
`)
	if len(spec.Packages) > 0 {
		b.WriteString("  <ItemGroup>\n")
		for _, pkg := range spec.Packages {
			b.WriteString(fmt.Sprintf("    <PackageReference Include=%q Version=%q />\n",
				strings.TrimSpace(pkg.Name), strings.TrimSpace(pkg.Version)))
		}
		b.WriteString("  </ItemGroup>\n")
	}
	b.WriteString("</Project>\n")
	return b.String()
}

func csharpSharedSource(spec CSharpRuntimeSpec) string {
	var b strings.Builder
	b.WriteString("// Auto-generated by Vortex — do not edit.\n")

	for _, using := range spec.Usings {
		b.WriteString(fmt.Sprintf("using %s;\n", strings.TrimSpace(using)))
	}
	if len(spec.Usings) > 0 {
		b.WriteByte('\n')
	}

	b.WriteString("static class Vortex\n{\n")

	// Vars as public static readonly fields.
	varKeys := make([]string, 0, len(spec.Vars))
	for key := range spec.Vars {
		varKeys = append(varKeys, key)
	}
	sort.Strings(varKeys)
	for _, key := range varKeys {
		typeName, literal := csharpLiteral(spec.Vars[key])
		b.WriteString(fmt.Sprintf("    public static readonly %s %s = %s;\n", typeName, key, literal))
	}
	if len(varKeys) > 0 && len(spec.Functions) > 0 {
		b.WriteByte('\n')
	}

	// Functions as public static methods.
	funcKeys := make([]string, 0, len(spec.Functions))
	for key := range spec.Functions {
		funcKeys = append(funcKeys, key)
	}
	sort.Strings(funcKeys)
	for i, key := range funcKeys {
		body := strings.TrimSpace(spec.Functions[key])
		// Indent each line by 4 spaces.
		lines := strings.Split(body, "\n")
		for _, line := range lines {
			b.WriteString("    " + line + "\n")
		}
		if i < len(funcKeys)-1 {
			b.WriteByte('\n')
		}
	}

	b.WriteString("}\n")
	return b.String()
}

func csharpProgramSource(spec CSharpRuntimeSpec, job JobSpec) string {
	var b strings.Builder
	b.WriteString("// Auto-generated by Vortex — do not edit.\n")
	if job.UsesCSharpRuntime() && !spec.Empty() {
		b.WriteString("using static Vortex;\n\n")
	}
	b.WriteString(strings.TrimSpace(job.Command))
	b.WriteByte('\n')
	return b.String()
}

func csharpLiteral(value any) (typeName, literal string) {
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
		return "long", fmt.Sprintf("%dL", v)
	case float64:
		if v == float64(int64(v)) {
			return "int", fmt.Sprintf("%d", int64(v))
		}
		return "double", fmt.Sprintf("%g", v)
	default:
		data, _ := json.Marshal(v)
		return "string", string(data)
	}
}

// writeFileIfChanged only writes the file if its content differs from what's
// already on disk. This avoids triggering unnecessary dotnet rebuilds.
func writeFileIfChanged(path string, data []byte, perm os.FileMode) error {
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, data) {
		return nil
	}
	return atomicWriteFile(path, data, perm)
}

// copyCSharpSources copies the listed source files into the project directory.
// Files are written only when their content has changed.
func copyCSharpSources(sources []string, projectDir, workDir string) error {
	for i, src := range sources {
		src = strings.TrimSpace(src)
		absPath := src
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(workDir, absPath)
		}
		data, err := os.ReadFile(absPath)
		if err != nil {
			return fmt.Errorf("csharp.sources[%d]: %w", i, err)
		}
		destName := filepath.Base(absPath)
		if err := writeFileIfChanged(filepath.Join(projectDir, destName), data, 0o600); err != nil {
			return fmt.Errorf("csharp.sources[%d]: write %s: %w", i, destName, err)
		}
	}
	return nil
}
