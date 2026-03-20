package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveInitConfigPath(t *testing.T) {
	tests := []struct {
		path    string
		want    string
		wantErr string
	}{
		{path: "", want: defaultInitConfigPath},
		{path: "dev", want: "dev.vortex"},
		{path: "configs/dev", want: filepath.Join("configs", "dev.vortex")},
		{path: "dev.vortex", want: "dev.vortex"},
		{path: "dev.yaml", wantErr: "must end in .vortex"},
	}

	for _, tc := range tests {
		got, err := resolveInitConfigPath(tc.path)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("resolveInitConfigPath(%q) error = %v, want substring %q", tc.path, err, tc.wantErr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("resolveInitConfigPath(%q) error = %v", tc.path, err)
		}
		if got != tc.want {
			t.Fatalf("resolveInitConfigPath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestSchemaURLForVersion(t *testing.T) {
	tests := []struct {
		version string
		want    string
	}{
		{version: "dev", want: "https://raw.githubusercontent.com/arcmantle/vortex/master/schemas/vortex.schema.json"},
		{version: "", want: "https://raw.githubusercontent.com/arcmantle/vortex/master/schemas/vortex.schema.json"},
		{version: "1.2.3", want: "https://raw.githubusercontent.com/arcmantle/vortex/v1.2.3/schemas/vortex.schema.json"},
		{version: "v1.2.3", want: "https://raw.githubusercontent.com/arcmantle/vortex/v1.2.3/schemas/vortex.schema.json"},
	}

	for _, tc := range tests {
		if got := schemaURLForVersion(tc.version); got != tc.want {
			t.Fatalf("schemaURLForVersion(%q) = %q, want %q", tc.version, got, tc.want)
		}
	}
}

func TestRunInitCommandWritesTemplate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.vortex")

	if err := runInitCommand(path, false); err != nil {
		t.Fatalf("runInitCommand() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "# yaml-language-server: $schema=") {
		t.Fatalf("template missing schema comment:\n%s", text)
	}
	if !strings.Contains(text, "name: demo") {
		t.Fatalf("template missing inferred name:\n%s", text)
	}
	if !strings.Contains(text, "shell: node") {
		t.Fatalf("template missing shell example:\n%s", text)
	}
}