// Package server implements the Vortex HTTP server.
//
// Routes:
//
//	GET  /                      → serves embedded web UI (SPA fallback)
//	GET  /api/terminals          → JSON list of all terminals
//	GET  /api/terminals/{id}     → JSON info + buffered output for a terminal
//	GET  /events?id=<id>         → SSE stream of a terminal's output chunks
//	POST /handoff                → single-instance handoff (args forwarding)
package server

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"arcmantle/vortex/internal/instance"
	"arcmantle/vortex/internal/orchestrator"
	"arcmantle/vortex/internal/terminal"
	"arcmantle/vortex/internal/webview"
)

// InstanceInfo describes the running Vortex instance served by this process.
type InstanceInfo struct {
	Name         string `json:"name"`
	RegistryName string `json:"-"`
	HTTPPort     int    `json:"http_port"`
}

// Server is the Vortex HTTP server.
type Server struct {
	appCtx         context.Context
	orch           *orchestrator.Orchestrator
	static         fs.FS
	devMode        bool
	devServerProxy string // e.g. "http://localhost:5173"
	instance       InstanceInfo
	token          string // session token for API authentication
}

// New creates a Server.
//   - orch: job orchestrator
//   - static: embedded FS containing the web UI build output (nil in dev mode)
//   - devMode: when true, /api/* is served but static files are not embedded
//   - devServerURL: Vite dev server URL to proxy static requests to (unused when devMode==false)
//   - token: session token for API auth (empty string disables auth)
func New(appCtx context.Context, orch *orchestrator.Orchestrator, static fs.FS, devMode bool, devServerURL string, instance InstanceInfo, token string) *Server {
	return &Server{
		appCtx:         appCtx,
		orch:           orch,
		static:         static,
		devMode:        devMode,
		devServerProxy: devServerURL,
		instance:       instance,
		token:          token,
	}
}

// Handler returns the root http.Handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/terminals", s.handleListTerminals)
	mux.HandleFunc("GET /api/terminals/{id}", s.handleGetTerminal)
	mux.HandleFunc("GET /events", s.handleEvents)
	mux.HandleFunc("DELETE /api/processes", s.handleKillProcesses)
	mux.HandleFunc("DELETE /api/terminals/{id}", s.handleKillTerminal)
	mux.HandleFunc("POST /api/terminals/{id}/input", s.handleInputTerminal)
	mux.HandleFunc("POST /api/terminals/{id}/rerun", s.handleRerunTerminal)
	mux.HandleFunc("POST /api/terminals/{id}/size", s.handleResizeTerminal)
	mux.HandleFunc("DELETE /api/terminals/{id}/buffer", s.handleClearBuffer)
	mux.HandleFunc("POST /api/open-path", s.handleOpenPath)

	if !s.devMode {
		// Serve the embedded SPA with fallback to index.html.
		mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serveEmbedded(w, r, s.static)
		}))
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Dev mode: add CORS headers for the Vite dev server.
		if s.devMode && s.devServerProxy != "" {
			w.Header().Set("Access-Control-Allow-Origin", s.devServerProxy)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		// Token auth: skip for static file serving and dev mode.
		isAPI := strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/events"
		if !s.devMode && s.token != "" && isAPI {
			token := r.URL.Query().Get("token")
			if token == "" {
				if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
					token = strings.TrimPrefix(auth, "Bearer ")
				}
			}
			if subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) != 1 {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}

		mux.ServeHTTP(w, r)
	})
}

// --- API handlers ---

type terminalInfo struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	Command string   `json:"command"`
	Group   string   `json:"group"`
	Needs   []string `json:"needs"`
	Status  string   `json:"status"` // pending|running|success|failure|skipped
	PID     int      `json:"pid"`
}

func jobToInfo(j *orchestrator.Job) terminalInfo {
	spec := j.Spec()
	needs := spec.Needs
	if needs == nil {
		needs = []string{}
	}
	pid := 0
	if term := j.Terminal(); term != nil {
		pid = term.PID()
	}
	return terminalInfo{
		ID:      spec.ID,
		Label:   spec.Label,
		Command: spec.DisplayCommand(),
		Group:   spec.Group,
		Needs:   needs,
		Status:  string(j.Status()),
		PID:     pid,
	}
}

