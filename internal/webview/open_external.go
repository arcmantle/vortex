package webview

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"arcmantle/vortex/internal/settings"
)

// startAndReap starts a command and reaps its process entry in the background
// to prevent zombie processes.
func startAndReap(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

type OpenFileTarget struct {
	Path   string
	Line   int
	Column int
}

func OpenExternalURL(target string) error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(target))
	if err != nil {
		return fmt.Errorf("parse external url: %w", err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("unsupported external url scheme %q", parsed.Scheme)
	}

	if cmd, ok := preferredBrowserCommand(parsed.String()); ok {
		if err := startAndReap(cmd); err != nil {
			return fmt.Errorf("open preferred browser: %w", err)
		}
		return nil
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", parsed.String())
	case "linux":
		cmd = exec.Command("xdg-open", parsed.String())
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", parsed.String())
	default:
		return fmt.Errorf("external browser is unsupported on %s", runtime.GOOS)
	}

	if err := startAndReap(cmd); err != nil {
		return fmt.Errorf("open external browser: %w", err)
	}
	return nil
}

func preferredBrowserCommand(target string) (*exec.Cmd, bool) {
	for _, raw := range preferredBrowserCandidates() {
		if raw == "" {
			continue
		}
		parts := splitCommandLine(raw)
		if len(parts) == 0 {
			continue
		}
		args := append([]string{}, parts[1:]...)
		args = append(args, target)
		return exec.Command(parts[0], args...), true
	}
	return nil, false
}

func preferredBrowserCandidates() []string {
	candidates := make([]string, 0, 3)
	if raw := strings.TrimSpace(os.Getenv("VORTEX_BROWSER")); raw != "" {
		candidates = append(candidates, raw)
	}
	if cfg, err := settings.Load(); err == nil && cfg.Browser != "" {
		candidates = append(candidates, cfg.Browser)
	}
	if raw := strings.TrimSpace(os.Getenv("BROWSER")); raw != "" {
		candidates = append(candidates, raw)
	}
	return candidates
}

func RevealPath(target string) error {
	path := strings.TrimSpace(target)
	if path == "" {
		return fmt.Errorf("path must not be empty")
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat path: %w", err)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		if info.IsDir() {
			cmd = exec.Command("open", path)
		} else {
			cmd = exec.Command("open", "-R", path)
		}
	case "linux":
		openPath := path
		if !info.IsDir() {
			openPath = filepath.Dir(path)
		}
		cmd = exec.Command("xdg-open", openPath)
	case "windows":
		if info.IsDir() {
			cmd = exec.Command("explorer", path)
		} else {
			cmd = exec.Command("explorer", "/select,"+path)
		}
	default:
		return fmt.Errorf("path reveal is unsupported on %s", runtime.GOOS)
	}

	if err := startAndReap(cmd); err != nil {
		return fmt.Errorf("reveal path: %w", err)
	}
	return nil
}

func OpenPathInEditor(target OpenFileTarget) error {
	path := strings.TrimSpace(target.Path)
	if path == "" {
		return fmt.Errorf("path must not be empty")
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat path: %w", err)
	}
	if info.IsDir() {
		return RevealPath(path)
	}

	if cmd, ok := preferredEditorCommand(target); ok {
		if err := startAndReap(cmd); err != nil {
			return fmt.Errorf("open in editor: %w", err)
		}
		return nil
	}

	var fallback *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		fallback = exec.Command("open", path)
	case "linux":
		fallback = exec.Command("xdg-open", path)
	case "windows":
		fallback = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		return fmt.Errorf("editor open is unsupported on %s", runtime.GOOS)
	}
	if err := startAndReap(fallback); err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	return nil
}

func preferredEditorCommand(target OpenFileTarget) (*exec.Cmd, bool) {
	for _, raw := range preferredEditorCandidates() {
		if raw == "" {
			continue
		}
		parts := splitCommandLine(raw)
		if len(parts) == 0 {
			continue
		}
		name := filepath.Base(parts[0])
		args := append([]string{}, parts[1:]...)
		args = appendEditorTargetArgs(name, args, target)
		return exec.Command(parts[0], args...), true
	}
	return nil, false
}

func preferredEditorCandidates() []string {
	candidates := make([]string, 0, 4)
	if raw := strings.TrimSpace(os.Getenv("VORTEX_EDITOR")); raw != "" {
		candidates = append(candidates, raw)
	}
	if cfg, err := settings.Load(); err == nil && cfg.Editor != "" {
		candidates = append(candidates, cfg.Editor)
	}
	if raw := strings.TrimSpace(os.Getenv("VISUAL")); raw != "" {
		candidates = append(candidates, raw)
	}
	if raw := strings.TrimSpace(os.Getenv("EDITOR")); raw != "" {
		candidates = append(candidates, raw)
	}
	return candidates
}

func appendEditorTargetArgs(command string, args []string, target OpenFileTarget) []string {
	location := target.Path
	line := target.Line
	column := target.Column
	if column <= 0 {
		column = 1
	}

	switch strings.ToLower(command) {
	case "code", "code-insiders", "codium", "cursor", "windsurf":
		if line > 0 {
			return append(args, "--goto", fmt.Sprintf("%s:%d:%d", target.Path, line, column))
		}
		return append(args, target.Path)
	case "subl", "mate", "zed":
		if line > 0 {
			location = fmt.Sprintf("%s:%d", target.Path, line)
			if target.Column > 0 {
				location = fmt.Sprintf("%s:%d", location, target.Column)
			}
		}
		return append(args, location)
	case "vim", "nvim", "vi", "nano":
		if line > 0 {
			return append(args, fmt.Sprintf("+%d", line), target.Path)
		}
		return append(args, target.Path)
	case "emacs", "emacsclient":
		if line > 0 {
			return append(args, fmt.Sprintf("+%d:%d", line, column), target.Path)
		}
		return append(args, target.Path)
	default:
		return append(args, target.Path)
	}
}

// splitCommandLine splits a command string into tokens, respecting single and
// double quotes so that paths containing spaces (common on Windows and macOS)
// are preserved as a single token. Unmatched trailing quotes are treated as
// ending at end-of-string. Examples:
//
//	`"C:\Program Files\app.exe" --flag`  → ["C:\Program Files\app.exe", "--flag"]
//	`code --reuse-window`                → ["code", "--reuse-window"]
//	`'/Applications/My App.app' -g`      → ["/Applications/My App.app", "-g"]
func splitCommandLine(s string) []string {
	var parts []string
	var current []byte
	inSingle := false
	inDouble := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case (c == ' ' || c == '\t') && !inSingle && !inDouble:
			if len(current) > 0 {
				parts = append(parts, string(current))
				current = current[:0]
			}
		default:
			current = append(current, c)
		}
	}
	if len(current) > 0 {
		parts = append(parts, string(current))
	}
	return parts
}
