package favorites

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsEmptyMap(t *testing.T) {
	dir := t.TempDir()
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = os.UserConfigDir })

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Load() = %v, want empty map", got)
	}
}

func TestAddAndResolveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = os.UserConfigDir })

	// Create a dummy config file to add as favorite.
	configFile := filepath.Join(dir, "test.vortex")
	if err := os.WriteFile(configFile, []byte("name: test\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := Add("my-app", configFile); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	got, err := Resolve("my-app")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != configFile {
		t.Fatalf("Resolve() = %q, want %q", got, configFile)
	}
}

func TestAddOverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = os.UserConfigDir })

	file1 := filepath.Join(dir, "a.vortex")
	file2 := filepath.Join(dir, "b.vortex")
	os.WriteFile(file1, []byte("name: a\n"), 0o644)
	os.WriteFile(file2, []byte("name: b\n"), 0o644)

	Add("app", file1)
	Add("app", file2)

	got, err := Resolve("app")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != file2 {
		t.Fatalf("Resolve() = %q, want %q", got, file2)
	}
}

func TestAddRejectsNonexistentFile(t *testing.T) {
	dir := t.TempDir()
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = os.UserConfigDir })

	err := Add("broken", filepath.Join(dir, "nope.vortex"))
	if err == nil {
		t.Fatal("Add() error = nil, want error for missing file")
	}
}

func TestAddRejectsInvalidAlias(t *testing.T) {
	dir := t.TempDir()
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = os.UserConfigDir })

	configFile := filepath.Join(dir, "test.vortex")
	os.WriteFile(configFile, []byte("name: test\n"), 0o644)

	tests := []string{"", "has space", "a/b", "@nope"}
	for _, alias := range tests {
		if err := Add(alias, configFile); err == nil {
			t.Fatalf("Add(%q) error = nil, want error", alias)
		}
	}
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = os.UserConfigDir })

	configFile := filepath.Join(dir, "test.vortex")
	os.WriteFile(configFile, []byte("name: test\n"), 0o644)

	Add("app", configFile)
	if err := Remove("app"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	_, err := Resolve("app")
	if err == nil {
		t.Fatal("Resolve() error = nil after Remove(), want error")
	}
}

func TestRemoveNotFoundReturnsError(t *testing.T) {
	dir := t.TempDir()
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = os.UserConfigDir })

	err := Remove("nonexistent")
	if err == nil {
		t.Fatal("Remove() error = nil, want error")
	}
}

func TestListReturnsSorted(t *testing.T) {
	dir := t.TempDir()
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = os.UserConfigDir })

	fileA := filepath.Join(dir, "a.vortex")
	fileB := filepath.Join(dir, "b.vortex")
	fileC := filepath.Join(dir, "c.vortex")
	os.WriteFile(fileA, []byte("name: a\n"), 0o644)
	os.WriteFile(fileB, []byte("name: b\n"), 0o644)
	os.WriteFile(fileC, []byte("name: c\n"), 0o644)

	Add("zebra", fileC)
	Add("alpha", fileA)
	Add("middle", fileB)

	entries, err := List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("List() returned %d entries, want 3", len(entries))
	}
	if entries[0].Alias != "alpha" || entries[1].Alias != "middle" || entries[2].Alias != "zebra" {
		t.Fatalf("List() order = [%s, %s, %s], want [alpha, middle, zebra]", entries[0].Alias, entries[1].Alias, entries[2].Alias)
	}
}

func TestIsFavoriteRef(t *testing.T) {
	tests := []struct {
		arg  string
		want bool
	}{
		{"@myapp", true},
		{"@dev", true},
		{"@", false},
		{"myapp", false},
		{"./config.vortex", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := IsFavoriteRef(tc.arg); got != tc.want {
			t.Fatalf("IsFavoriteRef(%q) = %v, want %v", tc.arg, got, tc.want)
		}
	}
}

func TestParseFavoriteRef(t *testing.T) {
	if got := ParseFavoriteRef("@myapp"); got != "myapp" {
		t.Fatalf("ParseFavoriteRef(@myapp) = %q, want %q", got, "myapp")
	}
}
