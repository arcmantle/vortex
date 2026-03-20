package main

import (
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
