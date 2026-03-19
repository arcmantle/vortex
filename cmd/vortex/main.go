// Vortex — start processes and stream their output to a webview UI.
//
// Usage (YAML config):
//
//	vortex [--headless] [--port <n>] config.yaml
//	vortex --dev [--port <n>] config.yaml
//
// Usage (instances):
//
//	vortex instances [name]
//
// Usage (instance control):
//
//	vortex <name> --quit
//	vortex <name> --kill
//	vortex <name> show-ui
//	vortex <name> hide-ui
//
// Usage (self-update):
//
//	vortex help
//	vortex version
//	vortex -v
//	vortex upgrade
//
// The YAML config file defines jobs with optional groups and dependency
// conditions. See internal/config/config.go for the full schema.
//
// Flags:
//
//	--headless  Run normally without opening the native window.
//	--dev   Development mode: do not open the webview and use the browser/Vite workflow.
//	--port  HTTP port for the Go server (default 7370).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"arcmantle/vortex/internal/config"
	"arcmantle/vortex/internal/instance"
	"arcmantle/vortex/internal/orchestrator"
	"arcmantle/vortex/internal/server"
	"arcmantle/vortex/internal/upgrade"
	"arcmantle/vortex/internal/webview"
)

// Version info, set via ldflags at build time.
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	rawArgs := os.Args[1:]
	if len(rawArgs) == 0 || isHelpRequest(rawArgs) {
		printHelp(os.Stdout)
		return
	}
	if isVersionRequest(rawArgs) {
		printVersion()
		return
	}
	if len(rawArgs) > 0 && rawArgs[0] == "upgrade" {
		if err := upgrade.Run(rawArgs[1:], upgrade.Options{CurrentVersion: Version}); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(rawArgs) > 0 && rawArgs[0] == "instances" {
		if err := runInstancesCommand(rawArgs[1:]); err != nil {
			log.Fatal(err)
		}
		return
	}

	opts, err := parseCLI(rawArgs)
	if err != nil {
		fatalCLIError(err)
	}

	if opts.quit {
		identity, err := resolveTargetIdentity(opts, "usage: vortex <name> --quit")
		if err != nil {
			log.Fatal(err)
		}

		l, first, err := instance.TryLock(identity)
		if err != nil {
			log.Fatalf("instance lock error: %v", err)
		}
		if first {
			l.Close()
			if meta, metaErr := instance.GetMetadata(identity.Name); metaErr == nil {
				_ = instance.CleanupInactiveMetadata(meta)
			}
			fmt.Printf("No active Vortex instance named %q.\n", identity.DisplayName)
			return
		}
		if err := instance.Quit(identity); err != nil {
			log.Fatalf("shutdown request failed: %v", err)
		}
		fmt.Printf("Requested shutdown of Vortex instance %q.\n", identity.DisplayName)
		return
	}
	if opts.showUI {
		identity, err := resolveTargetIdentity(opts, "usage: vortex <name> show-ui")
		if err != nil {
			log.Fatal(err)
		}
		meta, err := resolveInstanceMetadata(identity)
		if err != nil {
			log.Fatal(err)
		}
		if meta.DevMode {
			log.Fatalf("Vortex instance %q is running in --dev mode and cannot open a native webview", identity.DisplayName)
		}
		if err := instance.ShowUI(identity); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Requested UI for Vortex instance %q.\n", identity.DisplayName)
		return
	}
	if opts.hideUI {
		identity, err := resolveTargetIdentity(opts, "usage: vortex <name> hide-ui")
		if err != nil {
			log.Fatal(err)
		}
		meta, err := resolveInstanceMetadata(identity)
		if err != nil {
			log.Fatal(err)
		}
		if meta.DevMode {
			log.Fatalf("Vortex instance %q is running in --dev mode and has no native webview to hide", identity.DisplayName)
		}
		if err := instance.HideUI(identity); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Requested UI hide for Vortex instance %q.\n", identity.DisplayName)
		return
	}
	if opts.kill {
		identity, err := resolveTargetIdentity(opts, "usage: vortex <name> --kill")
		if err != nil {
			log.Fatal(err)
		}
		result, err := killInstanceProcesses(identity)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Requested termination of %d process(es) for Vortex instance %q.\n", result.Killed, identity.DisplayName)
		return
	}

	cfg, configPath, err := loadConfigFile(opts.configFile, opts.positionals)
	if err != nil {
		fatalCLIError(fmt.Errorf("config error: %w", err))
	}

	identity, err := instance.NewIdentity(cfg.Name)
	if err != nil {
		log.Fatalf("config error: invalid name %q: %v", cfg.Name, err)
	}

	// --- Named-instance check ---
	l, first, err := instance.TryLock(identity)
	if err != nil {
		log.Fatalf("instance lock error: %v", err)
	}
	if !first {
		if err := instance.Forward(identity, configPath, rawArgs); err != nil {
			log.Fatalf("handoff failed: %v", err)
		}
		fmt.Printf("Forwarded config to existing Vortex instance %q.\n", identity.DisplayName)
		os.Exit(0)
	}

	// On macOS/Linux, fork into a new session so the terminal is freed.
	// On Windows this is a no-op (handled by -H=windowsgui at build time).
	if !opts.dev && !opts.headless && !opts.forked {
		if maybeFork() {
			l.Close()
			return
		}
	}
	defer l.Close()
	httpPort := identity.HTTPPort
	if opts.portSet {
		httpPort = opts.port
	}
	cleanupRegistry, err := instance.Register(identity, httpPort, opts.dev, opts.headless, initialUIState(opts))
	if err != nil {
		log.Printf("instance registry warning: %v", err)
	} else {
		defer cleanupRegistry()
	}

	// --- Build orchestrator ---
	orch, err := orchestrator.New(cfg)
	if err != nil {
		log.Fatalf("orchestrator error: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	showUIRequests := make(chan struct{}, 1)
	var uiMu sync.Mutex
	uiOpen := false
	uiSuppressStop := false
	var closeUIWindow func(bool) bool
	closeUIWindow = func(bool) bool { return false }

	static := uiFS()
	if !opts.dev && static == nil {
		log.Fatal("non-dev mode requires embedded UI assets; run pnpm build in cmd/vortex-ui/web and start with go run -tags embed_ui ./cmd/vortex ..., or use --dev")
	}

	srv := server.New(orch, static, nil, opts.dev, "http://localhost:5173", server.InstanceInfo{Name: identity.DisplayName, RegistryName: identity.Name, HTTPPort: httpPort})
	addr := fmt.Sprintf("127.0.0.1:%d", httpPort)
	windowTitle := fmt.Sprintf("Vortex - %s", identity.DisplayName)
	windowURL := fmt.Sprintf("http://%s", addr)
	openUIWindow := func(stopOnClose bool) bool {
		uiMu.Lock()
		if uiOpen {
			uiMu.Unlock()
			return false
		}
		uiCtx, uiCancel := context.WithCancel(ctx)
		uiOpen = true
		uiSuppressStop = false
		if err := instance.SetUIState(identity, "open"); err != nil {
			log.Printf("instance registry warning: %v", err)
		}
		closeUIWindow = func(suppressStop bool) bool {
			uiMu.Lock()
			if !uiOpen {
				uiMu.Unlock()
				return false
			}
			uiSuppressStop = suppressStop
			uiMu.Unlock()
			uiCancel()
			return true
		}
		uiMu.Unlock()

		open := func() {
			webview.OpenWithContext(uiCtx, windowTitle, windowURL, 1280, 800)

			uiMu.Lock()
			suppressStop := uiSuppressStop
			uiOpen = false
			uiSuppressStop = false
			closeUIWindow = func(bool) bool { return false }
			uiMu.Unlock()
			if suppressStop {
				if err := instance.SetUIState(identity, "hidden"); err != nil {
					log.Printf("instance registry warning: %v", err)
				}
			}

			if stopOnClose && !suppressStop && ctx.Err() == nil {
				stop()
			}
		}
		if runtime.GOOS == "darwin" {
			open()
			return true
		}
		go open()
		return true
	}

	onHandoff := func(payload instance.HandoffPayload) {
		if payload.Action == "quit" {
			log.Printf("Shutdown requested for instance %q", identity.DisplayName)
			stop()
			return
		}
		if payload.Action == "hide-ui" {
			if opts.dev {
				log.Printf("Ignoring hide-ui for %q: instance is running in dev mode", identity.DisplayName)
				return
			}
			if !closeUIWindow(true) {
				log.Printf("Ignoring hide-ui for %q: UI is already hidden", identity.DisplayName)
				return
			}
			log.Printf("Hid native UI for %q", identity.DisplayName)
			return
		}
		if payload.Action == "show-ui" {
			select {
			case showUIRequests <- struct{}{}:
			default:
			}
			return
		}

		log.Printf("Received restart handoff for %q with args: %v", identity.DisplayName, payload.Args)
		handoffCfg, _, err := loadConfigFile(payload.ConfigFile, nil)
		if err != nil {
			log.Printf("handoff config error: %v", err)
			return
		}
		handoffIdentity, err := instance.NewIdentity(handoffCfg.Name)
		if err != nil {
			log.Printf("handoff identity error: %v", err)
			return
		}
		if handoffIdentity.Name != identity.Name {
			log.Printf("rejecting handoff for %q on instance %q", handoffIdentity.DisplayName, identity.DisplayName)
			return
		}
		if err := instance.Touch(identity); err != nil {
			log.Printf("instance registry warning: %v", err)
		}
		orch.Restart(ctx, handoffCfg)
	}

	// Serve the handoff endpoint on the per-instance lock listener
	// so that a second instance can POST to it. Without this, the raw TCP
	// listener accepts connections but never speaks HTTP, causing timeouts.
	instance.ServeHandoff(l, identity, onHandoff)

	// Start all jobs according to their dependency graph.
	orch.Start(ctx)
	startManagedPIDSync(ctx, identity, orch)

	// Start HTTP server in background.
	serverErr := make(chan error, 1)
	serverStopped := make(chan struct{})
	go func() {
		log.Printf("Vortex[%s] server listening on http://%s", identity.DisplayName, addr)
		serverErr <- server.ListenAndServe(ctx, addr, srv.Handler())
	}()
	go func() {
		err := <-serverErr
		if err != nil {
			log.Printf("server error: %v", err)
		}
		stop()
		close(serverStopped)
	}()

	if !opts.dev {
		if opts.headless {
			log.Printf("Headless mode for %q: open http://%s in your browser if needed", identity.DisplayName, addr)
		} else {
			openUIWindow(true)
		}
	} else {
		log.Printf("Dev mode for %q: open http://%s in your browser (or use the Vite dev server)", identity.DisplayName, addr)
	}

	for {
		select {
		case <-ctx.Done():
			orch.Shutdown()
			return
		case <-serverStopped:
			orch.Shutdown()
			return
		case <-showUIRequests:
			if opts.dev {
				log.Printf("Ignoring show-ui for %q: instance is running in dev mode", identity.DisplayName)
				continue
			}
			if static == nil {
				log.Printf("Ignoring show-ui for %q: embedded UI assets are unavailable", identity.DisplayName)
				continue
			}
			log.Printf("Opening native UI for %q", identity.DisplayName)
			if !openUIWindow(true) {
				log.Printf("Ignoring show-ui for %q: UI is already open", identity.DisplayName)
			}
		}
	}
}

func isHelpRequest(args []string) bool {
	return len(args) == 1 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h")
}

func isVersionRequest(args []string) bool {
	return len(args) == 1 && (args[0] == "version" || args[0] == "--version" || args[0] == "-v")
}

func printHelp(w io.Writer) {
	_, _ = fmt.Fprintf(w, `Vortex %s

Usage:
	vortex help
	vortex version
	vortex [--headless] [--port <n>] [--config <path>] config.yaml
	vortex --dev [--port <n>] [--config <path>] config.yaml
	vortex instances [name] [--json] [--prune]
	vortex <name> --quit
	vortex <name> --kill
	vortex <name> show-ui
	vortex <name> hide-ui
	vortex upgrade [--check]

Flags:
	-h, --help      Show this help text
	-v, --version   Show the current version
	--config        Path to YAML config file
	--port          Override the deterministic HTTP port for this instance
	--headless      Run without opening the native webview
	--dev           Development mode for browser/Vite workflow
	--quit          Ask a named Vortex instance to shut down and exit
	--kill          Ask a named Vortex instance to terminate its managed child processes

Examples:
	vortex help
	vortex version
	vortex --dev --config mock/dev.yaml
	vortex instances --json
	vortex dev --quit
`, Version)
}

func fatalCLIError(err error) {
	log.Printf("%v", err)
	fmt.Fprintln(os.Stderr, "Run 'vortex help' for usage.")
	os.Exit(1)
}

func printVersion() {
	fmt.Printf("vortex %s\n", Version)
	if GitCommit != "" && GitCommit != "unknown" {
		fmt.Printf("commit: %s\n", GitCommit)
	}
	if BuildTime != "" && BuildTime != "unknown" {
		fmt.Printf("built: %s\n", BuildTime)
	}
}

type cliOptions struct {
	dev         bool
	headless    bool
	quit        bool
	showUI      bool
	hideUI      bool
	kill        bool
	forked      bool
	port        int
	portSet     bool
	configFile  string
	positionals []string
}

func parseCLI(args []string) (cliOptions, error) {
	opts := cliOptions{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--dev":
			opts.dev = true
		case arg == "--headless":
			opts.headless = true
		case arg == "--quit":
			opts.quit = true
		case arg == "show-ui":
			opts.showUI = true
		case arg == "hide-ui":
			opts.hideUI = true
		case arg == "--kill":
			opts.kill = true
		case arg == "--forked":
			opts.forked = true
		case arg == "--config":
			i++
			if i >= len(args) {
				return cliOptions{}, fmt.Errorf("--config requires a path")
			}
			opts.configFile = args[i]
		case strings.HasPrefix(arg, "--config="):
			opts.configFile = strings.TrimPrefix(arg, "--config=")
		case arg == "--port":
			i++
			if i >= len(args) {
				return cliOptions{}, fmt.Errorf("--port requires a value")
			}
			port, err := strconv.Atoi(args[i])
			if err != nil {
				return cliOptions{}, fmt.Errorf("invalid --port value %q", args[i])
			}
			opts.port = port
			opts.portSet = true
		case strings.HasPrefix(arg, "--port="):
			portValue := strings.TrimPrefix(arg, "--port=")
			port, err := strconv.Atoi(portValue)
			if err != nil {
				return cliOptions{}, fmt.Errorf("invalid --port value %q", portValue)
			}
			opts.port = port
			opts.portSet = true
		case arg == "--":
			return cliOptions{}, fmt.Errorf("inline mode is no longer supported; provide a YAML config with a top-level name")
		case strings.HasPrefix(arg, "-"):
			return cliOptions{}, fmt.Errorf("unknown flag %q", arg)
		default:
			opts.positionals = append(opts.positionals, arg)
		}
	}
	if opts.dev && opts.headless {
		return cliOptions{}, fmt.Errorf("--dev and --headless cannot be used together")
	}
	actionCount := 0
	if opts.quit {
		actionCount++
	}
	if opts.showUI {
		actionCount++
	}
	if opts.hideUI {
		actionCount++
	}
	if opts.kill {
		actionCount++
	}
	if actionCount > 1 {
		return cliOptions{}, fmt.Errorf("choose only one instance control action: show-ui, hide-ui, --kill, or --quit")
	}
	return opts, nil
}

func loadConfigFile(configPath string, args []string) (*config.Config, string, error) {
	if configPath == "" {
		if len(args) == 0 {
			return nil, "", fmt.Errorf("missing config file; vortex now requires a named YAML config")
		}
		if len(args) > 1 {
			return nil, "", fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
		}
		ext := strings.ToLower(filepath.Ext(args[0]))
		if ext != ".yaml" && ext != ".yml" {
			return nil, "", fmt.Errorf("inline mode is no longer supported; provide a YAML config with a top-level name")
		}
		configPath = args[0]
	}
	if len(args) > 0 && configPath != args[0] {
		return nil, "", fmt.Errorf("unexpected positional args: %s", strings.Join(args, " "))
	}
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, "", fmt.Errorf("resolve config path: %w", err)
	}
	cfg, err := config.Load(absPath)
	if err != nil {
		return nil, "", err
	}
	return cfg, absPath, nil
}

