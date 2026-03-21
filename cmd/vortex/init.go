package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultInitConfigPath = "dev.vortex"

func runInitCommand(path string, force bool) error {
	resolvedPath, err := resolveInitConfigPath(path)
	if err != nil {
		return err
	}

	if !force {
		if _, err := os.Stat(resolvedPath); err == nil {
			return fmt.Errorf("config already exists: %s (use --force to overwrite)", resolvedPath)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat config path: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(resolvedPath), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	body := renderInitTemplate(filepath.Base(resolvedPath), Version)
	if err := os.WriteFile(resolvedPath, []byte(body), 0o644); err != nil {
		return fmt.Errorf("write config template: %w", err)
	}

	fmt.Printf("Created Vortex config template:\n%s\n", terminalClickablePath(resolvedPath))
	return nil
}

func resolveInitConfigPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = defaultInitConfigPath
	}
	path = filepath.Clean(path)

	lower := strings.ToLower(path)
	ext := filepath.Ext(lower)
	if ext == "" {
		path += ".vortex"
		lower = strings.ToLower(path)
		ext = filepath.Ext(lower)
	}
	if ext != ".vortex" {
		return "", fmt.Errorf("config path must end in .vortex")
	}

	return path, nil
}

func renderInitTemplate(path, version string) string {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if strings.TrimSpace(name) == "" {
		name = "dev"
	}

	return fmt.Sprintf(`# yaml-language-server: $schema=%s
name: %s

jobs:
  - id: app
    label: App
    command: echo "hello from vortex"
    group: dev

  - id: smoke-node
    label: Node Smoke
    shell: node
    command: |
      console.log('hello from vortex')
      console.log(process.version)
    needs: [app]
`, schemaURLForVersion(version), name)
}

func schemaURLForVersion(version string) string {
	version = strings.TrimSpace(version)
	ref := docsDevRef
	if version != "" && version != "dev" && version != "unknown" {
		if !strings.HasPrefix(version, "v") {
			version = "v" + version
		}
		ref = version
	}
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/schemas/vortex.schema.json", docsRepoOwner, docsRepoName, ref)
}
