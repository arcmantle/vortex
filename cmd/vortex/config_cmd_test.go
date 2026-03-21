package main

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"arcmantle/vortex/internal/settings"
)

func TestTerminalClickablePath(t *testing.T) {
	path := filepath.Join("Users", "roen", "Library", "Application Support", "vortex", "config.json")
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}

	got := terminalClickablePath(path)
	uri := (&url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}).String()
	want := fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", uri, absolute)
	if got != want {
		t.Fatalf("terminalClickablePath() = %q, want %q", got, want)
	}
}

func TestPrintConfigValuesUsesSingleDirLine(t *testing.T) {
	dir := t.TempDir()
	settings.SetUserConfigDir(func() (string, error) { return dir, nil })
	t.Cleanup(func() { settings.SetUserConfigDir(os.UserConfigDir) })

	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	defer readEnd.Close()

	stdout := os.Stdout
	os.Stdout = writeEnd
	t.Cleanup(func() { os.Stdout = stdout })

	if err := printConfigValues(settings.Settings{Browser: "firefox", Editor: "code"}); err != nil {
		t.Fatalf("printConfigValues() error = %v", err)
	}
	if err := writeEnd.Close(); err != nil {
		t.Fatalf("writeEnd.Close() error = %v", err)
	}

	output, err := io.ReadAll(readEnd)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}

	path, err := settings.Path()
	if err != nil {
		t.Fatalf("settings.Path() error = %v", err)
	}
	dirURL := (&url.URL{Scheme: "file", Path: filepath.Dir(path)}).String()
	wantPrefix := fmt.Sprintf("dir=%s\n", dirURL)
	if !strings.HasPrefix(string(output), wantPrefix) {
		t.Fatalf("printConfigValues() output = %q, want prefix %q", string(output), wantPrefix)
	}
	if strings.Contains(string(output), "dir:\n") {
		t.Fatalf("printConfigValues() output = %q, want single-line dir output", string(output))
	}
	if strings.Contains(string(output), fmt.Sprintf("dir=%s\n", filepath.Dir(path))) {
		t.Fatalf("printConfigValues() output = %q, want dir URL rather than raw path", string(output))
	}
	if strings.Contains(string(output), "\x1b]8;;") {
		t.Fatalf("printConfigValues() output = %q, want plain dir URL without hyperlink escape codes", string(output))
	}
}

func TestSetConfigValue(t *testing.T) {
	var cfg settings.Settings
	if err := setConfigValue(&cfg, "editor", "  code --goto  "); err != nil {
		t.Fatalf("setConfigValue() error = %v", err)
	}
	if cfg.Editor != "code --goto" {
		t.Fatalf("setConfigValue() editor = %q, want %q", cfg.Editor, "code --goto")
	}
	if err := setConfigValue(&cfg, "browser", "  firefox  "); err != nil {
		t.Fatalf("setConfigValue() error = %v", err)
	}
	if cfg.Browser != "firefox" {
		t.Fatalf("setConfigValue() browser = %q, want %q", cfg.Browser, "firefox")
	}
}

func TestConfigValueRejectsUnknownKey(t *testing.T) {
	if _, err := configValue(settings.Settings{}, "unknown"); err == nil {
		t.Fatal("configValue() error = nil, want error")
	}
}

func TestUnsetConfigValue(t *testing.T) {
	cfg := settings.Settings{Editor: "code", Browser: "firefox"}
	if err := unsetConfigValue(&cfg, "editor"); err != nil {
		t.Fatalf("unsetConfigValue() error = %v", err)
	}
	if cfg.Editor != "" {
		t.Fatalf("unsetConfigValue() editor = %q, want empty", cfg.Editor)
	}
	if err := unsetConfigValue(&cfg, "browser"); err != nil {
		t.Fatalf("unsetConfigValue() error = %v", err)
	}
	if cfg.Browser != "" {
		t.Fatalf("unsetConfigValue() browser = %q, want empty", cfg.Browser)
	}
}
