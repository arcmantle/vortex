# Implementation Plan: GUI Spawns Host

## Overview

Add a second entry point: when the GUI binary (`vortex`) is launched without `--url`, it detects whether a bare-mode host (`vortex-host`) is already running. If not, it spawns one headlessly and detached. Then it reads the host's port+token from the instance registry and opens the webview at that URL. The host stays alive independently; the GUI is ephemeral.

## Architecture Decisions

- **Detection via instance registry**: Use `instance.GetMetadata("vortex")` to check if the bare host is running. No new IPC mechanism needed.
- **Spawn detached**: On Unix, `Setsid: true`. On Windows, `DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP` + `HideWindow: true` to prevent console window flash.
- **Poll for readiness**: After spawning, poll the registry (short sleeps) until the host writes its metadata, then read token+port.
- **No protocol change**: When `--url` is provided, behavior is unchanged (thin slave mode spawned by host for `vortex run`).
- **Bare instance name**: Always "vortex" (matches existing `bareInstanceName` const in cmd/vortex/main.go).

## Task List

### Phase 1: GUI Self-Launch Mode

- [ ] Task 1: Make `--url` optional in cmd/vortex-window/main.go
- [ ] Task 2: Add host binary path resolution in cmd/vortex-window
- [ ] Task 3: Implement host spawning (platform-specific detach)
- [ ] Task 4: Implement registry polling for host readiness
- [ ] Task 5: Wire it together — full self-launch flow

### Checkpoint: Self-Launch
- [ ] `go build ./...` succeeds
- [ ] Launching `vortex` (no args) spawns host and opens GUI
- [ ] Closing GUI leaves host running
- [ ] Launching `vortex` again reuses existing host

### Phase 2: Edge Cases

- [ ] Task 6: Handle host crash during startup (timeout + error)
- [ ] Task 7: Handle stale registry entry (process dead but file exists)

### Checkpoint: Robustness
- [ ] Stale registry detected and cleaned → host respawns
- [ ] Timeout gives clear error message

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Windows console flash | High (UX) | DETACHED_PROCESS + HideWindow flags |
| Race: two GUIs spawn two hosts | Medium | Instance lock (TryLock on handoff port) already serializes — second host will fail to bind and exit |
| Registry not written yet when GUI polls | Medium | Retry with backoff, 5s timeout |
| Host binary not found | Low | Clear error message pointing to install |

## Open Questions

- None — all mechanisms already exist in the codebase.
