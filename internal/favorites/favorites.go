package favorites

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry represents a single favorite: an alias pointing to an absolute config path.
type Entry struct {
	Alias string
	Path  string
}

var userConfigDir = os.UserConfigDir

func SetUserConfigDir(fn func() (string, error)) {
	userConfigDir = fn
}

// Load returns all saved favorites. Returns an empty map when no file exists.
func Load() (map[string]string, error) {
	path, err := filePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return nil, fmt.Errorf("read favorites: %w", err)
	}

	var favs map[string]string
	if err := json.Unmarshal(data, &favs); err != nil {
		return nil, fmt.Errorf("parse favorites: %w", err)
	}
	if favs == nil {
		favs = make(map[string]string)
	}
	return favs, nil
}

// Save persists all favorites to disk using an atomic write.
func Save(favs map[string]string) error {
	path, err := filePath()
	if err != nil {
		return err
	}

	if err := ensureDir(path); err != nil {
		return err
	}

	data, err := json.MarshalIndent(favs, "", "  ")
	if err != nil {
		return fmt.Errorf("encode favorites: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".favorites-*.tmp")
	if err != nil {
		return fmt.Errorf("create favorites temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write favorites: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("sync favorites: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close favorites: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename favorites: %w", err)
	}
	return nil
}

// Add saves an alias→path mapping. The path is resolved to absolute.
// Returns an error if the alias is invalid or the path does not exist.
func Add(alias, configPath string) error {
	alias = normalizeAlias(alias)
	if err := validateAlias(alias); err != nil {
		return err
	}

	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("config file not found: %s", absPath)
	}

	favs, err := Load()
	if err != nil {
		return err
	}
	favs[alias] = absPath
	return Save(favs)
}

// Remove deletes a favorite by alias.
func Remove(alias string) error {
	alias = normalizeAlias(alias)
	favs, err := Load()
	if err != nil {
		return err
	}
	if _, ok := favs[alias]; !ok {
		return fmt.Errorf("favorite %q not found", alias)
	}
	delete(favs, alias)
	return Save(favs)
}

// Resolve returns the absolute config path for a given alias.
func Resolve(alias string) (string, error) {
	alias = normalizeAlias(alias)
	favs, err := Load()
	if err != nil {
		return "", err
	}
	path, ok := favs[alias]
	if !ok {
		return "", fmt.Errorf("favorite %q not found", alias)
	}
	return path, nil
}

// List returns all favorites sorted by alias.
func List() ([]Entry, error) {
	favs, err := Load()
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(favs))
	for alias, path := range favs {
		entries = append(entries, Entry{Alias: alias, Path: path})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Alias < entries[j].Alias
	})
	return entries, nil
}

// IsFavoriteRef returns true if the argument looks like a favorite reference (@alias).
func IsFavoriteRef(arg string) bool {
	return strings.HasPrefix(arg, "@") && len(arg) > 1
}

// ParseFavoriteRef strips the leading "@" and returns the alias.
func ParseFavoriteRef(arg string) string {
	return strings.TrimPrefix(arg, "@")
}

func validateAlias(alias string) error {
	if alias == "" {
		return fmt.Errorf("favorite alias must not be empty")
	}
	for _, ch := range alias {
		if !isValidAliasChar(ch) {
			return fmt.Errorf("favorite alias %q contains invalid character %q; use letters, digits, hyphens, underscores, or dots", alias, string(ch))
		}
	}
	return nil
}

func isValidAliasChar(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') ||
		ch == '-' || ch == '_' || ch == '.'
}

func normalizeAlias(alias string) string {
	return strings.TrimSpace(alias)
}

func filePath() (string, error) {
	dir, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	if strings.TrimSpace(dir) == "" {
		return "", fmt.Errorf("resolve user config directory: empty path")
	}
	return filepath.Join(dir, "vortex", "favorites.json"), nil
}

func ensureDir(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create favorites directory: %w", err)
	}
	return nil
}
