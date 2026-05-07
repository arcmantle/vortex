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
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"arcmantle/vortex/internal/config"
	"arcmantle/vortex/internal/favorites"
	"arcmantle/vortex/internal/instance"
	"arcmantle/vortex/internal/orchestrator"
	"arcmantle/vortex/internal/server"
)

// Version info, set via ldflags at build time.
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	// Windows: the uninstall cleanup helper is invoked as a detached process
	// with --uninstall-cleanup <pid> <installDir> <guiInstallDir> [...].
	// Intercept before cobra parsing since it's not a normal subcommand.
	if len(os.Args) > 1 && os.Args[1] == "--uninstall-cleanup" {
		runUninstallCleanup(os.Args[2:])
		return
	}

	prepareConsoleForCLI(os.Args[1:])
	defer cleanupConsole()
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
	if opts.portSet && (opts.port < 1 || opts.port > 65535) {
		return fmt.Errorf("--port must be between 1 and 65535, got %d", opts.port)
	}
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
		forked, forkErr := maybeFork()
		if forkErr != nil {
			l.Close()
			return fmt.Errorf("fork error: %w", forkErr)
		}
		if forked {
			l.Close()
			return nil
		}
	}

	// The forked child has no terminal attached, so redirect log output to a
	// file so diagnostics are not silently lost.
	if opts.forked {
		if logFile, err := openLogFile(identity.Name); err != nil {
			log.Printf("warning: could not open log file: %v", err)
		} else {
			log.SetOutput(logFile)
			defer logFile.Close()
		}
	}

	defer l.Close()
	httpPort := identity.HTTPPort
	if opts.portSet {
		httpPort = opts.port
	}
	sessionToken, cleanupRegistry, err := instance.Register(identity, httpPort, opts.dev, opts.headless, initialUIState(opts))
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
	static := uiFS()
	if !opts.dev && static == nil {
		return fmt.Errorf("non-dev mode requires embedded UI assets; build the frontend first (pnpm build in cmd/vortex-ui/web), or use --dev")
	}

	srv := server.New(ctx, orch, static, opts.dev, "http://localhost:5173", server.InstanceInfo{Name: identity.DisplayName, RegistryName: identity.Name, HTTPPort: httpPort}, sessionToken, configPath, cfg.WorkingDir)
	addr := fmt.Sprintf("127.0.0.1:%d", httpPort)
	windowTitle := fmt.Sprintf("Vortex - %s", identity.DisplayName)
	windowURL := fmt.Sprintf("http://%s?token=%s", addr, sessionToken)
	ui := newUILifecycle(identity, windowTitle, windowURL)
	ui.onClose = func() { orch.KillOnCloseJobs() }

	// Serve the handoff endpoint on the per-instance lock listener
	// so that a second instance can POST to it. Without this, the raw TCP
	// listener accepts connections but never speaks HTTP, causing timeouts.
	instance.ServeHandoff(l, identity, sessionToken, handoffHandler(ctx, stop, identity, orch, ui, showUIRequests, opts))

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
			ui.Open(ctx, stop, false)
		}
	} else {
		log.Printf("Dev mode for %q: open http://%s in your browser (or use the Vite dev server)", identity.DisplayName, addr)
	}

	eventLoop(ctx, stop, orch, ui, showUIRequests, serverStopped, opts, identity, static)
	return nil
}

func eventLoop(ctx context.Context, stop context.CancelFunc, orch *orchestrator.Orchestrator, ui *uiLifecycle, showUIRequests <-chan struct{}, serverStopped <-chan struct{}, opts cliOptions, identity instance.Identity, static fs.FS) {
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
			if !ui.Open(ctx, stop, false) {
				if ui.Focus() {
					log.Printf("Surfaced native UI for %q", identity.DisplayName)
					continue
				}
				log.Printf("Ignoring show-ui for %q: UI is already open", identity.DisplayName)
			}
		}
	}
}

const bareInstanceName = "vortex"
const bareDefaultPort = 7370

