package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Settings struct {
	Editor          string         `json:"editor,omitempty"`
	Browser         string         `json:"browser,omitempty"`
	Shells          []ShellProfile `json:"shells,omitempty"`
	FontFamily      string         `json:"fontFamily,omitempty"`
	FontSize        int            `json:"fontSize,omitempty"`
	Theme           string         `json:"theme,omitempty"`
	BackgroundImage string         `json:"backgroundImage,omitempty"`
}

var userConfigDir = os.UserConfigDir

func SetUserConfigDir(fn func() (string, error)) {
	userConfigDir = fn
}

func UserConfigDir() func() (string, error) {
	return userConfigDir
}

func Load() (Settings, error) {
	path, err := filePath()
	if err != nil {
		return Settings{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Settings{}, nil
		}
		return Settings{}, fmt.Errorf("read settings: %w", err)
	}

	var cfg Settings
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Settings{}, fmt.Errorf("parse settings: %w", err)
	}
	cfg.normalize()
	return cfg, nil
}

func Save(cfg Settings) error {
	path, err := filePath()
	if err != nil {
		return err
	}

	cfg.normalize()
	if err := ensureDir(path); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	data = append(data, '\n')

	// Atomic write: write to a temp file in the same directory, sync,
	// then rename over the target. Prevents corruption on crash.
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create settings temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write settings: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("sync settings: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close settings: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename settings: %w", err)
	}
	return nil
}

func Path() (string, error) {
	return filePath()
}

func EnsureDir() error {
	path, err := filePath()
	if err != nil {
		return err
	}
	return ensureDir(path)
}

func filePath() (string, error) {
	dir, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	if strings.TrimSpace(dir) == "" {
		return "", fmt.Errorf("resolve user config directory: empty path")
	}
	return filepath.Join(dir, "vortex", "config.json"), nil
}

func ensureDir(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}
	return nil
}

func (cfg *Settings) normalize() {
	cfg.Editor = strings.TrimSpace(cfg.Editor)
	cfg.Browser = strings.TrimSpace(cfg.Browser)
}