func resolveTargetIdentity(opts cliOptions, usage string) (instance.Identity, error) {
	if opts.configFile != "" {
		if len(opts.positionals) > 0 {
			return instance.Identity{}, fmt.Errorf("use either a config file or an instance name, not both")
		}
		cfg, _, err := loadConfigFile(opts.configFile, nil)
		if err != nil {
			return instance.Identity{}, err
		}
		return instance.NewIdentity(cfg.Name)
	}
	if len(opts.positionals) != 1 {
		return instance.Identity{}, fmt.Errorf("%s", usage)
	}
	return instance.NewIdentity(opts.positionals[0])
}

type instanceListResponse struct {
	Instance struct {
		Name     string `json:"name"`
		HTTPPort int    `json:"http_port"`
	} `json:"instance"`
	Gen       int                    `json:"gen"`
	Terminals []instanceTerminalInfo `json:"terminals"`
}

type instanceTerminalInfo struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Command string `json:"command"`
	Status  string `json:"status"`
	PID     int    `json:"pid"`
}

type killProcessesResponse struct {
	Killed int `json:"killed"`
}

type instancesCommandOptions struct {
	filterName string
	jsonOutput bool
	prune      bool
}

type instancesJSONEntry struct {
	Name          string                 `json:"name"`
	DisplayName   string                 `json:"display_name"`
	Mode          string                 `json:"mode"`
	UI            string                 `json:"ui"`
	VortexPID     int                    `json:"vortex_pid"`
	HTTPPort      int                    `json:"http_port"`
	HandoffPort   int                    `json:"handoff_port"`
	StartedAt     int64                  `json:"started_at"`
	UpdatedAt     int64                  `json:"updated_at"`
	LastControlAt int64                  `json:"last_control_at,omitempty"`
	Generation    int                    `json:"generation,omitempty"`
	Reachable     bool                   `json:"reachable"`
	Error         string                 `json:"error,omitempty"`
	Terminals     []instanceTerminalInfo `json:"terminals"`
}

