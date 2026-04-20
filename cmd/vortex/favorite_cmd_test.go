package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"arcmantle/vortex/internal/favorites"
)

func TestFavoriteAddAndList(t *testing.T) {
	dir := t.TempDir()
	favorites.SetUserConfigDir(func() (string, error) { return dir, nil })
	t.Cleanup(func() { favorites.SetUserConfigDir(os.UserConfigDir) })

	configFile := filepath.Join(dir, "test.vortex")
	if err := os.WriteFile(configFile, []byte("name: test\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Capture stdout for the add command.
	addOut := captureStdout(t, func() {
		cmd := favoriteCommand()
		cmd.SetArgs([]string{"add", "my-app", configFile})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("favorite add error = %v", err)
		}
	})

	if !strings.Contains(addOut, "my-app") {
		t.Fatalf("favorite add output = %q, want to contain alias", addOut)
	}

	// Capture stdout for the list command.
	listOut := captureStdout(t, func() {
		cmd := favoriteCommand()
		cmd.SetArgs([]string{"list"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("favorite list error = %v", err)
		}
	})

	if !strings.Contains(listOut, "@my-app") {
		t.Fatalf("favorite list output = %q, want to contain @my-app", listOut)
	}
}

func TestFavoriteRemove(t *testing.T) {
	dir := t.TempDir()
	favorites.SetUserConfigDir(func() (string, error) { return dir, nil })
	t.Cleanup(func() { favorites.SetUserConfigDir(os.UserConfigDir) })

	configFile := filepath.Join(dir, "test.vortex")
	os.WriteFile(configFile, []byte("name: test\n"), 0o644)
	favorites.Add("demo", configFile)

	cmd := favoriteCommand()
	cmd.SetArgs([]string{"remove", "demo"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("favorite remove error = %v", err)
	}

	_, err := favorites.Resolve("demo")
	if err == nil {
		t.Fatal("Resolve() should fail after remove")
	}
}

func TestFavoriteListEmpty(t *testing.T) {
	dir := t.TempDir()
	favorites.SetUserConfigDir(func() (string, error) { return dir, nil })
	t.Cleanup(func() { favorites.SetUserConfigDir(os.UserConfigDir) })

	out := captureStdout(t, func() {
		cmd := favoriteCommand()
		cmd.SetArgs([]string{"list"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("favorite list error = %v", err)
		}
	})

	if !strings.Contains(out, "No favorites") {
		t.Fatalf("favorite list output = %q, want 'No favorites' message", out)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	defer readEnd.Close()

	stdout := os.Stdout
	os.Stdout = writeEnd
	t.Cleanup(func() { os.Stdout = stdout })

	fn()
	writeEnd.Close()

	output, err := io.ReadAll(readEnd)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	os.Stdout = stdout
	return string(output)
}
