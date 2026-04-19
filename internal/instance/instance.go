// Package instance implements named Vortex instance locking and handoff.
package instance

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"arcmantle/vortex/internal/terminal"
)

const (
	handoffPortBase = 20000
	apiPortBase     = 30000
	portSpan        = 10000
)

// registryMu serializes all registry read-modify-write operations within a
// process, preventing lost updates when multiple goroutines call SetUIState,
// Touch, MarkControlAction, or SetManagedPIDs concurrently.
var registryMu sync.Mutex

const (
	handoffActionRestart = "restart"
	handoffActionQuit    = "quit"
	handoffActionShowUI  = "show-ui"
	handoffActionHideUI  = "hide-ui"
	handoffActionRerun   = "rerun"
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
	Token         string `json:"token,omitempty"`
	PID           int    `json:"pid"`
	StartedAt     int64  `json:"started_at"`
	UpdatedAt     int64  `json:"updated_at"`
	LastControlAt int64  `json:"last_control_at,omitempty"`
	ChildPIDs     []int  `json:"child_pids,omitempty"`
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
	identity := Identity{
		Name:        normalized,
		DisplayName: displayName,
		HandoffPort: handoffPortBase + offset,
		HTTPPort:    apiPortBase + offset,
	}

	return identity, nil
}

// HandoffPayload is the JSON body sent to an already-running instance.
type HandoffPayload struct {
	Action     string   `json:"action,omitempty"`
	Name       string   `json:"name"`
	Args       []string `json:"args"`
	ConfigFile string   `json:"config_file,omitempty"`
	Token      string   `json:"token,omitempty"`
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
	token := readTokenForInstance(identity.Name)
	payload, err := json.Marshal(HandoffPayload{
		Action:     handoffActionRestart,
		Name:       identity.Name,
		ConfigFile: configFile,
		Args:       args,
		Token:      token,
	})
	if err != nil {
		return err
	}

	return postHandoff(identity, payload)
}

// Quit asks the running instance to shut down.
func Quit(identity Identity) error {
	token := readTokenForInstance(identity.Name)
	payload, err := json.Marshal(HandoffPayload{Action: handoffActionQuit, Name: identity.Name, Token: token})
	if err != nil {
		return err
	}

	return postHandoff(identity, payload)
}

// ShowUI asks the running instance to surface its native UI window.
func ShowUI(identity Identity) error {
	token := readTokenForInstance(identity.Name)
	payload, err := json.Marshal(HandoffPayload{Action: handoffActionShowUI, Name: identity.Name, Token: token})
	if err != nil {
		return err
	}

	return postHandoff(identity, payload)
}

// HideUI asks the running instance to dismiss its native UI window without stopping jobs.
func HideUI(identity Identity) error {
	token := readTokenForInstance(identity.Name)
	payload, err := json.Marshal(HandoffPayload{Action: handoffActionHideUI, Name: identity.Name, Token: token})
	if err != nil {
		return err
	}

	return postHandoff(identity, payload)
}

// Rerun asks the running instance to rerun a specific job and its downstream dependents.
func Rerun(identity Identity, jobID string) error {
	token := readTokenForInstance(identity.Name)
	payload, err := json.Marshal(HandoffPayload{Action: handoffActionRerun, Name: identity.Name, Args: []string{jobID}, Token: token})
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
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return fmt.Errorf("port collision: port %d is in use by a different Vortex instance (name mismatch)", identity.HandoffPort)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("existing instance %q returned %s", identity.DisplayName, resp.Status)
	}
	return nil
}

// ServeHandoff starts an HTTP server on the instance-lock listener that accepts
// POST /handoff. This makes the lock port double as the handoff channel.
// When token is non-empty, payloads must include a matching token field.
func ServeHandoff(l net.Listener, identity Identity, token string, handler func(HandoffPayload)) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /handoff", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
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
		if token != "" && subtle.ConstantTimeCompare([]byte(payload.Token), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if handler != nil {
			go handler(payload)
		}
		w.WriteHeader(http.StatusOK)
	})
	go func() {
		if err := http.Serve(l, mux); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Printf("[instance] handoff server stopped: %v", err)
		}
	}()
}

// Register records a running instance in the local registry and returns
// the session token and a cleanup function. The token is used for HTTP API
// and handoff authentication.
func Register(identity Identity, httpPort int, devMode, headless bool, uiState string) (string, func(), error) {
	dir, err := registryDir()
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", nil, fmt.Errorf("create instance registry: %w", err)
	}

	// Clean up any leftover tmp files from previous crashed runs.
	cleanupStaleTempFiles(dir, identity.Name)

	token := generateSessionToken()
	meta := Metadata{
		Name:        identity.Name,
		DisplayName: identity.DisplayName,
		HandoffPort: identity.HandoffPort,
		HTTPPort:    httpPort,
		DevMode:     devMode,
		Headless:    headless,
		UIState:     uiState,
		Token:       token,
		PID:         os.Getpid(),
		StartedAt:   time.Now().UnixMilli(),
		UpdatedAt:   time.Now().UnixMilli(),
	}
	if err := writeMetadataFile(dir, meta); err != nil {
		return "", nil, err
	}

	// Warn about potential port collisions with other registered instances.
	if entries, err := ListMetadata(); err == nil {
		for _, m := range entries {
			if m.Name == identity.Name {
				continue
			}
			if m.HandoffPort == identity.HandoffPort || m.HTTPPort == identity.HTTPPort {
				log.Printf("[instance] warning: instance %q port hash collides with existing instance %q (handoff=%d, http=%d); consider renaming one",
					identity.DisplayName, m.DisplayName, identity.HandoffPort, identity.HTTPPort)
			}
		}
	}

	cleanup := func() {
		_ = os.Remove(registryPath(dir, identity.Name))
	}
	return token, cleanup, nil
}