type instancesJSONOutput struct {
	Pruned    []string             `json:"pruned,omitempty"`
	Instances []instancesJSONEntry `json:"instances"`
}

func runInstancesCommand(args []string) error {
	opts, err := parseInstancesCommand(args)
	if err != nil {
		return err
	}

	instances, err := instance.ListMetadata()
	if err != nil {
		return err
	}
	if len(instances) == 0 {
		if opts.jsonOutput {
			return writeInstancesJSON(instancesJSONOutput{Instances: []instancesJSONEntry{}})
		}
		if opts.prune {
			fmt.Println("Pruned 0 stale instance entries.")
		}
		fmt.Println("No running Vortex instances.")
		return nil
	}
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].Name < instances[j].Name
	})

	entries := make([]instancesJSONEntry, 0, len(instances))
	prunedNames := make([]string, 0)
	shown := 0
	for _, meta := range instances {
		if opts.filterName != "" && meta.Name != opts.filterName {
			continue
		}
		entry := instancesJSONEntry{
			Name:          meta.Name,
			DisplayName:   meta.DisplayName,
			Mode:          instanceModeLabel(meta),
			UI:            instanceUIStateLabel(meta),
			VortexPID:     meta.PID,
			HTTPPort:      meta.HTTPPort,
			HandoffPort:   meta.HandoffPort,
			StartedAt:     meta.StartedAt,
			UpdatedAt:     meta.UpdatedAt,
			LastControlAt: meta.LastControlAt,
			Reachable:     false,
			Terminals:     []instanceTerminalInfo{},
		}
		body, err := fetchInstanceTerminals(meta)
		if err != nil {
			pruned, pruneErr := cleanupStaleInstance(meta)
			if pruneErr != nil {
				entry.Error = fmt.Sprintf("%v; stale cleanup failed: %v", err, pruneErr)
				entries = append(entries, entry)
				shown++
				continue
			}
			if pruned {
				prunedNames = append(prunedNames, meta.DisplayName)
				continue
			}
			entry.Error = err.Error()
			entries = append(entries, entry)
			shown++
			continue
		}
		entry.Generation = body.Gen
		entry.Reachable = true
		entry.Terminals = body.Terminals
		entries = append(entries, entry)
		shown++
	}

	if opts.jsonOutput {
		return writeInstancesJSON(instancesJSONOutput{Pruned: prunedNames, Instances: entries})
	}
	if opts.prune {
		if len(prunedNames) == 0 {
			fmt.Println("Pruned 0 stale instance entries.")
		} else {
			fmt.Printf("Pruned %d stale instance %s: %s\n", len(prunedNames), pluralWord(len(prunedNames)), strings.Join(prunedNames, ", "))
		}
	}

	if shown == 0 {
		fmt.Printf("No running Vortex instances matched %q.\n", opts.filterName)
		return nil
	}

	for _, entry := range entries {
		printInstanceEntry(entry)
	}
	return nil
}

