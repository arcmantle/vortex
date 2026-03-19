// Package instance implements named Vortex instance locking and handoff.
package instance

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	handoffPortBase = 20000
	apiPortBase     = 30000
	portSpan        = 10000
)

const (
	handoffActionRestart = "restart"
	handoffActionQuit    = "quit"
	handoffActionShowUI  = "show-ui"
	handoffActionHideUI  = "hide-ui"
)

// Identity describes a named Vortex instance and its deterministic ports.
type Identity struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	HandoffPort int    `json:"handoff_port"`
	HTTPPort    int    `json:"http_port"`
}

// Metadata describes a running Vortex instance persisted in the local registry.
type Metadata struct {
	Name          string `json:"name"`
	DisplayName   string `json:"display_name"`
	HandoffPort   int    `json:"handoff_port"`
	HTTPPort      int    `json:"http_port"`
	DevMode       bool   `json:"dev_mode"`
	Headless      bool   `json:"headless"`
	UIState       string `json:"ui_state"`
	PID           int    `json:"pid"`
	StartedAt     int64  `json:"started_at"`
	UpdatedAt     int64  `json:"updated_at"`
	LastControlAt int64  `json:"last_control_at,omitempty"`
}

// NewIdentity normalizes a Vortex instance name and derives deterministic ports.
func NewIdentity(name string) (Identity, error) {
	displayName := strings.TrimSpace(name)
	if displayName == "" {
		return Identity{}, fmt.Errorf("instance name must not be empty")
	}

	normalized := strings.ToLower(displayName)
	for i, r := range normalized {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return Identity{}, fmt.Errorf("instance name %q contains unsupported character %q at position %d", displayName, r, i+1)
	}

	offset := hashOffset(normalized)
	return Identity{
		Name:        normalized,
		DisplayName: displayName,
		HandoffPort: handoffPortBase + offset,
		HTTPPort:    apiPortBase + offset,
	}, nil
}

// HandoffPayload is the JSON body sent to an already-running instance.
type HandoffPayload struct {
	Action     string   `json:"action,omitempty"`
	Name       string   `json:"name"`
	Args       []string `json:"args"`
	ConfigFile string   `json:"config_file,omitempty"`
}

// TryLock attempts to bind the handoff port. Returns (listener, true, nil) if
// this process won the lock, or (nil, false, nil) if another instance is
// already running.
func TryLock(identity Identity) (net.Listener, bool, error) {
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", identity.HandoffPort))
	if err != nil {
		if isAddrInUse(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return l, true, nil
}

// Forward sends args to the running instance and waits for an acknowledgement.
func Forward(identity Identity, configFile string, args []string) error {
	payload, err := json.Marshal(HandoffPayload{
		Action:     handoffActionRestart,
		Name:       identity.Name,
		ConfigFile: configFile,
		Args:       args,
	})
	if err != nil {
		return err
	}

	return postHandoff(identity, payload)
}

// Quit asks the running instance to shut down.
func Quit(identity Identity) error {
	payload, err := json.Marshal(HandoffPayload{Action: handoffActionQuit, Name: identity.Name})
	if err != nil {
		return err
	}

	return postHandoff(identity, payload)
}

// ShowUI asks the running instance to surface its native UI window.
func ShowUI(identity Identity) error {
	payload, err := json.Marshal(HandoffPayload{Action: handoffActionShowUI, Name: identity.Name})
	if err != nil {
		return err
	}

	return postHandoff(identity, payload)
}

// HideUI asks the running instance to dismiss its native UI window without stopping jobs.
func HideUI(identity Identity) error {
	payload, err := json.Marshal(HandoffPayload{Action: handoffActionHideUI, Name: identity.Name})
	if err != nil {
		return err
	}

	return postHandoff(identity, payload)
}

func postHandoff(identity Identity, payload []byte) error {
	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/handoff", identity.HandoffPort)
	resp, err := client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("could not reach existing instance %q: %w", identity.DisplayName, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("existing instance %q returned %s", identity.DisplayName, resp.Status)
	}
	return nil
}

// ServeHandoff starts an HTTP server on the instance-lock listener that accepts
// POST /handoff. This makes the lock port double as the handoff channel.
func ServeHandoff(l net.Listener, identity Identity, handler func(HandoffPayload)) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /handoff", func(w http.ResponseWriter, r *http.Request) {
		var payload HandoffPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if payload.Name == "" {
			http.Error(w, "missing instance name", http.StatusBadRequest)
			return
		}
		if payload.Name != identity.Name {
			http.Error(w, "instance name mismatch", http.StatusConflict)
			return
		}
		if handler != nil {
			go handler(payload)
		}
		w.WriteHeader(http.StatusOK)
	})
	go http.Serve(l, mux) //nolint:errcheck
}

