// Package server implements the Vortex HTTP server.
//
// Routes:
//
//	GET  /                      → serves embedded web UI (SPA fallback)
//	GET  /api/terminals          → JSON list of all terminals
//	GET  /api/terminals/{id}     → JSON info + buffered output for a terminal
//	GET  /events?id=<id>         → SSE stream of a terminal's output lines
//	POST /handoff                → single-instance handoff (args forwarding)
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"arcmantle/vortex/internal/instance"
	"arcmantle/vortex/internal/orchestrator"
	"arcmantle/vortex/internal/terminal"
)

// HandoffHandler is called when a second instance forwards its arguments.
type HandoffHandler func(args []string)

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
	onHandoff      HandoffHandler
	devMode        bool
	devServerProxy string // e.g. "http://localhost:5173"
	instance       InstanceInfo
}

// New creates a Server.
//   - orch: job orchestrator
//   - static: embedded FS containing the web UI build output (nil in dev mode)
//   - onHandoff: called when a second instance forwards its args
//   - devMode: when true, /api/* is served but static files are not embedded
//   - devServerURL: Vite dev server URL to proxy static requests to (unused when devMode==false)
func New(appCtx context.Context, orch *orchestrator.Orchestrator, static fs.FS, onHandoff HandoffHandler, devMode bool, devServerURL string, instance InstanceInfo) *Server {
	return &Server{
		appCtx:         appCtx,
		orch:           orch,
		static:         static,
		onHandoff:      onHandoff,
		devMode:        devMode,
		devServerProxy: devServerURL,
		instance:       instance,
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
	mux.HandleFunc("POST /api/terminals/{id}/rerun", s.handleRerunTerminal)
	mux.HandleFunc("DELETE /api/terminals/{id}/buffer", s.handleClearBuffer)
	mux.HandleFunc("POST /handoff", s.handleHandoff)

	if !s.devMode {
		// Serve the embedded SPA with fallback to index.html.
		mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serveEmbedded(w, r, s.static)
		}))
	}

	return mux
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
	needs := j.Spec.Needs
	if needs == nil {
		needs = []string{}
	}
	pid := 0
	if term := j.Terminal(); term != nil {
		pid = term.PID()
	}
	return terminalInfo{
		ID:      j.Spec.ID,
		Label:   j.Spec.Label,
		Command: j.Spec.Command,
		Group:   j.Spec.Group,
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
	Lines []lineDTO `json:"lines"`
}

type lineDTO struct {
	T    int64  `json:"t"` // unix millis
	Text string `json:"text"`
}

func (s *Server) handleGetTerminal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	j, ok := s.orch.GetJob(id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	info := jobToInfo(j)
	var dtos []lineDTO
	if term := j.Terminal(); term != nil {
		lines := term.Lines()
		dtos = make([]lineDTO, len(lines))
		for i, l := range lines {
			dtos[i] = lineDTO{T: l.T.UnixMilli(), Text: l.Text}
		}
	}
	if dtos == nil {
		dtos = []lineDTO{}
	}
	writeJSON(w, terminalDetail{terminalInfo: info, Lines: dtos})
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
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	firstIteration := true

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

		// Stream output. On the first iteration replay the full buffer;
		// on restarts the client already has the history so only subscribe
		// to new lines.
		var lines []terminal.Line
		var ch <-chan terminal.Line
		var cancel func()
		if firstIteration {
			lines, ch, cancel = term.LinesAndSubscribe()
			for _, l := range lines {
				writeSSELine(w, l.Text, l.T)
			}
			flusher.Flush()
			firstIteration = false
		} else {
			ch, cancel = term.Subscribe()
		}

		streaming := true
		for streaming {
			select {
			case <-ctx.Done():
				cancel()
				return
			case <-s.orch.Restarted():
				// Job graph replaced while streaming — send exit, loop.
				cancel()
				fmt.Fprintf(w, "event: exit\ndata: done\n\n")
				flusher.Flush()
				streaming = false
			case line, open := <-ch:
				if !open {
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
					writeSSELine(w, line.Text, line.T)
					flusher.Flush()
				}
			}
		}
	}
}

func writeSSELine(w http.ResponseWriter, text string, t time.Time) {
	data, _ := json.Marshal(lineDTO{T: t.UnixMilli(), Text: text})
	fmt.Fprintf(w, "data: %s\n\n", data)
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
				writeJSON(w, struct {
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

func (s *Server) handleHandoff(w http.ResponseWriter, r *http.Request) {
	var payload instance.HandoffPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if s.onHandoff != nil {
		go s.onHandoff(payload.Args)
	}
	w.WriteHeader(http.StatusOK)
}

// --- static SPA helper ---

func serveEmbedded(w http.ResponseWriter, r *http.Request, fsys fs.FS) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}

	f, err := fsys.Open(path)
	if err != nil {
		// SPA fallback: serve index.html for unknown paths.
		f, err = fsys.Open("index.html")
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		path = "index.html"
	}
	f.Close()

	http.ServeFileFS(w, r, fsys, path)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// ListenAndServe starts the HTTP server on addr and blocks until ctx is done.
func ListenAndServe(ctx context.Context, addr string, h http.Handler) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: h,
	}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutCtx) //nolint:errcheck
	}()
	err := srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
