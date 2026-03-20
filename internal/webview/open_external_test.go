package webview

import (
	"os"
	"reflect"
	"testing"

	"arcmantle/vortex/internal/settings"
)

func TestPreferredEditorCandidatesIncludesSavedSetting(t *testing.T) {
	dir := t.TempDir()
	previousDir := settings.UserConfigDir()
	settings.SetUserConfigDir(func() (string, error) { return dir, nil })
	t.Cleanup(func() { settings.SetUserConfigDir(previousDir) })

	for _, envKey := range []string{"VORTEX_EDITOR", "VISUAL", "EDITOR"} {
		oldValue, ok := os.LookupEnv(envKey)
		_ = os.Unsetenv(envKey)
		t.Cleanup(func() {
			if ok {
				_ = os.Setenv(envKey, oldValue)
			} else {
				_ = os.Unsetenv(envKey)
			}
		})
	}

	if err := settings.Save(settings.Settings{Editor: "code --reuse-window"}); err != nil {
		t.Fatalf("settings.Save() error = %v", err)
	}

	got := preferredEditorCandidates()
	want := []string{"code --reuse-window"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("preferredEditorCandidates() = %#v, want %#v", got, want)
	}
}

func TestPreferredBrowserCandidatesIncludesSavedSetting(t *testing.T) {
	dir := t.TempDir()
	previousDir := settings.UserConfigDir()
	settings.SetUserConfigDir(func() (string, error) { return dir, nil })
	t.Cleanup(func() { settings.SetUserConfigDir(previousDir) })

	for _, envKey := range []string{"VORTEX_BROWSER", "BROWSER"} {
		oldValue, ok := os.LookupEnv(envKey)
		_ = os.Unsetenv(envKey)
		t.Cleanup(func() {
			if ok {
				_ = os.Setenv(envKey, oldValue)
			} else {
				_ = os.Unsetenv(envKey)
			}
		})
	}

	if err := settings.Save(settings.Settings{Browser: "firefox"}); err != nil {
		t.Fatalf("settings.Save() error = %v", err)
	}

	got := preferredBrowserCandidates()
	want := []string{"firefox"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("preferredBrowserCandidates() = %#v, want %#v", got, want)
	}
}
