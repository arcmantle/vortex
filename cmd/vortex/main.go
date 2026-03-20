// Vortex — start processes and stream their output to a webview UI.
//
// Usage (Vortex config):
//
//	vortex run [--headless] [--port <n>] config.vortex
//	vortex run --dev [--port <n>] config.vortex
//
// Usage (instances):
//
//	vortex instance list [name]
//
// Usage (instance control):
//
//	vortex instance quit <name>
//	vortex instance kill <name>
//	vortex instance rerun <name> <job-id>
//	vortex instance show <name>
//	vortex instance hide <name>
//
// Usage (self-update):
//
//	vortex help
//	vortex docs
//	vortex version
//	vortex -v
//	vortex upgrade
//
// The Vortex config file defines jobs with optional groups and dependency
// conditions. See internal/config/config.go for the full schema.
package main

import (
	"context"
	"encoding/json"
	"fmt"
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
	"arcmantle/vortex/internal/webview"
	"arcmantle/windowfocus"
)

// Version info, set via ldflags at build time.
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	root := rootCommand()
	if err := validateCommandPath(root, os.Args[1:]); err != nil {
		log.Fatal(err)
	}
	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}

func executeRunCommand(opts cliOptions) error {
	opts.portSet = opts.port != 0
	rawArgs := buildRunRawArgs(opts)
	return runWithOptions(rawArgs, opts)
}

func buildRunRawArgs(opts cliOptions) []string {
	rawArgs := []string{"run"}
	if opts.dev {
		rawArgs = append(rawArgs, "--dev")
	}
	if opts.headless {
		rawArgs = append(rawArgs, "--headless")
	}
	if opts.forked {
		rawArgs = append(rawArgs, "--forked")
	}
	if opts.portSet {
		rawArgs = append(rawArgs, "--port", strconv.Itoa(opts.port))
	}
	if strings.TrimSpace(opts.cwd) != "" {
		rawArgs = append(rawArgs, "--cwd", opts.cwd)
	}
	if opts.configFile != "" {
		rawArgs = append(rawArgs, "--config", opts.configFile)
	}
	rawArgs = append(rawArgs, opts.positionals...)
	return rawArgs
}