func runBareMode(opts cliOptions) error {
	opts.portSet = opts.port != 0
	if opts.portSet && (opts.port < 1 || opts.port > 65535) {
		return fmt.Errorf("--port must be between 1 and 65535, got %d", opts.port)
	}
	if opts.dev && opts.headless {
		return fmt.Errorf("--dev and --headless cannot be used together")
	}

	identity, err := instance.NewIdentity(bareInstanceName)
	if err != nil {
		return fmt.Errorf("internal error: %w", err)
	}

	// Override identity port to use bare default.
	httpPort := bareDefaultPort
	if opts.portSet {
		httpPort = opts.port
	}

	// --- Named-instance check ---
	l, first, err := instance.TryLock(identity)
	if err != nil {
		return fmt.Errorf("instance lock error: %w", err)
	}
	if !first {
		// Singleton already running — request it to show its UI.
		if err := instance.ShowUI(identity); err != nil {
			return fmt.Errorf("failed to connect to running instance: %w", err)
		}
		fmt.Println("Connected to running Vortex instance.")
		return nil
	}

	// Detach from terminal in non-dev mode.
	if shouldDetachFromTerminal(opts) {
		forked, forkErr := maybeFork()
		if forkErr != nil {
			l.Close()
			return fmt.Errorf("fork error: %w", forkErr)
		}
		if forked {
			l.Close()
			return nil
		}
	}

	if opts.forked {
		if logFile, err := openLogFile(identity.Name); err != nil {
			log.Printf("warning: could not open log file: %v", err)
		} else {
			log.SetOutput(logFile)
			defer logFile.Close()
		}
	}

	defer l.Close()
	sessionToken, cleanupRegistry, err := instance.Register(identity, httpPort, opts.dev, opts.headless, initialUIState(opts))
	if err != nil {
		log.Printf("instance registry warning: %v", err)
	} else {
		defer cleanupRegistry()
	}

	// Resolve working directory for shells — use $HOME.
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		homeDir, _ = os.Getwd()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	showUIRequests := make(chan struct{}, 1)
	static := uiFS()
	if !opts.dev && static == nil {
		return fmt.Errorf("non-dev mode requires embedded UI assets; build the frontend first (pnpm build in cmd/vortex-ui/web), or use --dev")
	}

	srv := server.New(ctx, nil, static, opts.dev, "http://localhost:5173", server.InstanceInfo{Name: identity.DisplayName, RegistryName: identity.Name, HTTPPort: httpPort}, sessionToken, "", homeDir)
	addr := fmt.Sprintf("127.0.0.1:%d", httpPort)
	windowTitle := "Vortex"
	windowURL := fmt.Sprintf("http://%s?token=%s", addr, sessionToken)
	ui := newUILifecycle(identity, windowTitle, windowURL)

	instance.ServeHandoff(l, identity, sessionToken, handoffHandler(ctx, stop, identity, nil, ui, showUIRequests, opts))

	// Start HTTP server in background.
	serverErr := make(chan error, 1)
	serverStopped := make(chan struct{})
	go func() {
		log.Printf("Vortex listening on http://%s", addr)
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
			log.Printf("Headless mode: open http://%s in your browser", addr)
		} else {
			ui.Open(ctx, stop, false)
		}
	} else {
		log.Printf("Dev mode: open http://%s in your browser (or use the Vite dev server)", addr)
	}

	bareEventLoop(ctx, stop, ui, showUIRequests, serverStopped, opts, identity, static)
	return nil
}

func bareEventLoop(ctx context.Context, stop context.CancelFunc, ui *uiLifecycle, showUIRequests <-chan struct{}, serverStopped <-chan struct{}, opts cliOptions, identity instance.Identity, static fs.FS) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-serverStopped:
			return
		case <-showUIRequests:
			if opts.dev {
				log.Printf("Ignoring show-ui: instance is running in dev mode")
				continue
			}
			if static == nil {
				log.Printf("Ignoring show-ui: embedded UI assets are unavailable")
				continue
			}
			if !ui.Open(ctx, stop, false) {
				if ui.Focus() {
					continue
				}
			}
		}
	}
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
		// Resolve @alias favorites before checking config path format.
		if favorites.IsFavoriteRef(args[0]) {
			resolved, err := favorites.Resolve(favorites.ParseFavoriteRef(args[0]))
			if err != nil {
				return nil, "", fmt.Errorf("favorite: %w", err)
			}
			configPath = resolved
		} else {
			if !isSupportedConfigPath(args[0]) {
				return nil, "", fmt.Errorf("inline mode is no longer supported; provide a Vortex config with a top-level name")
			}
			configPath = args[0]
		}
	}
	if len(args) > 0 && configPath != args[0] && !favorites.IsFavoriteRef(args[0]) {
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

// openLogFile creates (or truncates) a log file for the current instance in
// the user cache directory. The caller is responsible for closing the file.
func openLogFile(instanceName string) (*os.File, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("user cache dir: %w", err)
	}
	logDir := filepath.Join(cacheDir, "vortex", "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	logPath := filepath.Join(logDir, instanceName+".log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	return f, nil
}
