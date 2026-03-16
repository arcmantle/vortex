// Vortex — start processes and stream their output to a webview UI.
//
// Usage (YAML config):
//
//	vortex [--dev] [--port <n>] config.yaml
//
// Usage (inline, no config file):
//
//	vortex [--dev] [--port <n>] -- <label:command> [<label:command> ...]
//
// The YAML config file defines jobs with optional groups and dependency
// conditions. See internal/config/config.go for the full schema.
//
// Inline specs use the form:
//
//	<label>:<command> [args...]
//
// All inline jobs run in parallel with no dependencies.
//
// Flags:
//
//	--dev   Do not open the webview. The Go server is still started so the
//	        Vite dev server can proxy /api/* and /events to it.
//	--port  HTTP port for the Go server (default 7370).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"arcmantle/vortex/internal/config"
	"arcmantle/vortex/internal/instance"
	"arcmantle/vortex/internal/orchestrator"
	"arcmantle/vortex/internal/server"
	"arcmantle/vortex/internal/webview"
)

// Version info, set via ldflags at build time.
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	dev := flag.Bool("dev", false, "dev mode: skip webview, keep API server running")
	port := flag.Int("port", 7370, "HTTP port for the Go server")
	configFile := flag.String("config", "", "path to YAML config file")
	forked := flag.Bool("forked", false, "internal: already running in a detached session")
	flag.Parse()

	args := flag.Args()

	// --- Single-instance check ---
	l, first, err := instance.TryLock()
	if err != nil {
		log.Fatalf("instance lock error: %v", err)
	}
	if !first {
		// Another instance is running. Forward our args and exit.
		if err := instance.Forward(*configFile, args); err != nil {
			log.Fatalf("handoff failed: %v", err)
		}
		fmt.Println("Forwarded to existing instance.")
		os.Exit(0)
	}

	// On macOS/Linux, fork into a new session so the terminal is freed.
	// On Windows this is a no-op (handled by -H=windowsgui at build time).
	if !*dev && !*forked {
		if maybeFork() {
			l.Close()
			return
		}
	}
	defer l.Close()

	// --- Load config ---
	cfg, err := loadConfig(*configFile, args)
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	// --- Build orchestrator ---
	orch, err := orchestrator.New(cfg)
	if err != nil {
		log.Fatalf("orchestrator error: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	onHandoff := func(configFile string, handoffArgs []string) {
		log.Printf("Second instance connected with args: %v", handoffArgs)
		handoffCfg, err := loadConfig(configFile, handoffArgs)
		if err != nil {
			log.Printf("handoff config error: %v", err)
			return
		}
		orch.Restart(ctx, handoffCfg)
	}

	// Serve the handoff endpoint on the instance-lock listener (port 7371)
	// so that a second instance can POST to it. Without this, the raw TCP
	// listener accepts connections but never speaks HTTP, causing timeouts.
	instance.ServeHandoff(l, onHandoff)

	srv := server.New(orch, uiFS(), nil, *dev, "http://localhost:5173")
	addr := fmt.Sprintf("127.0.0.1:%d", *port)

	// Start all jobs according to their dependency graph.
	orch.Start(ctx)

	// Start HTTP server in background.
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Vortex server listening on http://%s", addr)
		serverErr <- server.ListenAndServe(ctx, addr, srv.Handler())
	}()

	if !*dev {
		// Open webview in background; when the window is closed, cancel ctx.
		go func() {
			webview.Open("Vortex", fmt.Sprintf("http://%s", addr), 1280, 800)
			stop()
		}()
	} else {
		log.Printf("Dev mode: open http://%s in your browser (or use the Vite dev server)", addr)
	}

	select {
	case <-ctx.Done():
	case err := <-serverErr:
		if err != nil {
			log.Printf("server error: %v", err)
		}
	}

	orch.Shutdown()
}

// loadConfig returns a Config from either a YAML config file (via --config
// flag or first positional arg ending in .yaml/.yml) or a slice of inline
// "label:command" specs.
func loadConfig(configPath string, args []string) (*config.Config, error) {
	// Explicit --config flag takes priority.
	if configPath != "" {
		return config.Load(configPath)
	}
	// Fall back: check if first positional arg is a YAML file.
	if len(args) > 0 {
		ext := strings.ToLower(filepath.Ext(args[0]))
		if ext == ".yaml" || ext == ".yml" {
			return config.Load(args[0])
		}
	}
	// Fall back to inline label:command specs — all run in parallel.
	specs := parseSpecs(args)
	if len(specs) == 0 {
		return &config.Config{}, nil
	}
	cfg := &config.Config{}
	for _, s := range specs {
		fullCmd := s.command
		if len(s.args) > 0 {
			fullCmd += " " + strings.Join(s.args, " ")
		}
		cfg.Jobs = append(cfg.Jobs, config.JobSpec{
			ID:      s.id,
			Label:   s.label,
			Command: fullCmd,
		})
	}
	return cfg, nil
}

// processSpec describes a process to launch (used for inline args only).
type processSpec struct {
	id      string
	label   string
	command string
	args    []string
}

// parseSpecs parses arguments of the form "label:command [args...]".
func parseSpecs(rawArgs []string) []processSpec {
	var specs []processSpec
	for i, raw := range rawArgs {
		idx := strings.IndexByte(raw, ':')
		var label, rest string
		if idx < 0 {
			label = fmt.Sprintf("proc%d", i)
			rest = raw
		} else {
			label = raw[:idx]
			rest = raw[idx+1:]
		}
		parts := strings.Fields(rest)
		if len(parts) == 0 {
			continue
		}
		specs = append(specs, processSpec{
			id:      fmt.Sprintf("%s-%d", label, i),
			label:   label,
			command: parts[0],
			args:    parts[1:],
		})
	}
	return specs
}

