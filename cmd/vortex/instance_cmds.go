package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"arcmantle/vortex/internal/instance"
)

// ---------------------------------------------------------------------------
// Instance CLI command implementations
// ---------------------------------------------------------------------------

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
			if cleanupErr := instance.CleanupInactiveMetadata(meta); cleanupErr != nil {
				log.Printf("cleanup warning: %v", cleanupErr)
			}
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
			if cleanupErr := instance.CleanupInactiveMetadata(meta); cleanupErr != nil {
				log.Printf("cleanup warning: %v", cleanupErr)
			}
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

// ---------------------------------------------------------------------------
// Instance list types and display
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// HTTP client helpers for talking to running instances
// ---------------------------------------------------------------------------

func killInstanceProcesses(identity instance.Identity) (killProcessesResponse, error) {
	meta, err := instance.GetMetadata(identity.Name)
	if err != nil {
		return killProcessesResponse{}, err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/api/processes", meta.HTTPPort)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return killProcessesResponse{}, err
	}
	if meta.Token != "" {
		req.Header.Set("Authorization", "Bearer "+meta.Token)
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
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return killProcessesResponse{}, fmt.Errorf("decode process kill response: %w", err)
	}
	return body, nil
}

func fetchInstanceTerminals(meta instance.Metadata) (instanceListResponse, error) {
	// ListMetadata strips tokens; re-read for auth if needed.
	if meta.Token == "" {
		if full, err := instance.GetMetadata(meta.Name); err == nil {
			meta.Token = full.Token
		}
	}
	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/api/terminals", meta.HTTPPort)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return instanceListResponse{}, err
	}
	if meta.Token != "" {
		req.Header.Set("Authorization", "Bearer "+meta.Token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return instanceListResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return instanceListResponse{}, fmt.Errorf("HTTP %s", resp.Status)
	}
	var body instanceListResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
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
