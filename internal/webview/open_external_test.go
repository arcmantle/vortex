package webview

import (
	"os"
	"reflect"
	"testing"

	"arcmantle/vortex/internal/settings"
)

func TestSplitCommandLine(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{input: "code --reuse-window", want: []string{"code", "--reuse-window"}},
		{input: `"C:\Program Files\app.exe" --flag`, want: []string{`C:\Program Files\app.exe`, "--flag"}},
		{input: `'/Applications/My App.app/Contents/MacOS/app' -g`, want: []string{"/Applications/My App.app/Contents/MacOS/app", "-g"}},
		{input: `  spaced   args  `, want: []string{"spaced", "args"}},
		{input: `"quoted path" 'single quoted' plain`, want: []string{"quoted path", "single quoted", "plain"}},
		{input: "", want: nil},
		{input: "   ", want: nil},
		{input: "single", want: []string{"single"}},
		{input: `"unmatched quote`, want: []string{"unmatched quote"}},
	}

	for _, tc := range tests {
		got := splitCommandLine(tc.input)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitCommandLine(%q) = %#v, want %#v", tc.input, got, tc.want)
		}
	}
}

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