func runWithOptions(rawArgs []string, opts cliOptions) error {
	if opts.dev && opts.headless {
		return fmt.Errorf("--dev and --headless cannot be used together")
	}
	cfg, configPath, err := loadConfigFile(opts.configFile, opts.positionals)
	if err != nil {
		return fmt.Errorf("config error: %w", err)
	}
	if cfg.WorkingDir, err = resolveWorkingDir(configPath, opts.cwd); err != nil {
		return fmt.Errorf("config error: %w", err)
	}

	identity, err := instance.NewIdentity(cfg.Name)
	if err != nil {
		return fmt.Errorf("config error: invalid name %q: %w", cfg.Name, err)
	}

	// --- Named-instance check ---
	l, first, err := instance.TryLock(identity)
	if err != nil {
		return fmt.Errorf("instance lock error: %w", err)
	}
	if !first {
		if err := instance.Forward(identity, configPath, rawArgs); err != nil {
			return fmt.Errorf("handoff failed: %w", err)
		}
		fmt.Printf("Forwarded config to existing Vortex instance %q.\n", identity.DisplayName)
		return nil
	}

	// Non-dev launches detach from the caller's terminal. On Unix this forks
	// into a new session. On Windows, GUI-subsystem builds already start
	// detached and console-attached launches respawn without the console.
	if shouldDetachFromTerminal(opts) {
		if maybeFork() {
			l.Close()
			return nil
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
		return fmt.Errorf("orchestrator error: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	showUIRequests := make(chan struct{}, 1)
	var uiMu sync.Mutex
	uiOpen := false
	uiSuppressStop := false
	var closeUIWindow func(bool) bool
	closeUIWindow = func(bool) bool { return false }
	var focusUIWindow func() bool
	focusUIWindow = func() bool { return false }

	static := uiFS()
	if !opts.dev && static == nil {
		return fmt.Errorf("non-dev mode requires embedded UI assets; run pnpm build in cmd/vortex-ui/web and start with go run -tags embed_ui ./cmd/vortex ..., or use --dev")
	}

	srv := server.New(ctx, orch, static, nil, opts.dev, "http://localhost:5173", server.InstanceInfo{Name: identity.DisplayName, RegistryName: identity.Name, HTTPPort: httpPort})
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
		focusUIWindow = func() bool { return false }
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
			webview.OpenWithContextAndReady(uiCtx, windowTitle, windowURL, 1280, 800, func(controller webview.Controller) {
				if controller == nil {
					return
				}
				uiMu.Lock()
				if uiOpen {
					focusUIWindow = func() bool {
						uiMu.Lock()
						defer uiMu.Unlock()
						if !uiOpen {
							return false
						}
						controller.Focus()
						return true
					}
				}
				uiMu.Unlock()
			})

			uiMu.Lock()
			suppressStop := uiSuppressStop
			uiOpen = false
			uiSuppressStop = false
			closeUIWindow = func(bool) bool { return false }
			focusUIWindow = func() bool { return false }
			uiMu.Unlock()
			if suppressStop {
				windowfocus.HideApp()
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
		if payload.Action == "rerun" {
			if len(payload.Args) != 1 || strings.TrimSpace(payload.Args[0]) == "" {
				log.Printf("Ignoring rerun for %q: missing job id", identity.DisplayName)
				return
			}
			jobID := strings.TrimSpace(payload.Args[0])
			if err := orch.Rerun(ctx, jobID); err != nil {
				log.Printf("rerun request failed for %q on instance %q: %v", jobID, identity.DisplayName, err)
				return
			}
			if err := instance.MarkControlAction(identity); err != nil {
				log.Printf("instance registry warning: %v", err)
			}
			log.Printf("Rerunning %q for instance %q", jobID, identity.DisplayName)
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
		handoffCfg, handoffConfigPath, err := loadConfigFile(payload.ConfigFile, nil)
		if err != nil {
			log.Printf("handoff config error: %v", err)
			return
		}
		handoffCwd, err := cwdFromRunArgs(payload.Args)
		if err != nil {
			log.Printf("handoff cwd error: %v", err)
			return
		}
		if handoffCfg.WorkingDir, err = resolveWorkingDir(handoffConfigPath, handoffCwd); err != nil {
			log.Printf("handoff cwd error: %v", err)
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
			return nil
		case <-serverStopped:
			orch.Shutdown()
			return nil
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
				if focusUIWindow() {
					log.Printf("Surfaced native UI for %q", identity.DisplayName)
					continue
				}
				log.Printf("Ignoring show-ui for %q: UI is already open", identity.DisplayName)
			}
		}
	}
}

func resolveCommandIdentity(nameArg, configPath, usage string) (instance.Identity, error) {
	if configPath != "" {
		if strings.TrimSpace(nameArg) != "" {
			return instance.Identity{}, fmt.Errorf("use either a config file or an instance name, not both")
		}
		cfg, _, err := loadConfigFile(configPath, nil)
		if err != nil {
			return instance.Identity{}, err
		}
		return instance.NewIdentity(cfg.Name)
	}
	if strings.TrimSpace(nameArg) == "" {
		return instance.Identity{}, fmt.Errorf("%s", usage)
	}
	return instance.NewIdentity(nameArg)
}

func runQuitCommand(identity instance.Identity) error {
	l, first, err := instance.TryLock(identity)
	if err != nil {
		return fmt.Errorf("instance lock error: %w", err)
	}
	if first {
		if l != nil {
			_ = l.Close()
		}
		if meta, metaErr := instance.GetMetadata(identity.Name); metaErr == nil {
			_ = instance.CleanupInactiveMetadata(meta)
		}
		fmt.Printf("No active Vortex instance named %q.\n", identity.DisplayName)
		return nil
	}
	if err := instance.Quit(identity); err != nil {
		return fmt.Errorf("shutdown request failed: %w", err)
	}
	fmt.Printf("Requested shutdown of Vortex instance %q.\n", identity.DisplayName)
	return nil
}

func runKillCommand(identity instance.Identity) error {
	result, err := killInstanceProcesses(identity)
	if err != nil {
		return err
	}
	fmt.Printf("Requested termination of %d process(es) for Vortex instance %q.\n", result.Killed, identity.DisplayName)
	return nil
}

func runShowUICommand(identity instance.Identity) error {
	meta, err := resolveInstanceMetadata(identity)
	if err != nil {
		return err
	}
	if meta.DevMode {
		return fmt.Errorf("Vortex instance %q is running in --dev mode and cannot open a native webview", identity.DisplayName)
	}
	if err := instance.ShowUI(identity); err != nil {
		return err
	}
	fmt.Printf("Requested UI for Vortex instance %q.\n", identity.DisplayName)
	return nil
}

func runHideUICommand(identity instance.Identity) error {
	meta, err := resolveInstanceMetadata(identity)
	if err != nil {
		return err
	}
	if meta.DevMode {
		return fmt.Errorf("Vortex instance %q is running in --dev mode and has no native webview to hide", identity.DisplayName)
	}
	if err := instance.HideUI(identity); err != nil {
		return err
	}
	fmt.Printf("Requested UI hide for Vortex instance %q.\n", identity.DisplayName)
	return nil
}

func runRerunCommand(identity instance.Identity, jobID string) error {
	l, first, err := instance.TryLock(identity)
	if err != nil {
		return fmt.Errorf("instance lock error: %w", err)
	}
	if first {
		if l != nil {
			_ = l.Close()
		}
		if meta, metaErr := instance.GetMetadata(identity.Name); metaErr == nil {
			_ = instance.CleanupInactiveMetadata(meta)
		}
		fmt.Printf("No active Vortex instance named %q.\n", identity.DisplayName)
		return nil
	}
	if err := instance.Rerun(identity, strings.TrimSpace(jobID)); err != nil {
		return err
	}
	fmt.Printf("Requested rerun of %q for Vortex instance %q.\n", strings.TrimSpace(jobID), identity.DisplayName)
	return nil
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
	forked      bool
	port        int
	portSet     bool
	cwd         string
	configFile  string
	positionals []string
}

func shouldDetachFromTerminal(opts cliOptions) bool {
	return !opts.dev && !opts.forked
}

func loadConfigFile(configPath string, args []string) (*config.Config, string, error) {
	if configPath == "" {
		if len(args) == 0 {
			return nil, "", fmt.Errorf("missing config file; vortex now requires a named config")
		}
		if len(args) > 1 {
			return nil, "", fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
		}
		if !isSupportedConfigPath(args[0]) {
			return nil, "", fmt.Errorf("inline mode is no longer supported; provide a Vortex config with a top-level name")
		}
		configPath = args[0]
	}
	if len(args) > 0 && configPath != args[0] {
		return nil, "", fmt.Errorf("unexpected positional args: %s", strings.Join(args, " "))
	}
	resolvedConfigPath, err := resolveConfigPath(configPath)
	if err != nil {
		return nil, "", err
	}
	absPath, err := filepath.Abs(resolvedConfigPath)
	if err != nil {
		return nil, "", fmt.Errorf("resolve config path: %w", err)
	}
	cfg, err := config.Load(absPath)
	if err != nil {
		return nil, "", err
	}
	return cfg, absPath, nil
}

func isSupportedConfigPath(path string) bool {
	lower := strings.ToLower(path)
	ext := filepath.Ext(lower)
	return ext == "" || ext == ".vortex"
}

func resolveConfigPath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("config path must not be empty")
	}

	lower := strings.ToLower(trimmed)
	ext := filepath.Ext(lower)
	if ext == "" {
		return trimmed + ".vortex", nil
	}
	if ext != ".vortex" {
		return "", fmt.Errorf("config path must end in .vortex")
	}
	return trimmed, nil
}

func resolveWorkingDir(configPath, override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return "", fmt.Errorf("resolve cwd: %w", err)
		}
		return abs, nil
	}
	return filepath.Dir(configPath), nil
}

func cwdFromRunArgs(args []string) (string, error) {
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			break
		}
		if arg == "--cwd" {
			if index+1 >= len(args) {
				return "", fmt.Errorf("--cwd requires a value")
			}
			return args[index+1], nil
		}
		if strings.HasPrefix(arg, "--cwd=") {
			return strings.TrimPrefix(arg, "--cwd="), nil
		}
	}
	return "", nil
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

func runInstancesCommandWithOptions(opts instancesCommandOptions) error {
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
	fmt.Println("Tip: rerun a job with 'vortex instance rerun <instance> <job-id>'.")
	fmt.Println()

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
