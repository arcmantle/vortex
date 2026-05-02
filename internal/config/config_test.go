package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestJobSpecCommandLineWithShell(t *testing.T) {
	tests := []struct {
		name     string
		job      JobSpec
		wantCmd  string
		wantArgs []string
		wantErr  string
	}{
		{
			name:     "direct command",
			job:      JobSpec{Command: "go test ./..."},
			wantCmd:  "go",
			wantArgs: []string{"test", "./..."},
		},
		{
			name:     "node shell",
			job:      JobSpec{Shell: "node", Command: "console.log('ok')"},
			wantCmd:  "node",
			wantArgs: []string{"-e", "console.log('ok')"},
		},
		{
			name:    "unsupported shell",
			job:     JobSpec{Shell: "ruby", Command: "puts 'hi'"},
			wantErr: "unsupported shell",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotCmd, gotArgs, err := tc.job.CommandLine()
			if tc.wantErr != "" {
				if err == nil || !contains(err.Error(), tc.wantErr) {
					t.Fatalf("CommandLine() error = %v, want substring %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("CommandLine() error = %v", err)
			}
			if gotCmd != tc.wantCmd {
				t.Fatalf("CommandLine() cmd = %q, want %q", gotCmd, tc.wantCmd)
			}
			if len(gotArgs) != len(tc.wantArgs) {
				t.Fatalf("CommandLine() args len = %d, want %d (%v)", len(gotArgs), len(tc.wantArgs), gotArgs)
			}
			for i := range gotArgs {
				if gotArgs[i] != tc.wantArgs[i] {
					t.Fatalf("CommandLine() args[%d] = %q, want %q", i, gotArgs[i], tc.wantArgs[i])
				}
			}
		})
	}
}

func TestLoadRejectsGroupNameWithAtPrefix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.vortex")
	data := []byte("name: dev\njobs:\n  - id: smoke\n    command: echo ok\n    group: \"@settings\"\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(path)
	if err == nil || !contains(err.Error(), "must not start with '@'") {
		t.Fatalf("Load() error = %v, want group name rejection", err)
	}
}

func TestLoadRejectsUnsupportedShell(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.vortex")
	data := []byte("name: dev\njobs:\n  - id: smoke\n    shell: ruby\n    command: puts 'hi'\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(path)
	if err == nil || !contains(err.Error(), "unsupported shell") {
		t.Fatalf("Load() error = %v, want unsupported shell", err)
	}
}

func TestLoadResolvesOSSpecificCommandAndShell(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.vortex")
	data := []byte("name: dev\njobs:\n  - id: smoke\n    shell:\n      default: bash\n      windows: pwsh\n    command:\n      default: echo hello\n      windows: Write-Host hello\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Jobs) != 1 {
		t.Fatalf("Load() jobs len = %d, want 1", len(cfg.Jobs))
	}

	job := cfg.Jobs[0]
	if runtime.GOOS == "windows" {
		if job.Shell != "pwsh" || job.Command != "Write-Host hello" {
			t.Fatalf("windows resolution = shell %q command %q", job.Shell, job.Command)
		}
		return
	}
	if job.Shell != "bash" || job.Command != "echo hello" {
		t.Fatalf("default resolution = shell %q command %q", job.Shell, job.Command)
	}
}

func TestLoadRejectsUnsupportedOSSelectorKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.vortex")
	data := []byte("name: dev\njobs:\n  - id: smoke\n    command:\n      plan9: echo hello\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(path)
	if err == nil || !contains(err.Error(), "unsupported OS key") {
		t.Fatalf("Load() error = %v, want unsupported OS key", err)
	}
}

func TestLoadRejectsMissingCurrentOSAndDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.vortex")
	selector := "darwin: echo hello\n      windows: Write-Host hello"
	if runtime.GOOS == "darwin" {
		selector = "linux: echo hello\n      windows: Write-Host hello"
	} else if runtime.GOOS == "windows" {
		selector = "darwin: echo hello\n      linux: echo hello"
	}
	data := []byte("name: dev\njobs:\n  - id: smoke\n    command:\n      " + selector + "\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(path)
	if err == nil || !contains(err.Error(), "has no default") {
		t.Fatalf("Load() error = %v, want missing current OS/default", err)
	}
}

func TestLoadAcceptsNodeRuntimeAndUseNode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.vortex")
	data := []byte(`name: dev
node:
  imports:
    - from: node:path
      names: [basename]
  vars:
    apiBase: http://localhost:3000
  functions:
    logBanner: |
      export function logBanner(text) {
        console.log(text)
      }
jobs:
  - id: smoke
    shell: node
    use: node
    command: |
      logBanner(apiBase)
      console.log(basename('/tmp/demo.txt'))
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := cfg.Jobs[0].Use; got != "node" {
		t.Fatalf("Load() job use = %q, want %q", got, "node")
	}
	if cfg.Node.Empty() {
		t.Fatalf("Load() node runtime should not be empty")
	}
}

func TestLoadRejectsUseNodeWithoutNodeShell(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.vortex")
	data := []byte(`name: dev
node:
  vars:
    apiBase: http://localhost:3000
jobs:
  - id: smoke
    shell: bash
    use: node
    command: echo hi
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(path)
	if err == nil || !contains(err.Error(), "use: node requires shell: node") {
		t.Fatalf("Load() error = %v, want node shell validation", err)
	}
}

func TestLoadRejectsNodeRuntimeNameConflicts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.vortex")
	data := []byte(`name: dev
node:
  imports:
    - from: kleur
      default: helper
  functions:
    helper: |
      export function helper() {
        return 'ok'
      }
jobs:
  - id: smoke
    shell: node
    use: node
    command: helper()
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(path)
	if err == nil || !contains(err.Error(), "conflicts") {
		t.Fatalf("Load() error = %v, want name conflict", err)
	}
}

func TestPrepareJobCommandForSharedNodeRuntime(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	dir := t.TempDir()
	helperPath := filepath.Join(dir, "helpers.mjs")
	helperSource := `export function slug(value) {
  return String(value).toLowerCase().replace(/\s+/g, '-')
}

export async function describeFile(filePath) {
  return { path: filePath }
}
`
	if err := os.WriteFile(helperPath, []byte(helperSource), 0o600); err != nil {
		t.Fatalf("WriteFile(helper) error = %v", err)
	}

	cfg := Config{
		Name:       "dev",
		WorkingDir: dir,
		Node: NodeRuntimeSpec{
			Imports: []JSImportSpec{
				{From: "node:path", Names: []string{"basename"}},
				{From: "./helpers.mjs", Namespace: "helpers"},
			},
			Vars: map[string]any{
				"apiBase": "http://localhost:3000",
			},
			Functions: map[string]string{
				"logBanner": `export function logBanner(text) {
  console.log("banner:" + text)
}`,
			},
		},
	}
	if err := cfg.validateNodeRuntime(); err != nil {
		t.Fatalf("validateNodeRuntime() error = %v", err)
	}

	job := JobSpec{
		ID:      "smoke",
		Shell:   "node",
		Use:     "node",
		Command: "logBanner(apiBase)\nconsole.log(basename('/tmp/demo.txt'))\nconsole.log(helpers.slug('Hello World'))",
	}

	command, args, err := cfg.PrepareJobCommand(job)
	if err != nil {
		t.Fatalf("PrepareJobCommand() error = %v", err)
	}
	if command != "node" {
		t.Fatalf("PrepareJobCommand() command = %q, want node", command)
	}
	if len(args) != 1 {
		t.Fatalf("PrepareJobCommand() args len = %d, want 1", len(args))
	}

	wrapperData, err := os.ReadFile(args[0])
	if err != nil {
		t.Fatalf("ReadFile(wrapper) error = %v", err)
	}
	wrapperText := string(wrapperData)
	if !strings.Contains(wrapperText, `from "file://`) {
		t.Fatalf("wrapper missing shared import:\n%s", wrapperText)
	}
	if !strings.Contains(wrapperText, "helpers") || !strings.Contains(wrapperText, "logBanner") {
		t.Fatalf("wrapper missing shared bindings:\n%s", wrapperText)
	}

	output, err := exec.Command(command, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("node wrapper execution error = %v\n%s", err, string(output))
	}
	text := string(output)
	if !strings.Contains(text, "banner:http://localhost:3000") {
		t.Fatalf("node output missing banner:\n%s", text)
	}
	if !strings.Contains(text, "demo.txt") {
		t.Fatalf("node output missing basename:\n%s", text)
	}
	if !strings.Contains(text, "hello-world") {
		t.Fatalf("node output missing helper output:\n%s", text)
	}
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

func TestLoadAcceptsBunRuntimeAndUseBun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.vortex")
	data := []byte(`name: dev
bun:
  imports:
    - from: node:path
      names: [basename]
  vars:
    apiBase: http://localhost:3000
  functions:
    logBanner: |
      export function logBanner(text) {
        console.log(text)
      }
jobs:
  - id: smoke
    shell: bun
    use: bun
    command: |
      logBanner(apiBase)
      console.log(basename('/tmp/demo.txt'))
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := cfg.Jobs[0].Use; got != "bun" {
		t.Fatalf("Load() job use = %q, want %q", got, "bun")
	}
	if cfg.Bun.Empty() {
		t.Fatalf("Load() bun runtime should not be empty")
	}
}

func TestLoadRejectsUseBunWithoutBunShell(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.vortex")
	data := []byte(`name: dev
bun:
  vars:
    apiBase: http://localhost:3000
jobs:
  - id: smoke
    shell: bash
    use: bun
    command: echo hi
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(path)
	if err == nil || !contains(err.Error(), "use: bun requires shell: bun") {
		t.Fatalf("Load() error = %v, want bun shell validation", err)
	}
}

func TestLoadRejectsBunRuntimeNameConflicts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.vortex")
	data := []byte(`name: dev
bun:
  imports:
    - from: kleur
      default: helper
  functions:
    helper: |
      export function helper() {
        return 'ok'
      }
jobs:
  - id: smoke
    shell: bun
    use: bun
    command: helper()
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(path)
	if err == nil || !contains(err.Error(), "conflicts") {
		t.Fatalf("Load() error = %v, want name conflict", err)
	}
}

func TestPrepareJobCommandForSharedBunRuntime(t *testing.T) {
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun is not installed")
	}

	dir := t.TempDir()
	helperPath := filepath.Join(dir, "helpers.mjs")
	helperSource := `export function slug(value) {
  return String(value).toLowerCase().replace(/\s+/g, '-')
}
`
	if err := os.WriteFile(helperPath, []byte(helperSource), 0o600); err != nil {
		t.Fatalf("WriteFile(helper) error = %v", err)
	}

	cfg := Config{
		Name:       "dev",
		WorkingDir: dir,
		Bun: BunRuntimeSpec{
			Imports: []JSImportSpec{
				{From: "node:path", Names: []string{"basename"}},
				{From: "./helpers.mjs", Namespace: "helpers"},
			},
			Vars: map[string]any{
				"apiBase": "http://localhost:3000",
			},
			Functions: map[string]string{
				"logBanner": `export function logBanner(text) {
  console.log("banner:" + text)
}`,
			},
		},
	}
	if err := cfg.validateBunRuntime(); err != nil {
		t.Fatalf("validateBunRuntime() error = %v", err)
	}

	job := JobSpec{
		ID:      "smoke",
		Shell:   "bun",
		Use:     "bun",
		Command: "logBanner(apiBase)\nconsole.log(basename('/tmp/demo.txt'))\nconsole.log(helpers.slug('Hello World'))",
	}

	command, args, err := cfg.PrepareJobCommand(job)
	if err != nil {
		t.Fatalf("PrepareJobCommand() error = %v", err)
	}
	if command != "bun" {
		t.Fatalf("PrepareJobCommand() command = %q, want bun", command)
	}
	if len(args) != 1 {
		t.Fatalf("PrepareJobCommand() args len = %d, want 1", len(args))
	}

	wrapperData, err := os.ReadFile(args[0])
	if err != nil {
		t.Fatalf("ReadFile(wrapper) error = %v", err)
	}
	wrapperText := string(wrapperData)
	if !strings.Contains(wrapperText, `from "file://`) {
		t.Fatalf("wrapper missing shared import:\n%s", wrapperText)
	}
	if !strings.Contains(wrapperText, "helpers") || !strings.Contains(wrapperText, "logBanner") {
		t.Fatalf("wrapper missing shared bindings:\n%s", wrapperText)
	}

	output, err := exec.Command(command, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("bun wrapper execution error = %v\n%s", err, string(output))
	}
	text := string(output)
	if !strings.Contains(text, "banner:http://localhost:3000") {
		t.Fatalf("bun output missing banner:\n%s", text)
	}
	if !strings.Contains(text, "demo.txt") {
		t.Fatalf("bun output missing basename:\n%s", text)
	}
	if !strings.Contains(text, "hello-world") {
		t.Fatalf("bun output missing helper output:\n%s", text)
	}
}

func TestLoadAcceptsDenoRuntimeAndUseDeno(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.vortex")
	data := []byte(`name: dev
deno:
  imports:
    - from: https://deno.land/std/path/mod.ts
      names: [basename]
  vars:
    apiBase: http://localhost:3000
  functions:
    logBanner: |
      export function logBanner(text) {
        console.log(text)
      }
jobs:
  - id: smoke
    shell: deno
    use: deno
    command: |
      logBanner(apiBase)
      console.log(basename('/tmp/demo.txt'))
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := cfg.Jobs[0].Use; got != "deno" {
		t.Fatalf("Load() job use = %q, want %q", got, "deno")
	}
	if cfg.Deno.Empty() {
		t.Fatalf("Load() deno runtime should not be empty")
	}
}

func TestLoadRejectsUseDenoWithoutDenoShell(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.vortex")
	data := []byte(`name: dev
deno:
  vars:
    apiBase: http://localhost:3000
jobs:
  - id: smoke
    shell: bash
    use: deno
    command: echo hi
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(path)
	if err == nil || !contains(err.Error(), "use: deno requires shell: deno") {
		t.Fatalf("Load() error = %v, want deno shell validation", err)
	}
}

func TestLoadRejectsDenoRuntimeNameConflicts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.vortex")
	data := []byte(`name: dev
deno:
  imports:
    - from: https://deno.land/std/path/mod.ts
      default: helper
  functions:
    helper: |
      export function helper() {
        return 'ok'
      }
jobs:
  - id: smoke
    shell: deno
    use: deno
    command: helper()
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(path)
	if err == nil || !contains(err.Error(), "conflicts") {
		t.Fatalf("Load() error = %v, want name conflict", err)
	}
}

func TestPrepareJobCommandForSharedDenoRuntime(t *testing.T) {
	if _, err := exec.LookPath("deno"); err != nil {
		t.Skip("deno is not installed")
	}

	dir := t.TempDir()
	helperPath := filepath.Join(dir, "helpers.mjs")
	helperSource := `export function slug(value) {
  return String(value).toLowerCase().replace(/\s+/g, '-')
}
`
	if err := os.WriteFile(helperPath, []byte(helperSource), 0o600); err != nil {
		t.Fatalf("WriteFile(helper) error = %v", err)
	}

	cfg := Config{
		Name:       "dev",
		WorkingDir: dir,
		Deno: DenoRuntimeSpec{
			Imports: []JSImportSpec{
				{From: "https://deno.land/std@0.224.0/path/basename.ts", Names: []string{"basename"}},
				{From: "./helpers.mjs", Namespace: "helpers"},
			},
			Vars: map[string]any{
				"apiBase": "http://localhost:3000",
			},
			Functions: map[string]string{
				"logBanner": `export function logBanner(text) {
  console.log("banner:" + text)
}`,
			},
		},
	}
	if err := cfg.validateDenoRuntime(); err != nil {
		t.Fatalf("validateDenoRuntime() error = %v", err)
	}

	job := JobSpec{
		ID:      "smoke",
		Shell:   "deno",
		Use:     "deno",
		Command: "logBanner(apiBase)\nconsole.log(basename('/tmp/demo.txt'))\nconsole.log(helpers.slug('Hello World'))",
	}

	command, args, err := cfg.PrepareJobCommand(job)
	if err != nil {
		t.Fatalf("PrepareJobCommand() error = %v", err)
	}
	if command != "deno" {
		t.Fatalf("PrepareJobCommand() command = %q, want deno", command)
	}
	if len(args) < 2 || args[0] != "run" || args[1] != "--allow-all" {
		t.Fatalf("PrepareJobCommand() args = %v, want [run --allow-all <path>]", args)
	}

	wrapperData, err := os.ReadFile(args[len(args)-1])
	if err != nil {
		t.Fatalf("ReadFile(wrapper) error = %v", err)
	}
	wrapperText := string(wrapperData)
	if !strings.Contains(wrapperText, `from "file://`) {
		t.Fatalf("wrapper missing shared import:\n%s", wrapperText)
	}
	if !strings.Contains(wrapperText, "helpers") || !strings.Contains(wrapperText, "logBanner") {
		t.Fatalf("wrapper missing shared bindings:\n%s", wrapperText)
	}

	output, err := exec.Command(command, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("deno wrapper execution error = %v\n%s", err, string(output))
	}
	text := string(output)
	if !strings.Contains(text, "banner:http://localhost:3000") {
		t.Fatalf("deno output missing banner:\n%s", text)
	}
	if !strings.Contains(text, "demo.txt") {
		t.Fatalf("deno output missing basename:\n%s", text)
	}
	if !strings.Contains(text, "hello-world") {
		t.Fatalf("deno output missing helper output:\n%s", text)
	}
}

func TestLoadAcceptsCSharpRuntimeAndUseCSharp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.vortex")
	data := []byte(`name: dev
csharp:
  usings:
    - System.IO
  packages:
    - name: Newtonsoft.Json
      version: "13.0.3"
  vars:
    apiBase: http://localhost:3000
  functions:
    LogBanner: |
      public static void LogBanner(string text)
      {
          Console.WriteLine(text);
      }
jobs:
  - id: smoke
    shell: csharp
    use: csharp
    command: |
      LogBanner(apiBase);
      Console.WriteLine(Path.GetFileName("/tmp/demo.txt"));
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := cfg.Jobs[0].Use; got != "csharp" {
		t.Fatalf("Load() job use = %q, want %q", got, "csharp")
	}
	if cfg.CSharp.Empty() {
		t.Fatalf("Load() csharp runtime should not be empty")
	}
}

func TestLoadRejectsUseCSharpWithoutCSharpShell(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.vortex")
	data := []byte(`name: dev
csharp:
  vars:
    apiBase: http://localhost:3000
jobs:
  - id: smoke
    shell: bash
    use: csharp
    command: echo hi
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(path)
	if err == nil || !contains(err.Error(), "use: csharp requires shell: csharp") {
		t.Fatalf("Load() error = %v, want csharp shell validation", err)
	}
}

func TestLoadRejectsCSharpRuntimeVarNameConflicts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.vortex")
	data := []byte(`name: dev
csharp:
  vars:
    helper: true
  functions:
    helper: |
      public static void helper()
      {
          Console.WriteLine("ok");
      }
jobs:
  - id: smoke
    shell: csharp
    use: csharp
    command: helper();
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(path)
	if err == nil || !contains(err.Error(), "conflicts") {
		t.Fatalf("Load() error = %v, want name conflict", err)
	}
}

func TestPrepareJobCommandForSharedCSharpRuntime(t *testing.T) {
	if _, err := exec.LookPath("dotnet"); err != nil {
		t.Skip("dotnet is not installed")
	}

	dir := t.TempDir()
	cfg := Config{
		Name:       "dev",
		WorkingDir: dir,
		CSharp: CSharpRuntimeSpec{
			Usings: []string{"System.IO"},
			Vars: map[string]any{
				"apiBase": "http://localhost:3000",
			},
			Functions: map[string]string{
				"LogBanner": `public static void LogBanner(string text)
{
    Console.WriteLine("banner:" + text);
}`,
			},
		},
	}
	if err := cfg.validateCSharpRuntime(); err != nil {
		t.Fatalf("validateCSharpRuntime() error = %v", err)
	}

	job := JobSpec{
		ID:      "smoke",
		Shell:   "csharp",
		Use:     "csharp",
		Command: "LogBanner(apiBase);\nConsole.WriteLine(Path.GetFileName(\"/tmp/demo.txt\"));",
	}

	command, args, err := cfg.PrepareJobCommand(job)
	if err != nil {
		t.Fatalf("PrepareJobCommand() error = %v", err)
	}
	if command != "dotnet" {
		t.Fatalf("PrepareJobCommand() command = %q, want dotnet", command)
	}
	if len(args) < 3 || args[0] != "run" || args[1] != "--project" {
		t.Fatalf("PrepareJobCommand() args = %v, want [run --project <dir>]", args)
	}

	projectDir := args[2]
	programData, err := os.ReadFile(filepath.Join(projectDir, "Program.cs"))
	if err != nil {
		t.Fatalf("ReadFile(Program.cs) error = %v", err)
	}
	programText := string(programData)
	if !strings.Contains(programText, "using static Vortex;") {
		t.Fatalf("Program.cs missing using static:\n%s", programText)
	}
	if !strings.Contains(programText, "LogBanner") {
		t.Fatalf("Program.cs missing LogBanner call:\n%s", programText)
	}

	sharedData, err := os.ReadFile(filepath.Join(projectDir, "Shared.cs"))
	if err != nil {
		t.Fatalf("ReadFile(Shared.cs) error = %v", err)
	}
	sharedText := string(sharedData)
	if !strings.Contains(sharedText, "static class Vortex") {
		t.Fatalf("Shared.cs missing Vortex class:\n%s", sharedText)
	}
	if !strings.Contains(sharedText, "apiBase") {
		t.Fatalf("Shared.cs missing var:\n%s", sharedText)
	}

	output, err := exec.Command(command, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("dotnet run execution error = %v\n%s", err, string(output))
	}
	text := string(output)
	if !strings.Contains(text, "banner:http://localhost:3000") {
		t.Fatalf("csharp output missing banner:\n%s", text)
	}
	if !strings.Contains(text, "demo.txt") {
		t.Fatalf("csharp output missing filename:\n%s", text)
	}
}

func TestPrepareJobCommandForInlineCSharp(t *testing.T) {
	if _, err := exec.LookPath("dotnet"); err != nil {
		t.Skip("dotnet is not installed")
	}

	dir := t.TempDir()
	cfg := Config{
		Name:       "dev",
		WorkingDir: dir,
	}

	job := JobSpec{
		ID:      "inline",
		Shell:   "csharp",
		Command: "Console.WriteLine(\"hello-inline\");",
	}

	command, args, err := cfg.PrepareJobCommand(job)
	if err != nil {
		t.Fatalf("PrepareJobCommand() error = %v", err)
	}
	if command != "dotnet" {
		t.Fatalf("PrepareJobCommand() command = %q, want dotnet", command)
	}

	output, err := exec.Command(command, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("dotnet run execution error = %v\n%s", err, string(output))
	}
	if !strings.Contains(string(output), "hello-inline") {
		t.Fatalf("inline csharp output missing expected text:\n%s", string(output))
	}
}

func TestNodeSourcesNamespaceImport(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	dir := t.TempDir()
	helperPath := filepath.Join(dir, "run-helper.mjs")
	helperSource := `export function run(opts) {
  return "ran:" + opts.command
}
`
	if err := os.WriteFile(helperPath, []byte(helperSource), 0o600); err != nil {
		t.Fatalf("WriteFile(helper) error = %v", err)
	}

	cfg := Config{
		Name:       "dev",
		WorkingDir: dir,
		Node: NodeRuntimeSpec{
			Sources: []string{"./run-helper.mjs"},
			Vars: map[string]any{
				"greeting": "hi",
			},
		},
	}
	if err := cfg.validateNodeRuntime(); err != nil {
		t.Fatalf("validateNodeRuntime() error = %v", err)
	}

	job := JobSpec{
		ID:      "smoke",
		Shell:   "node",
		Use:     "node",
		Command: "console.log(runHelper.run({command: 'test'}))\nconsole.log(greeting)",
	}

	command, args, err := cfg.PrepareJobCommand(job)
	if err != nil {
		t.Fatalf("PrepareJobCommand() error = %v", err)
	}

	output, err := exec.Command(command, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("node execution error = %v\n%s", err, string(output))
	}
	text := string(output)
	if !strings.Contains(text, "ran:test") {
		t.Fatalf("output missing runHelper result:\n%s", text)
	}
	if !strings.Contains(text, "hi") {
		t.Fatalf("output missing greeting var:\n%s", text)
	}
}

func TestCSharpSourcesIncluded(t *testing.T) {
	if _, err := exec.LookPath("dotnet"); err != nil {
		t.Skip("dotnet is not installed")
	}

	dir := t.TempDir()
	helperPath := filepath.Join(dir, "RunHelper.cs")
	helperSource := `public static class RunHelper
{
    public static string Run(string command) => "ran:" + command;
}
`
	if err := os.WriteFile(helperPath, []byte(helperSource), 0o600); err != nil {
		t.Fatalf("WriteFile(helper) error = %v", err)
	}

	cfg := Config{
		Name:       "dev",
		WorkingDir: dir,
		CSharp: CSharpRuntimeSpec{
			Sources: []string{"./RunHelper.cs"},
			Vars: map[string]any{
				"greeting": "hi",
			},
		},
	}
	if err := cfg.validateCSharpRuntime(); err != nil {
		t.Fatalf("validateCSharpRuntime() error = %v", err)
	}

	job := JobSpec{
		ID:      "smoke",
		Shell:   "csharp",
		Use:     "csharp",
		Command: "Console.WriteLine(RunHelper.Run(\"test\"));\nConsole.WriteLine(greeting);",
	}

	command, args, err := cfg.PrepareJobCommand(job)
	if err != nil {
		t.Fatalf("PrepareJobCommand() error = %v", err)
	}

	output, err := exec.Command(command, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("dotnet run execution error = %v\n%s", err, string(output))
	}
	text := string(output)
	if !strings.Contains(text, "ran:test") {
		t.Fatalf("output missing RunHelper result:\n%s", text)
	}
	if !strings.Contains(text, "hi") {
		t.Fatalf("output missing greeting var:\n%s", text)
	}
}

func TestFileNameToNamespace(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"./run-helper.mjs", "runHelper"},
		{"../utils/api.js", "api"},
		{"./MyHelpers.mjs", "myHelpers"},
		{"./foo_bar.ts", "fooBar"},
		{"./simple.mjs", "simple"},
		{"./a-b-c.d.ts", "aBC"},
	}
	for _, tc := range tests {
		got := fileNameToNamespace(tc.input)
		if got != tc.want {
			t.Errorf("fileNameToNamespace(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestDetectCycleNoCycle(t *testing.T) {
	jobs := []JobSpec{
		{ID: "a", Needs: []string{"b"}},
		{ID: "b", Needs: []string{"c"}},
		{ID: "c"},
	}
	if cycle := detectCycle(jobs); cycle != nil {
		t.Fatalf("detectCycle() = %v, want nil", cycle)
	}
}

func TestDetectCycleDirectSelfLoop(t *testing.T) {
	jobs := []JobSpec{
		{ID: "a", Needs: []string{"a"}},
	}
	cycle := detectCycle(jobs)
	if cycle == nil {
		t.Fatal("detectCycle() = nil, want cycle")
	}
	if len(cycle) < 2 || cycle[0] != "a" || cycle[len(cycle)-1] != "a" {
		t.Fatalf("detectCycle() = %v, want [a ... a]", cycle)
	}
}

func TestDetectCycleIndirect(t *testing.T) {
	jobs := []JobSpec{
		{ID: "a", Needs: []string{"b"}},
		{ID: "b", Needs: []string{"c"}},
		{ID: "c", Needs: []string{"a"}},
	}
	cycle := detectCycle(jobs)
	if cycle == nil {
		t.Fatal("detectCycle() = nil, want cycle")
	}
	if len(cycle) < 2 {
		t.Fatalf("detectCycle() = %v, want at least 2 elements", cycle)
	}
}

func TestLoadRejectsCyclicDependencies(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.vortex")
	data := []byte("name: dev\njobs:\n  - id: a\n    command: echo a\n    needs: [b]\n  - id: b\n    command: echo b\n    needs: [a]\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(path)
	if err == nil || !contains(err.Error(), "dependency cycle") {
		t.Fatalf("Load() error = %v, want dependency cycle", err)
	}
}

func TestLoadAcceptsGoRuntimeAndUseGo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.vortex")
	data := []byte(`name: dev
go:
  vars:
    apiBase: http://localhost:3000
  functions:
    logBanner: |
      func logBanner(text string) {
      	fmt.Printf("== %s ==\n", text)
      }
jobs:
  - id: smoke
    shell: go
    use: go
    command: |
      logBanner(apiBase)
      fmt.Println("done")
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := cfg.Jobs[0].Use; got != "go" {
		t.Fatalf("Load() job use = %q, want %q", got, "go")
	}
	if cfg.Go.Empty() {
		t.Fatalf("Load() go runtime should not be empty")
	}
}

func TestLoadRejectsUseGoWithoutGoShell(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.vortex")
	data := []byte(`name: dev
go:
  vars:
    apiBase: http://localhost:3000
jobs:
  - id: smoke
    shell: bash
    use: go
    command: echo hi
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(path)
	if err == nil || !contains(err.Error(), "use: go requires shell: go") {
		t.Fatalf("Load() error = %v, want go shell validation", err)
	}
}

func TestLoadRejectsGoRuntimeVarNameConflicts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.vortex")
	data := []byte(`name: dev
go:
  vars:
    helper: true
  functions:
    helper: |
      func helper() {
      	fmt.Println("ok")
      }
jobs:
  - id: smoke
    shell: go
    use: go
    command: helper()
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(path)
	if err == nil || !contains(err.Error(), "conflicts") {
		t.Fatalf("Load() error = %v, want name conflict", err)
	}
}

func TestPrepareJobCommandForSharedGoRuntime(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}

	dir := t.TempDir()
	cfg := Config{
		Name:       "dev",
		WorkingDir: dir,
		Go: GoRuntimeSpec{
			Vars: map[string]any{
				"apiBase": "http://localhost:3000",
			},
			Functions: map[string]string{
				"logBanner": `func logBanner(text string) {
	fmt.Printf("banner:%s\n", text)
}`,
			},
		},
	}
	if err := cfg.validateGoRuntime(); err != nil {
		t.Fatalf("validateGoRuntime() error = %v", err)
	}

	job := JobSpec{
		ID:      "smoke",
		Shell:   "go",
		Use:     "go",
		Command: "logBanner(apiBase)\nfmt.Println(\"done\")",
	}

	command, args, err := cfg.PrepareJobCommand(job)
	if err != nil {
		t.Fatalf("PrepareJobCommand() error = %v", err)
	}
	if command != "go" {
		t.Fatalf("PrepareJobCommand() command = %q, want go", command)
	}
	if len(args) < 2 || args[0] != "run" || args[1] != "." {
		t.Fatalf("PrepareJobCommand() args = %v, want [run .]", args)
	}

	// Verify generated files.
	runtimeDir, err := goRuntimeDir(cfg)
	if err != nil {
		t.Fatalf("goRuntimeDir() error = %v", err)
	}
	projectDir := filepath.Join(runtimeDir, sanitizeFileComponent(job.ID)+"-"+hashString(job.ID)[:8])

	mainData, err := os.ReadFile(filepath.Join(projectDir, "main.go"))
	if err != nil {
		t.Fatalf("ReadFile(main.go) error = %v", err)
	}
	mainText := string(mainData)
	if !strings.Contains(mainText, "func main()") {
		t.Fatalf("main.go missing func main():\n%s", mainText)
	}
	if !strings.Contains(mainText, "logBanner") {
		t.Fatalf("main.go missing logBanner call:\n%s", mainText)
	}

	sharedData, err := os.ReadFile(filepath.Join(projectDir, "shared.go"))
	if err != nil {
		t.Fatalf("ReadFile(shared.go) error = %v", err)
	}
	sharedText := string(sharedData)
	if !strings.Contains(sharedText, "apiBase") {
		t.Fatalf("shared.go missing var:\n%s", sharedText)
	}
	if !strings.Contains(sharedText, "func logBanner") {
		t.Fatalf("shared.go missing function:\n%s", sharedText)
	}

	// Execute the generated project.
	cmd := exec.Command(command, args...)
	cmd.Dir = projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run execution error = %v\n%s", err, string(output))
	}
	text := string(output)
	if !strings.Contains(text, "banner:http://localhost:3000") {
		t.Fatalf("go output missing banner:\n%s", text)
	}
	if !strings.Contains(text, "done") {
		t.Fatalf("go output missing done:\n%s", text)
	}
}