func printInstanceEntry(entry instancesJSONEntry) {
	fmt.Printf("%s\n", entry.DisplayName)
	printInstanceField("name", entry.Name)
	printInstanceField("mode", entry.Mode)
	printInstanceField("ui", entry.UI)
	printInstanceField("reachable", strconv.FormatBool(entry.Reachable))
	printInstanceField("vortex pid", strconv.Itoa(entry.VortexPID))
	printInstanceField("http port", strconv.Itoa(entry.HTTPPort))
	printInstanceField("handoff port", strconv.Itoa(entry.HandoffPort))
	printInstanceField("started", formatInstanceTimestamp(entry.StartedAt))
	printInstanceField("updated", formatInstanceTimestamp(entry.UpdatedAt))
	printInstanceField("last control", formatInstanceTimestamp(entry.LastControlAt))
	if entry.Reachable {
		printInstanceField("generation", strconv.Itoa(entry.Generation))
	} else {
		printInstanceField("status", "unreachable")
		printInstanceField("error", entry.Error)
	}

	if len(entry.Terminals) == 0 {
		fmt.Println("  jobs:")
		fmt.Println("    - none")
		fmt.Println()
		return
	}

	fmt.Println("  jobs:")
	for _, term := range entry.Terminals {
		fmt.Printf("    - %s\n", term.ID)
		printJobField("label", term.Label)
		printJobField("pid", strconv.Itoa(term.PID))
		printJobField("status", term.Status)
		printJobField("command", term.Command)
	}
	fmt.Println()
}

