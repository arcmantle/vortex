package settings

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ShellProfile describes a named shell profile with visual identity.
type ShellProfile struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Command    string   `json:"command"`
	Args       []string `json:"args,omitempty"`
	Color      string   `json:"color,omitempty"`
	Icon       string   `json:"icon,omitempty"` // built-in icon name, path, or data URI
	Default    bool     `json:"default,omitempty"`
	FontFamily string   `json:"fontFamily,omitempty"`
	FontSize   int      `json:"fontSize,omitempty"`
}

// DetectShells discovers available shells on the system and returns a list of
// profiles with sensible defaults. On Unix it reads /etc/shells; on Windows it
// scans known paths.
func DetectShells() []ShellProfile {
	if runtime.GOOS == "windows" {
		return detectShellsWindows()
	}
	return detectShellsUnix()
}

// DefaultShellProfile returns the profile marked as default from the list, or
// the first profile if none is marked default.
func DefaultShellProfile(profiles []ShellProfile) (ShellProfile, bool) {
	for _, p := range profiles {
		if p.Default {
			return p, true
		}
	}
	if len(profiles) > 0 {
		return profiles[0], true
	}
	return ShellProfile{}, false
}

// FindProfile returns the profile with the given ID, or false if not found.
func FindProfile(profiles []ShellProfile, id string) (ShellProfile, bool) {
	for _, p := range profiles {
		if p.ID == id {
			return p, true
		}
	}
	return ShellProfile{}, false
}

func detectShellsUnix() []ShellProfile {
	seen := make(map[string]bool)
	var profiles []ShellProfile

	// Prefer $SHELL as the first (default) entry.
	if userShell := os.Getenv("SHELL"); userShell != "" {
		if _, err := os.Stat(userShell); err == nil {
			p := shellProfileFromPath(userShell)
			p.Default = true
			profiles = append(profiles, p)
			seen[userShell] = true
		}
	}

	// Parse /etc/shells.
	f, err := os.Open("/etc/shells")
	if err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if seen[line] {
				continue
			}
			if _, err := os.Stat(line); err != nil {
				continue
			}
			profiles = append(profiles, shellProfileFromPath(line))
			seen[line] = true
		}
	}

	// If nothing found, add a minimal fallback.
	if len(profiles) == 0 {
		profiles = append(profiles, ShellProfile{
			ID:      "sh",
			Name:    "sh",
			Command: "/bin/sh",
			Args:    []string{"-l"},
			Color:   "#888888",
			Icon:    "terminal",
			Default: true,
		})
	}

	return profiles
}

func detectShellsWindows() []ShellProfile {
	var profiles []ShellProfile

	// PowerShell Core (pwsh).
	if path, err := exec.LookPath("pwsh"); err == nil {
		profiles = append(profiles, ShellProfile{
			ID:      "pwsh",
			Name:    "PowerShell",
			Command: path,
			Args:    []string{"-NoLogo"},
			Color:   "#012456",
			Icon:    "powershell",
			Default: true,
		})
	}

	// Windows PowerShell (legacy).
	legacyPS := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	if _, err := os.Stat(legacyPS); err == nil {
		profiles = append(profiles, ShellProfile{
			ID:      "powershell",
			Name:    "Windows PowerShell",
			Command: legacyPS,
			Args:    []string{"-NoLogo"},
			Color:   "#012456",
			Icon:    "powershell",
			Default: len(profiles) == 0,
		})
	}

	// cmd.exe.
	cmdPath := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	if _, err := os.Stat(cmdPath); err == nil {
		profiles = append(profiles, ShellProfile{
			ID:      "cmd",
			Name:    "Command Prompt",
			Command: cmdPath,
			Color:   "#0c0c0c",
			Icon:    "terminal",
			Default: len(profiles) == 0,
		})
	}

	// Git Bash.
	gitBashPaths := []string{
		`C:\Program Files\Git\bin\bash.exe`,
		`C:\Program Files (x86)\Git\bin\bash.exe`,
	}
	for _, p := range gitBashPaths {
		if _, err := os.Stat(p); err == nil {
			profiles = append(profiles, ShellProfile{
				ID:      "git-bash",
				Name:    "Git Bash",
				Command: p,
				Args:    []string{"--login", "-i"},
				Color:   "#f05032",
				Icon:    "bash",
			})
			break
		}
	}

	// WSL distros.
	profiles = append(profiles, detectWSLDistros()...)

	if len(profiles) == 0 {
		profiles = append(profiles, ShellProfile{
			ID:      "cmd",
			Name:    "Command Prompt",
			Command: "cmd.exe",
			Color:   "#0c0c0c",
			Icon:    "terminal",
			Default: true,
		})
	}

	return profiles
}

func detectWSLDistros() []ShellProfile {
	out, err := exec.Command("wsl", "--list", "--quiet").Output()
	if err != nil {
		return nil
	}
	var profiles []ShellProfile
	for _, line := range strings.Split(string(out), "\n") {
		name := strings.TrimSpace(line)
		// Filter out null bytes from UTF-16 output on Windows.
		name = strings.ReplaceAll(name, "\x00", "")
		if name == "" {
			continue
		}
		id := "wsl-" + strings.ToLower(strings.ReplaceAll(name, " ", "-"))
		profiles = append(profiles, ShellProfile{
			ID:      id,
			Name:    fmt.Sprintf("WSL: %s", name),
			Command: "wsl",
			Args:    []string{"-d", name},
			Color:   "#e95420",
			Icon:    "linux",
		})
	}
	return profiles
}

// shellProfileFromPath creates a ShellProfile from a shell binary path.
func shellProfileFromPath(path string) ShellProfile {
	base := filepath.Base(path)
	name := base
	id := strings.TrimSuffix(base, filepath.Ext(base))

	color := shellColor(id)
	icon := shellIcon(id)

	return ShellProfile{
		ID:      id,
		Name:    name,
		Command: path,
		Args:    shellArgs(id),
		Color:   color,
		Icon:    icon,
	}
}

func shellColor(id string) string {
	switch id {
	case "zsh":
		return "#4ec9b0"
	case "bash":
		return "#d4843e"
	case "fish":
		return "#f5a623"
	case "nu", "nushell":
		return "#3aa675"
	case "pwsh", "powershell":
		return "#012456"
	default:
		return "#888888"
	}
}

func shellIcon(id string) string {
	switch id {
	case "zsh":
		return "zsh"
	case "bash":
		return "bash"
	case "fish":
		return "fish"
	case "nu", "nushell":
		return "nushell"
	case "pwsh", "powershell":
		return "powershell"
	default:
		return "terminal"
	}
}

func shellArgs(id string) []string {
	switch id {
	case "fish":
		return []string{"-l"}
	case "bash":
		return []string{"--login"}
	case "zsh":
		return []string{"-l"}
	case "nu", "nushell":
		return []string{"--login"}
	default:
		return []string{"-l"}
	}
}
