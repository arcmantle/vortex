package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingSettingsReturnsZeroValue(t *testing.T) {
	dir := t.TempDir()
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = os.UserConfigDir })

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Editor != "" {
		t.Fatalf("Load().Editor = %q, want empty", got.Editor)
	}
	if got.Browser != "" {
		t.Fatalf("Load().Browser = %q, want empty", got.Browser)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = os.UserConfigDir })

	if err := Save(Settings{Editor: "  code --reuse-window  ", Browser: "  firefox  "}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	path := filepath.Join(dir, "vortex", "config.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("settings file stat error = %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Editor != "code --reuse-window" {
		t.Fatalf("Load().Editor = %q, want %q", got.Editor, "code --reuse-window")
	}
	if got.Browser != "firefox" {
		t.Fatalf("Load().Browser = %q, want %q", got.Browser, "firefox")
	}
}