func printInstanceField(label, value string) {
	fmt.Printf("  %-13s %s\n", label+":", value)
}

func printJobField(label, value string) {
	fmt.Printf("      %-9s %s\n", label+":", value)
}

func parseInstancesCommand(args []string) (instancesCommandOptions, error) {
	var opts instancesCommandOptions
	for _, arg := range args {
		switch {
		case arg == "--json":
			opts.jsonOutput = true
		case arg == "--prune":
			opts.prune = true
		case strings.HasPrefix(arg, "-"):
			return instancesCommandOptions{}, fmt.Errorf("unknown flag %q", arg)
		case opts.filterName != "":
			return instancesCommandOptions{}, fmt.Errorf("usage: vortex instances [name] [--json] [--prune]")
		default:
			identity, err := instance.NewIdentity(arg)
			if err != nil {
				return instancesCommandOptions{}, err
			}
			opts.filterName = identity.Name
		}
	}
	return opts, nil
}

func pluralWord(n int) string {
	if n == 1 {
		return "entry"
	}
	return "entries"
}

func instanceModeLabel(meta instance.Metadata) string {
	if meta.DevMode {
		return "dev"
	}
	if meta.Headless {
		return "headless"
	}
	return "windowed"
}

func instanceUIStateLabel(meta instance.Metadata) string {
	if meta.UIState != "" {
		return meta.UIState
	}
	if meta.DevMode {
		return "none"
	}
	if meta.Headless {
		return "hidden"
	}
	return "open"
}