func (s *Server) handleListTerminals(w http.ResponseWriter, r *http.Request) {
	jobs := s.orch.AllJobs()
	infos := make([]terminalInfo, len(jobs))
	for i, j := range jobs {
		infos[i] = jobToInfo(j)
	}
	writeJSON(w, struct {
		Instance  InstanceInfo   `json:"instance"`
		Gen       int            `json:"gen"`
		Terminals []terminalInfo `json:"terminals"`
	}{
		Instance:  s.instance,
		Gen:       s.orch.Generation(),
		Terminals: infos,
	})
}

type terminalDetail struct {
	terminalInfo
	Chunks []outputDTO `json:"chunks"`
}

type outputDTO struct {
	T    int64  `json:"t"` // unix millis
	Data string `json:"data"`
}

func (s *Server) handleGetTerminal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	j, ok := s.orch.GetJob(id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	info := jobToInfo(j)
	var dtos []outputDTO
	if term := j.Terminal(); term != nil {
		chunks := term.Output()
		dtos = make([]outputDTO, len(chunks))
		for i, chunk := range chunks {
			dtos[i] = outputDTO{T: chunk.T.UnixMilli(), Data: base64.StdEncoding.EncodeToString(chunk.Data)}
		}
	}
	if dtos == nil {
		dtos = []outputDTO{}
	}
	writeJSON(w, terminalDetail{terminalInfo: info, Chunks: dtos})
}

// handleEvents streams SSE output for a job identified by ?id=<id>.
//
// The connection stays open for the lifetime of the terminal panel, not the
// process. When a process exits an "exit" event is sent but the stream
// continues. When the orchestrator restarts, the handler picks up the new job
// and streams its output on the same connection.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id query param", http.StatusBadRequest)
		return
	}
	if _, ok := s.orch.GetJob(id); !ok {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	for {
		// Fetch the (possibly new) job for this ID.
		j, ok := s.orch.GetJob(id)
		if !ok {
			// Job was removed entirely — end stream.
			return
		}

		// Wait for the job to start, or for a restart / client disconnect.
		select {
		case <-j.Started():
		case <-s.orch.Restarted():
			// Job graph was replaced — loop to pick up the new job.
			continue
		case <-ctx.Done():
			return
		}

		term := j.Terminal()
		if term == nil {
			// Reached terminal state without starting (skipped / failed to launch).
			status := string(j.Status())
			fmt.Fprintf(w, "event: %s\ndata: done\n\n", status)
			flusher.Flush()
			// Wait for restart or disconnect before looping.
			select {
			case <-s.orch.Restarted():
				continue
			case <-ctx.Done():
				return
			}
		}

		// Stream output. On the first iteration replay the full buffer so
		// the client has history. On restarts the new terminal buffer is
		// empty so only fresh output is sent — old output stays on screen.
		var chunks []terminal.OutputChunk
		var ch <-chan terminal.OutputChunk
		var cancel func()
		chunks, ch, cancel = term.OutputAndSubscribe()
		for _, chunk := range chunks {
			writeSSEChunk(w, chunk)
		}
		flusher.Flush()

		keepalive := time.NewTicker(15 * time.Second)
		streaming := true
		for streaming {
			select {
			case <-ctx.Done():
				keepalive.Stop()
				cancel()
				return
			case <-keepalive.C:
				fmt.Fprintf(w, ": keepalive\n\n")
				flusher.Flush()
			case <-s.orch.Restarted():
				// Job graph replaced while streaming — send exit, loop.
				keepalive.Stop()
				cancel()
				fmt.Fprintf(w, "event: exit\ndata: done\n\n")
				flusher.Flush()
				streaming = false
			case line, open := <-ch:
				if !open {
					keepalive.Stop()
					cancel()
					fmt.Fprintf(w, "event: exit\ndata: done\n\n")
					flusher.Flush()
					// Process exited naturally — wait for restart or disconnect.
					select {
					case <-s.orch.Restarted():
					case <-ctx.Done():
						return
					}
					streaming = false
				} else {
					writeSSEChunk(w, line)
					flusher.Flush()
				}
			}
		}
	}
}