func TestPrepareJobCommandForInlineGo(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}

	dir := t.TempDir()
	cfg := Config{
		Name:       "dev",
		WorkingDir: dir,
	}

	job := JobSpec{
		ID:      "inline",
		Shell:   "go",
		Command: "println(\"hello-inline\")",
	}

	command, args, err := cfg.PrepareJobCommand(job)
	if err != nil {
		t.Fatalf("PrepareJobCommand() error = %v", err)
	}
	if command != "go" {
		t.Fatalf("PrepareJobCommand() command = %q, want go", command)
	}

	runtimeDir, err := goRuntimeDir(cfg)
	if err != nil {
		t.Fatalf("goRuntimeDir() error = %v", err)
	}
	projectDir := filepath.Join(runtimeDir, sanitizeFileComponent(job.ID)+"-"+hashString(job.ID)[:8])

	cmd := exec.Command(command, args...)
	cmd.Dir = projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run execution error = %v\n%s", err, string(output))
	}
	text := string(output)
	if !strings.Contains(text, "hello-inline") {
		t.Fatalf("go inline output missing expected text:\n%s", text)
	}
}

func TestNodeTypeScriptSources(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	dir := t.TempDir()
	helperPath := filepath.Join(dir, "math-helper.ts")
	helperSource := `export function add(a: number, b: number): number {
  return a + b;
}
`
	if err := os.WriteFile(helperPath, []byte(helperSource), 0o600); err != nil {
		t.Fatalf("WriteFile(helper) error = %v", err)
	}

	cfg := Config{
		Name:       "dev",
		WorkingDir: dir,
		Node: NodeRuntimeSpec{
			Sources: []string{"./math-helper.ts"},
			Vars: map[string]any{
				"greeting": "hello-ts",
			},
		},
	}
	if err := cfg.validateNodeRuntime(); err != nil {
		t.Fatalf("validateNodeRuntime() error = %v", err)
	}

	job := JobSpec{
		ID:      "ts-smoke",
		Shell:   "node",
		Use:     "node",
		Command: "console.log(mathHelper.add(2, 3))\nconsole.log(greeting)",
	}

	command, args, err := cfg.PrepareJobCommand(job)
	if err != nil {
		t.Fatalf("PrepareJobCommand() error = %v", err)
	}

	output, err := exec.Command(command, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("node execution error = %v\n%s", err, string(output))
	}
	text := string(output)
	if !strings.Contains(text, "5") {
		t.Fatalf("output missing add result:\n%s", text)
	}
	if !strings.Contains(text, "hello-ts") {
		t.Fatalf("output missing greeting var:\n%s", text)
	}
}

