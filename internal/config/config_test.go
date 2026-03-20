package config

import (
	"os"
	"path/filepath"
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

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
