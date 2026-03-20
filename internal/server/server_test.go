package server

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestNormalizeTerminalPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "src/main.go:12:4", want: "src/main.go"},
		{input: "./internal/server/server.go:88", want: "./internal/server/server.go"},
		{input: "'file:///tmp/example.txt:8'", want: "/tmp/example.txt"},
	}

	for _, tc := range tests {
		if got := normalizeTerminalPath(tc.input); got != tc.want {
			t.Fatalf("normalizeTerminalPath(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestParseTerminalPath(t *testing.T) {
	tests := []struct {
		input      string
		wantPath   string
		wantLine   int
		wantColumn int
	}{
		{input: "src/main.go:12:4", wantPath: "src/main.go", wantLine: 12, wantColumn: 4},
		{input: "./internal/server/server.go:88", wantPath: "./internal/server/server.go", wantLine: 88, wantColumn: 0},
		{input: "'file:///tmp/example.txt:8'", wantPath: "/tmp/example.txt", wantLine: 8, wantColumn: 0},
	}

	for _, tc := range tests {
		got := parseTerminalPath(tc.input)
		if got.Path != tc.wantPath || got.Line != tc.wantLine || got.Column != tc.wantColumn {
			t.Fatalf("parseTerminalPath(%q) = %+v, want path=%q line=%d column=%d", tc.input, got, tc.wantPath, tc.wantLine, tc.wantColumn)
		}
	}
}

func TestResolveRevealPath(t *testing.T) {
	workDir := filepath.Join(string(filepath.Separator), "workspace")
	got, err := resolveRevealPath(workDir, "src/main.go:12")
	if err != nil {
		t.Fatalf("resolveRevealPath() error = %v", err)
	}
	want := filepath.Join(workDir, "src", "main.go")
	if got != want {
		t.Fatalf("resolveRevealPath() = %q, want %q", got, want)
	}
}

func TestResolveOpenPathTarget(t *testing.T) {
	workDir := filepath.Join(string(filepath.Separator), "workspace")
	got, err := resolveOpenPathTarget(workDir, "src/main.go:12:7")
	if err != nil {
		t.Fatalf("resolveOpenPathTarget() error = %v", err)
	}
	want := filepath.Join(workDir, "src", "main.go")
	if got.Path != want || got.Line != 12 || got.Column != 7 {
		t.Fatalf("resolveOpenPathTarget() = %+v, want path=%q line=12 column=7", got, want)
	}
}

func TestStripLineColumnSuffixPreservesWindowsDrive(t *testing.T) {
	path := `C:\repo\main.go:12:7`
	want := `C:\repo\main.go`
	if runtime.GOOS == "windows" {
		if got := stripLineColumnSuffix(path); got != want {
			t.Fatalf("stripLineColumnSuffix(%q) = %q, want %q", path, got, want)
		}
		return
	}
	if got := stripLineColumnSuffix(path); got != want {
		t.Fatalf("stripLineColumnSuffix(%q) = %q, want %q", path, got, want)
	}
}
