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
			Imports: []NodeImportSpec{
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
