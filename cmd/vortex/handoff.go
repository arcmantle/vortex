package main

import (
	"context"
	"log"
	"strings"

	"arcmantle/vortex/internal/instance"
	"arcmantle/vortex/internal/orchestrator"
)

// handoffHandler builds the callback used by instance.ServeHandoff to process
// actions from a second Vortex invocation: quit, rerun, show/hide-ui, and
// config-reload restarts.
func handoffHandler(
	ctx context.Context,
	stop context.CancelFunc,
	identity instance.Identity,
	orch *orchestrator.Orchestrator,
	ui *uiLifecycle,
	showUIRequests chan<- struct{},
	opts cliOptions,
) func(instance.HandoffPayload) {
	return func(payload instance.HandoffPayload) {
		switch payload.Action {
		case "quit":
			log.Printf("Shutdown requested for instance %q", identity.DisplayName)
			stop()

		case "rerun":
			handleRerun(ctx, identity, orch, payload)

		case "hide-ui":
			handleHideUI(identity, ui, opts)

		case "show-ui":
			select {
			case showUIRequests <- struct{}{}:
			default:
			}

		default:
			handleRestart(ctx, identity, orch, payload)
			// After reloading config, surface the UI so the user sees the result.
			select {
			case showUIRequests <- struct{}{}:
			default:
			}
		}
	}
}

func handleRerun(ctx context.Context, identity instance.Identity, orch *orchestrator.Orchestrator, payload instance.HandoffPayload) {
	if orch == nil {
		log.Printf("Ignoring rerun for %q: no orchestrator (bare mode)", identity.DisplayName)
		return
	}
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
}

func handleHideUI(identity instance.Identity, ui *uiLifecycle, opts cliOptions) {
	if opts.dev {
		log.Printf("Ignoring hide-ui for %q: instance is running in dev mode", identity.DisplayName)
		return
	}
	if !ui.Close(true) {
		log.Printf("Ignoring hide-ui for %q: UI is already hidden", identity.DisplayName)
		return
	}
	log.Printf("Hid native UI for %q", identity.DisplayName)
}

func handleRestart(ctx context.Context, identity instance.Identity, orch *orchestrator.Orchestrator, payload instance.HandoffPayload) {
	if orch == nil {
		log.Printf("Ignoring restart handoff for %q: no orchestrator (bare mode)", identity.DisplayName)
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
