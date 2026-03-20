package main

import (
	"testing"

	"arcmantle/vortex/internal/settings"
)

func TestTerminalClickablePath(t *testing.T) {
	got := terminalClickablePath("/Users/roen/Library/Application Support/vortex/config.json")
	want := "file:///Users/roen/Library/Application%20Support/vortex/config.json"
	if got != want {
		t.Fatalf("terminalClickablePath() = %q, want %q", got, want)
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