func TestNodeTypeScriptInlineCommand(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	dir := t.TempDir()
	cfg := Config{
		Name:       "dev",
		WorkingDir: dir,
		Node: NodeRuntimeSpec{
			Typescript: true,
			Functions: map[string]string{
				"greet": "export function greet(name: string): string { return `hello ${name}`; }",
			},
		},
	}
	if err := cfg.validateNodeRuntime(); err != nil {
		t.Fatalf("validateNodeRuntime() error = %v", err)
	}

	job := JobSpec{
		ID:      "ts-inline",
		Shell:   "node",
		Use:     "node",
		Command: "const msg: string = greet('world')\nconsole.log(msg)",
	}

	command, args, err := cfg.PrepareJobCommand(job)
	if err != nil {
		t.Fatalf("PrepareJobCommand() error = %v", err)
	}

	output, err := exec.Command(command, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("node execution error = %v\n%s", err, string(output))
	}
	text := string(output)
	if !strings.Contains(text, "hello world") {
		t.Fatalf("output missing greet result:\n%s", text)
	}
}

func TestBunTypeScriptSources(t *testing.T) {
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun is not installed")
	}

	dir := t.TempDir()
	helperPath := filepath.Join(dir, "math-helper.ts")
	helperSource := `export function add(a: number, b: number): number {
  return a + b;
}
`
	if err := os.WriteFile(helperPath, []byte(helperSource), 0o600); err != nil {
		t.Fatalf("WriteFile(helper) error = %v", err)
	}

	cfg := Config{
		Name:       "dev",
		WorkingDir: dir,
		Bun: BunRuntimeSpec{
			Sources: []string{"./math-helper.ts"},
			Vars: map[string]any{
				"greeting": "hello-bun-ts",
			},
		},
	}
	if err := cfg.validateBunRuntime(); err != nil {
		t.Fatalf("validateBunRuntime() error = %v", err)
	}

	job := JobSpec{
		ID:      "ts-bun",
		Shell:   "bun",
		Use:     "bun",
		Command: "console.log(mathHelper.add(10, 7))\nconsole.log(greeting)",
	}

	command, args, err := cfg.PrepareJobCommand(job)
	if err != nil {
		t.Fatalf("PrepareJobCommand() error = %v", err)
	}

	output, err := exec.Command(command, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("bun execution error = %v\n%s", err, string(output))
	}
	text := string(output)
	if !strings.Contains(text, "17") {
		t.Fatalf("output missing add result:\n%s", text)
	}
	if !strings.Contains(text, "hello-bun-ts") {
		t.Fatalf("output missing greeting var:\n%s", text)
	}
}

func TestJsRuntimeUsesTypeScript(t *testing.T) {
	tests := []struct {
		name       string
		sources    []string
		typescript bool
		want       bool
	}{
		{"explicit true no sources", nil, true, true},
		{"explicit false no sources", nil, false, false},
		{"ts source auto-detect", []string{"./helper.ts"}, false, true},
		{"mts source auto-detect", []string{"./helper.mts"}, false, true},
		{"cts source auto-detect", []string{"./helper.cts"}, false, true},
		{"js source no ts", []string{"./helper.mjs"}, false, false},
		{"mixed sources", []string{"./a.mjs", "./b.ts"}, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := jsRuntimeUsesTypeScript(tc.sources, tc.typescript)
			if got != tc.want {
				t.Fatalf("jsRuntimeUsesTypeScript(%v, %v) = %v, want %v", tc.sources, tc.typescript, got, tc.want)
			}
		})
	}
}