// Register records a running instance in the local registry and returns a cleanup function.
func Register(identity Identity, httpPort int, devMode, headless bool, uiState string) (func(), error) {
	dir, err := registryDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create instance registry: %w", err)
	}

	meta := Metadata{
		Name:        identity.Name,
		DisplayName: identity.DisplayName,
		HandoffPort: identity.HandoffPort,
		HTTPPort:    httpPort,
		DevMode:     devMode,
		Headless:    headless,
		UIState:     uiState,
		PID:         os.Getpid(),
		StartedAt:   time.Now().UnixMilli(),
		UpdatedAt:   time.Now().UnixMilli(),
	}
	if err := writeMetadataFile(dir, meta); err != nil {
		return nil, err
	}

	cleanup := func() {
		_ = os.Remove(registryPath(dir, identity.Name))
	}
	return cleanup, nil
}

// SetUIState updates the live UI visibility state for a running instance.
func SetUIState(identity Identity, state string) error {
	meta, err := GetMetadata(identity.Name)
	if err != nil {
		return err
	}
	meta.UIState = state
	meta.UpdatedAt = time.Now().UnixMilli()
	dir, err := registryDir()
	if err != nil {
		return err
	}
	if err := writeMetadataFile(dir, meta); err != nil {
		return err
	}
	return nil
}

// Touch updates the instance metadata timestamp without changing any other fields.
func Touch(identity Identity) error {
	meta, err := GetMetadata(identity.Name)
	if err != nil {
		return err
	}
	meta.UpdatedAt = time.Now().UnixMilli()
	dir, err := registryDir()
	if err != nil {
		return err
	}
	if err := writeMetadataFile(dir, meta); err != nil {
		return err
	}
	return nil
}

// MarkControlAction updates the instance metadata timestamp for explicit control actions
// without changing the broader metadata-updated timestamp semantics.
func MarkControlAction(identity Identity) error {
	meta, err := GetMetadata(identity.Name)
	if err != nil {
		return err
	}
	meta.LastControlAt = time.Now().UnixMilli()
	dir, err := registryDir()
	if err != nil {
		return err
	}
	if err := writeMetadataFile(dir, meta); err != nil {
		return err
	}
	return nil
}

// ListMetadata returns all registered Vortex instances.
func ListMetadata() ([]Metadata, error) {
	dir, err := registryDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read instance registry: %w", err)
	}

	instances := make([]Metadata, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var meta Metadata
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		instances = append(instances, meta)
	}
	return instances, nil
}

// GetMetadata returns registry metadata for a single named instance.
func GetMetadata(name string) (Metadata, error) {
	identity, err := NewIdentity(name)
	if err != nil {
		return Metadata{}, err
	}
	dir, err := registryDir()
	if err != nil {
		return Metadata{}, err
	}
	data, err := os.ReadFile(registryPath(dir, identity.Name))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Metadata{}, fmt.Errorf("no running Vortex instance named %q", identity.DisplayName)
		}
		return Metadata{}, fmt.Errorf("read instance metadata: %w", err)
	}
	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return Metadata{}, fmt.Errorf("parse instance metadata: %w", err)
	}
	return meta, nil
}

// RemoveMetadata removes the registry entry for a named instance.
func RemoveMetadata(name string) error {
	identity, err := NewIdentity(name)
	if err != nil {
		return err
	}
	dir, err := registryDir()
	if err != nil {
		return err
	}
	if err := os.Remove(registryPath(dir, identity.Name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove instance metadata: %w", err)
	}
	return nil
}

func hashOffset(name string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return int(h.Sum32() % portSpan)
}

func registryDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "vortex", "instances"), nil
}

func registryPath(dir, name string) string {
	return filepath.Join(dir, name+".json")
}

func writeMetadataFile(dir string, meta Metadata) error {
	path := registryPath(dir, meta.Name)
	tmpPath := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal instance metadata: %w", err)
	}
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write instance metadata: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("publish instance metadata: %w", err)
	}
	return nil
}

func isAddrInUse(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		var errno syscall.Errno
		if errors.As(opErr.Err, &errno) {
			// EADDRINUSE on Unix, WSAEADDRINUSE (10048) on Windows.
			return errno == syscall.EADDRINUSE || errno == 10048
		}
	}
	return false
}
