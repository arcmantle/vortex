package server

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"

	"arcmantle/vortex/internal/settings"
	"arcmantle/vortex/internal/terminal"
)

// ShellInfo describes a user-spawned interactive shell terminal.
type ShellInfo struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	ProfileID string `json:"profile_id,omitempty"`
	Color     string `json:"color,omitempty"`
	Icon      string `json:"icon,omitempty"`
	PID       int    `json:"pid"`
}

// shellEntry holds a terminal and its associated profile metadata.
type shellEntry struct {
	term      *terminal.Terminal
	profileID string
	color     string
	icon      string
}

// ShellManager manages user-spawned interactive shell terminals that are
// independent of the orchestrator job graph.
type ShellManager struct {
	mu      sync.RWMutex
	shells  map[string]*shellEntry
	order   []string
	counter int
	workDir string
}

// NewShellManager creates a ShellManager that spawns shells in workDir.
func NewShellManager(workDir string) *ShellManager {
	return &ShellManager{
		shells:  make(map[string]*shellEntry),
		workDir: workDir,
	}
}

// Create spawns a new interactive shell terminal using the given profile.
// If profile is nil, it falls back to the system default shell.
func (m *ShellManager) Create(ctx context.Context, profile *settings.ShellProfile) (ShellInfo, error) {
	m.mu.Lock()
	m.counter++
	id := fmt.Sprintf("shell-%d", m.counter)
	m.mu.Unlock()

	var shellCmd string
	var shellArgs []string
	var label, profileID, color, icon string

	if profile != nil {
		shellCmd = profile.Command
		shellArgs = profile.Args
		label = profile.Name
		profileID = profile.ID
		color = profile.Color
		icon = profile.Icon
	} else {
		shellCmd, shellArgs = defaultShell()
		label = fmt.Sprintf("Shell %d", m.counter)
	}

	term, err := terminal.New(ctx, id, label, shellCmd, shellArgs, m.workDir)
	if err != nil {
		return ShellInfo{}, fmt.Errorf("spawn shell: %w", err)
	}

	m.mu.Lock()
	m.shells[id] = &shellEntry{
		term:      term,
		profileID: profileID,
		color:     color,
		icon:      icon,
	}
	m.order = append(m.order, id)
	m.mu.Unlock()

	return ShellInfo{
		ID:        id,
		Label:     label,
		ProfileID: profileID,
		Color:     color,
		Icon:      icon,
		PID:       term.PID(),
	}, nil
}

// Get returns a shell terminal by ID, or nil if not found.
func (m *ShellManager) Get(id string) *terminal.Terminal {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry := m.shells[id]
	if entry == nil {
		return nil
	}
	return entry.term
}

// All returns info for all open shells in creation order.
func (m *ShellManager) All() []ShellInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	infos := make([]ShellInfo, 0, len(m.order))
	for _, id := range m.order {
		entry := m.shells[id]
		if entry == nil {
			continue
		}
		infos = append(infos, ShellInfo{
			ID:        id,
			Label:     entry.term.Label,
			ProfileID: entry.profileID,
			Color:     entry.color,
			Icon:      entry.icon,
			PID:       entry.term.PID(),
		})
	}
	return infos
}

// Close kills a shell terminal and removes it from the manager.
func (m *ShellManager) Close(id string) bool {
	m.mu.Lock()
	entry, ok := m.shells[id]
	if !ok {
		m.mu.Unlock()
		return false
	}
	delete(m.shells, id)
	for i, oid := range m.order {
		if oid == id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	m.mu.Unlock()

	entry.term.Kill()
	return true
}

// CloseAll kills all shell terminals.
func (m *ShellManager) CloseAll() {
	m.mu.Lock()
	entries := make([]*shellEntry, 0, len(m.shells))
	for _, e := range m.shells {
		entries = append(entries, e)
	}
	m.shells = make(map[string]*shellEntry)
	m.order = nil
	m.mu.Unlock()

	for _, e := range entries {
		e.term.Kill()
	}
}

// defaultShell returns the command and args to spawn an interactive shell.
func defaultShell() (string, []string) {
	if runtime.GOOS == "windows" {
		if _, err := os.Stat(`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`); err == nil {
			return `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, []string{"-NoLogo"}
		}
		return "cmd.exe", nil
	}
	// Unix: use $SHELL, fallback to /bin/sh.
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
	}
	return sh, []string{"-l"}
}
