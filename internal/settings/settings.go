package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Settings struct {
	Editor  string `json:"editor,omitempty"`
	Browser string `json:"browser,omitempty"`
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

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write settings: %w", err)
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}
	return nil
}

func (cfg *Settings) normalize() {
	cfg.Editor = strings.TrimSpace(cfg.Editor)
	cfg.Browser = strings.TrimSpace(cfg.Browser)
}