func initialUIState(opts cliOptions) string {
	if opts.dev {
		return "none"
	}
	if opts.headless {
		return "hidden"
	}
	return "open"
}

func formatInstanceTimestamp(unixMillis int64) string {
	if unixMillis <= 0 {
		return "unknown"
	}
	return time.UnixMilli(unixMillis).Format(time.RFC3339)
}

func writeInstancesJSON(output instancesJSONOutput) error {
	if output.Instances == nil {
		output.Instances = []instancesJSONEntry{}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func cleanupStaleInstance(meta instance.Metadata) (bool, error) {
	identity, err := instance.NewIdentity(meta.Name)
	if err != nil {
		return false, err
	}
	l, first, err := instance.TryLock(identity)
	if err != nil {
		return false, err
	}
	if !first {
		return false, nil
	}
	if l != nil {
		_ = l.Close()
	}
	if err := instance.CleanupInactiveMetadata(meta); err != nil {
		return false, err
	}
	return true, nil
}

func startManagedPIDSync(ctx context.Context, identity instance.Identity, orch *orchestrator.Orchestrator) {
	sync := func() {
		pids := collectManagedPIDs(orch)
		if err := instance.SetManagedPIDs(identity, pids); err != nil {
			log.Printf("instance registry warning: %v", err)
		}
	}

	sync()
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sync()
			}
		}
	}()
}