// SetUIState updates the live UI visibility state for a running instance.
func SetUIState(identity Identity, state string) error {
	registryMu.Lock()
	defer registryMu.Unlock()
	meta, err := GetMetadataLocked(identity.Name)
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
	registryMu.Lock()
	defer registryMu.Unlock()
	meta, err := GetMetadataLocked(identity.Name)
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
	registryMu.Lock()
	defer registryMu.Unlock()
	meta, err := GetMetadataLocked(identity.Name)
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

// SetManagedPIDs records the currently running child process IDs for an instance.
func SetManagedPIDs(identity Identity, pids []int) error {
	registryMu.Lock()
	defer registryMu.Unlock()
	meta, err := GetMetadataLocked(identity.Name)
	if err != nil {
		return err
	}
	meta.ChildPIDs = normalizePIDs(pids)
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
		meta.Token = ""
		instances = append(instances, meta)
	}
	return instances, nil
}

// GetMetadata returns registry metadata for a single named instance.
func GetMetadata(name string) (Metadata, error) {
	registryMu.Lock()
	defer registryMu.Unlock()
	return GetMetadataLocked(name)
}

// GetMetadataLocked is the internal implementation of GetMetadata.
// It does NOT acquire registryMu; callers that need serialization must hold it.
func GetMetadataLocked(name string) (Metadata, error) {
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

// CleanupInactiveMetadata removes a stale registry entry after best-effort termination
// of the recorded controller and child processes.
//
// To guard against PID reuse, each candidate PID is only killed if the OS
// reports that the process was created no later than the metadata's StartedAt
// timestamp (with a small tolerance). When the creation time cannot be
// determined the kill is skipped to avoid terminating an unrelated process.
func CleanupInactiveMetadata(meta Metadata) error {
	for _, pid := range normalizePIDs(append([]int{meta.PID}, meta.ChildPIDs...)) {
		if !isStaleProcess(pid, meta.StartedAt) {
			continue
		}
		if err := terminal.KillProcessTreeByPID(pid); err != nil {
			log.Printf("[instance] CleanupInactiveMetadata %q: failed to kill PID %d: %v", meta.Name, pid, err)
			continue
		}
	}
	return RemoveMetadata(meta.Name)
}

func normalizePIDs(pids []int) []int {
	if len(pids) == 0 {
		return nil
	}
	unique := make(map[int]struct{}, len(pids))
	filtered := make([]int, 0, len(pids))
	for _, pid := range pids {
		if pid <= 0 {
			continue
		}
		if _, seen := unique[pid]; seen {
			continue
		}
		unique[pid] = struct{}{}
		filtered = append(filtered, pid)
	}
	if len(filtered) == 0 {
		return nil
	}
	sort.Ints(filtered)
	return filtered
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

// writeMetadataFile atomically writes instance metadata via tmp+rename.
//
// Cross-process safety: the TCP-based TryLock ensures only one vortex process
// owns a given instance name at a time, so only the owning process modifies
// its metadata file. Other processes only read metadata or delete stale entries.
// This makes the in-process registryMu sufficient without file-level locking.
func writeMetadataFile(dir string, meta Metadata) error {
	path := registryPath(dir, meta.Name)
	tmpPath := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal instance metadata: %w", err)
	}
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("write instance metadata: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write instance metadata: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("sync instance metadata: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close instance metadata: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("publish instance metadata: %w", err)
	}
	return nil
}

// cleanupStaleTempFiles removes leftover *.tmp registry files that may have
// been left behind when a previous process crashed between WriteFile and Rename.
func cleanupStaleTempFiles(dir, name string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	// Temp files are written as "<name>.json.<pid>.tmp"
	prefix := name + ".json."
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		n := entry.Name()
		if strings.HasPrefix(n, prefix) && strings.HasSuffix(n, ".tmp") {
			_ = os.Remove(filepath.Join(dir, n))
		}
	}
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

// generateSessionToken creates a cryptographically random hex token for
// authenticating HTTP API and handoff requests to this instance.
func generateSessionToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure is catastrophic — do not fall back to a
		// predictable token.  Panic so the issue is immediately visible.
		panic(fmt.Sprintf("crypto/rand.Read failed: %v", err))
	}
	return hex.EncodeToString(b)
}

// readTokenForInstance reads the session token from the registry metadata
// for the named instance. Returns empty string if the metadata cannot be read.
func readTokenForInstance(name string) string {
	meta, err := GetMetadata(name)
	if err != nil {
		return ""
	}
	return meta.Token
}