func writeSSEChunk(w http.ResponseWriter, chunk terminal.OutputChunk) {
	data, err := json.Marshal(outputDTO{T: chunk.T.UnixMilli(), Data: base64.StdEncoding.EncodeToString(chunk.Data)})
	if err != nil {
		log.Printf("[server] failed to marshal SSE chunk: %v", err)
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
}

type resizeTerminalRequest struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

type terminalInputRequest struct {
	Data string `json:"data"`
}

type openPathRequest struct {
	Path string `json:"path"`
}

type openPathTarget struct {
	Path   string
	Line   int
	Column int
}

func (s *Server) handleKillProcesses(w http.ResponseWriter, r *http.Request) {
	killed := 0
	for _, job := range s.orch.AllJobs() {
		term := job.Terminal()
		if term == nil || term.PID() == 0 {
			continue
		}
		term.Kill()
		killed++
	}
	if killed > 0 && s.instance.RegistryName != "" {
		identity, err := instance.NewIdentity(s.instance.RegistryName)
		if err == nil {
			if err := instance.MarkControlAction(identity); err != nil {
				writeJSONStatus(w, http.StatusInternalServerError, struct {
					Killed int    `json:"killed"`
					Error  string `json:"error,omitempty"`
				}{Killed: killed, Error: err.Error()})
				return
			}
		}
	}
	writeJSON(w, struct {
		Killed int `json:"killed"`
	}{Killed: killed})
}

func (s *Server) handleKillTerminal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	j, ok := s.orch.GetJob(id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if term := j.Terminal(); term != nil {
		term.Kill()
		if s.instance.RegistryName != "" {
			identity, err := instance.NewIdentity(s.instance.RegistryName)
			if err == nil {
				if err := instance.MarkControlAction(identity); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRerunTerminal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.orch.GetJob(id); !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err := s.orch.Rerun(s.appCtx, id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if s.instance.RegistryName != "" {
		identity, err := instance.NewIdentity(s.instance.RegistryName)
		if err == nil {
			if err := instance.MarkControlAction(identity); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleInputTerminal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	j, ok := s.orch.GetJob(id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	term := j.Terminal()
	if term == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var req terminalInputRequest
	if err := decodeJSON(w, r, &req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	data, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := term.WriteInput(data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleResizeTerminal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	j, ok := s.orch.GetJob(id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	term := j.Terminal()
	if term == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var req resizeTerminalRequest
	if err := decodeJSON(w, r, &req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := term.Resize(req.Cols, req.Rows); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleClearBuffer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	j, ok := s.orch.GetJob(id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if term := j.Terminal(); term != nil {
		term.ClearBuffer()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleOpenPath(w http.ResponseWriter, r *http.Request) {
	var req openPathRequest
	if err := decodeJSON(w, r, &req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	target, err := resolveOpenPathTarget(s.orch.WorkDir(), req.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := webview.OpenPathInEditor(webview.OpenFileTarget{Path: target.Path, Line: target.Line, Column: target.Column}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- static SPA helper ---

func serveEmbedded(w http.ResponseWriter, r *http.Request, fsys fs.FS) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}

	if _, err := fs.Stat(fsys, path); err != nil {
		// SPA fallback: serve index.html for unknown paths.
		if _, err := fs.Stat(fsys, "index.html"); err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		path = "index.html"
	}

	http.ServeFileFS(w, r, fsys, path)
}

const maxRequestBodyBytes = 1 << 20 // 1 MiB

// decodeJSON reads at most maxRequestBodyBytes from r.Body and JSON-decodes
// it into v. Callers should return 400 if decodeJSON returns an error.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	return json.NewDecoder(r.Body).Decode(v)
}

// writeJSON marshals v to JSON and writes it as the full response body.
// Marshaling happens before any bytes are sent so that errors can still
// produce a proper HTTP 500 instead of corrupting a partially-written response.
func writeJSON(w http.ResponseWriter, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
	_, _ = w.Write([]byte("\n"))
}

// writeJSONStatus is like writeJSON but sets an explicit HTTP status code.
func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
	_, _ = w.Write([]byte("\n"))
}

func resolveOpenPathTarget(workDir, rawPath string) (openPathTarget, error) {
	target := parseTerminalPath(rawPath)
	if target.Path == "" {
		return openPathTarget{}, fmt.Errorf("path must not be empty")
	}
	path := target.Path

	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return openPathTarget{}, fmt.Errorf("resolve home directory: %w", err)
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, path[2:])
		}
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(workDir, path)
	}

	path = filepath.Clean(path)

	// Resolve symlinks so that a symlink inside an allowed directory
	// pointing outside cannot bypass the traversal check.
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}

	// Guard against path traversal: resolved path must be within the working
	// directory or the user's home directory. This prevents the open-path
	// API from being used to open arbitrary files on the system.
	home, _ := os.UserHomeDir()
	allowed := false
	resolvedWorkDir := workDir
	if resolvedWorkDir != "" {
		if rwd, err := filepath.EvalSymlinks(resolvedWorkDir); err == nil {
			resolvedWorkDir = rwd
		}
	}
	resolvedHome := home
	if resolvedHome != "" {
		if rh, err := filepath.EvalSymlinks(resolvedHome); err == nil {
			resolvedHome = rh
		}
	}
	if resolvedWorkDir != "" && (path == resolvedWorkDir || strings.HasPrefix(path, resolvedWorkDir+string(filepath.Separator))) {
		allowed = true
	}
	if resolvedHome != "" && (path == resolvedHome || strings.HasPrefix(path, resolvedHome+string(filepath.Separator))) {
		allowed = true
	}
	if !allowed {
		return openPathTarget{}, fmt.Errorf("path %q is outside the allowed directories", path)
	}

	target.Path = path
	return target, nil
}

func parseTerminalPath(raw string) openPathTarget {
	path := strings.TrimSpace(raw)
	path = strings.Trim(path, "\"'`()[]{}<>,")
	path = strings.TrimPrefix(path, "file://")

	line := 0
	column := 0
	trimmed, maybeColumn := trimTrailingNumber(path)
	if maybeColumn > 0 {
		prev, maybeLine := trimTrailingNumber(trimmed)
		if maybeLine > 0 {
			path = prev
			line = maybeLine
			column = maybeColumn
		} else {
			path = trimmed
			line = maybeColumn
		}
	}

	return openPathTarget{Path: path, Line: line, Column: column}
}

func trimTrailingNumber(path string) (string, int) {
	last := strings.LastIndex(path, ":")
	if last < 0 {
		return path, 0
	}
	segment := path[last+1:]
	if !isDigits(segment) {
		return path, 0
	}
	prefix := path[:last]
	if len(prefix) == 1 && path[1] == ':' {
		return path, 0
	}
	value, err := strconv.Atoi(segment)
	if err != nil || value < 0 {
		return path, 0
	}
	return prefix, value
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func resolveRevealPath(workDir, rawPath string) (string, error) {
	target, err := resolveOpenPathTarget(workDir, rawPath)
	if err != nil {
		return "", err
	}
	return target.Path, nil
}

func normalizeTerminalPath(raw string) string {
	return parseTerminalPath(raw).Path
}

func stripLineColumnSuffix(path string) string {
	trimmed := path
	last := strings.LastIndex(trimmed, ":")
	if last < 0 {
		return trimmed
	}
	if !isDigits(trimmed[last+1:]) {
		return trimmed
	}
	trimmed = trimmed[:last]
	last = strings.LastIndex(trimmed, ":")
	if last >= 0 && isDigits(trimmed[last+1:]) {
		trimmed = trimmed[:last]
	}
	if len(trimmed) == 2 && trimmed[1] == ':' {
		return path
	}
	return trimmed
}

// ListenAndServe starts the HTTP server on addr and blocks until ctx is done.
func ListenAndServe(ctx context.Context, addr string, h http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0, // SSE streams are long-lived; no per-response write deadline
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutCtx)
	}()
	err := srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