func collectManagedPIDs(orch *orchestrator.Orchestrator) []int {
	jobs := orch.AllJobs()
	pids := make([]int, 0, len(jobs))
	for _, job := range jobs {
		if term := job.Terminal(); term != nil {
			if pid := term.PID(); pid > 0 {
				pids = append(pids, pid)
			}
		}
	}
	return pids
}

func killInstanceProcesses(identity instance.Identity) (killProcessesResponse, error) {
	meta, err := resolveInstanceMetadata(identity)
	if err != nil {
		return killProcessesResponse{}, err
	}
	client := &http.Client{}
	url := fmt.Sprintf("http://127.0.0.1:%d/api/processes", meta.HTTPPort)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return killProcessesResponse{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return killProcessesResponse{}, fmt.Errorf("could not reach Vortex instance %q: %w", identity.DisplayName, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return killProcessesResponse{}, fmt.Errorf("Vortex instance %q returned %s", identity.DisplayName, resp.Status)
	}
	var body killProcessesResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return killProcessesResponse{}, fmt.Errorf("decode process kill response: %w", err)
	}
	return body, nil
}

func fetchInstanceTerminals(meta instance.Metadata) (instanceListResponse, error) {
	client := &http.Client{}
	url := fmt.Sprintf("http://127.0.0.1:%d/api/terminals", meta.HTTPPort)
	resp, err := client.Get(url)
	if err != nil {
		return instanceListResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return instanceListResponse{}, fmt.Errorf("HTTP %s", resp.Status)
	}
	var body instanceListResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return instanceListResponse{}, err
	}
	return body, nil
}

func resolveInstanceMetadata(identity instance.Identity) (instance.Metadata, error) {
	instances, err := instance.ListMetadata()
	if err != nil {
		return instance.Metadata{}, err
	}
	for _, meta := range instances {
		if meta.Name == identity.Name {
			return meta, nil
		}
	}
	return instance.Metadata{}, fmt.Errorf("no running Vortex instance named %q", identity.DisplayName)
}
