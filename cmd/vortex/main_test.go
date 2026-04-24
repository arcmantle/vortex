package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIsSupportedConfigPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "dev", want: true},
		{path: "dev.vortex", want: true},
		{path: "dev.vortex.yaml", want: false},
		{path: "dev.yaml", want: false},
		{path: "dev.yml", want: false},
		{path: "dev.txt", want: false},
	}

	for _, tc := range tests {
		if got := isSupportedConfigPath(tc.path); got != tc.want {
			t.Fatalf("isSupportedConfigPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestResolveConfigPath(t *testing.T) {
	tests := []struct {
		path    string
		want    string
		wantErr bool
	}{
		{path: "dev", want: "dev.vortex"},
		{path: filepath.Join("configs", "dev"), want: filepath.Join("configs", "dev.vortex")},
		{path: "dev.vortex", want: "dev.vortex"},
		{path: "dev.yaml", wantErr: true},
		{path: "", wantErr: true},
	}

	for _, tc := range tests {
		got, err := resolveConfigPath(tc.path)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("resolveConfigPath(%q) error = nil, want error", tc.path)
			}
			continue
		}
		if err != nil {
			t.Fatalf("resolveConfigPath(%q) error = %v", tc.path, err)
		}
		if got != tc.want {
			t.Fatalf("resolveConfigPath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestResolveWorkingDir(t *testing.T) {
	configPath := filepath.Join("configs", "dev.vortex")

	got, err := resolveWorkingDir(configPath, "")
	if err != nil {
		t.Fatalf("resolveWorkingDir() error = %v", err)
	}
	want := filepath.Dir(configPath)
	if got != want {
		t.Fatalf("resolveWorkingDir() = %q, want %q", got, want)
	}
}

func TestCwdFromRunArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{name: "missing", args: []string{"run", "dev.vortex"}, want: ""},
		{name: "separate flag", args: []string{"run", "--cwd", "subdir", "dev.vortex"}, want: "subdir"},
		{name: "equals flag", args: []string{"run", "--cwd=subdir", "dev.vortex"}, want: "subdir"},
		{name: "missing value", args: []string{"run", "--cwd"}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cwdFromRunArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatal("cwdFromRunArgs() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("cwdFromRunArgs() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("cwdFromRunArgs() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestShouldDetachFromTerminal(t *testing.T) {
	tests := []struct {
		name string
		opts cliOptions
		want bool
	}{
		{name: "windowed release", opts: cliOptions{}, want: true},
		{name: "headless release", opts: cliOptions{headless: true}, want: true},
		{name: "dev mode", opts: cliOptions{dev: true}, want: false},
		{name: "already forked", opts: cliOptions{forked: true}, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldDetachFromTerminal(tc.opts); got != tc.want {
				t.Fatalf("shouldDetachFromTerminal(%+v) = %v, want %v", tc.opts, got, tc.want)
			}
		})
	}
}

func TestShouldAttachConsoleForCLI(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "root help", args: nil, want: true},
		{name: "global help flag", args: []string{"--help"}, want: true},
		{name: "run help flag", args: []string{"run", "--help"}, want: true},
		{name: "version flag", args: []string{"--version"}, want: true},
		{name: "version command", args: []string{"version"}, want: true},
		{name: "config command", args: []string{"config", "list"}, want: true},
		{name: "instance command", args: []string{"instance", "list"}, want: true},
		{name: "docs command", args: []string{"docs"}, want: true},
		{name: "run command", args: []string{"run", "dev.vortex"}, want: false},
		{name: "run command with flags", args: []string{"run", "--dev", "dev.vortex"}, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldAttachConsoleForCLI(tc.args); got != tc.want {
				t.Fatalf("shouldAttachConsoleForCLI(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestWindowBinaryNameUsesAbsolutePathForRelativeLookup(t *testing.T) {
	tempDir := t.TempDir()
	oldExecutablePath := resolveExecutablePath
	oldLookupWindowBinary := lookupWindowBinary
	t.Cleanup(func() {
		resolveExecutablePath = oldExecutablePath
		lookupWindowBinary = oldLookupWindowBinary
	})

	resolveExecutablePath = func() (string, error) {
		return filepath.Join(tempDir, "bin", "vortex.exe"), nil
	}
	lookupWindowBinary = func(string) (string, error) {
		return "vortex-window.exe", exec.ErrDot
	}

	want, err := filepath.Abs("vortex-window.exe")
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}
	if got := windowBinaryName(); got != want {
		t.Fatalf("windowBinaryName() = %q, want %q", got, want)
	}
}

func TestWindowBinaryNamePrefersSiblingExecutable(t *testing.T) {
	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	sibling := filepath.Join(binDir, "vortex-window.exe")
	if err := os.WriteFile(sibling, []byte("stub"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	oldExecutablePath := resolveExecutablePath
	oldLookupWindowBinary := lookupWindowBinary
	t.Cleanup(func() {
		resolveExecutablePath = oldExecutablePath
		lookupWindowBinary = oldLookupWindowBinary
	})

	resolveExecutablePath = func() (string, error) {
		return filepath.Join(binDir, "vortex.exe"), nil
	}
	lookupWindowBinary = func(string) (string, error) {
		return "", errors.New("should not be called")
	}

	if got := windowBinaryName(); got != sibling {
		t.Fatalf("windowBinaryName() = %q, want %q", got, sibling)
	}
}
