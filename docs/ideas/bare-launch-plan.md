# Implementation Plan: Bare Launch (Standalone Terminal Mode)

## Overview
Make Vortex launchable with zero arguments as a persistent terminal manager.
`vortex` (no args) starts or reconnects to a singleton host process that keeps shell
PTYs alive across window open/close cycles. Config-based `vortex run` continues
to work but also adopts the "window close = detach" behavior.

## Architecture Decisions
- **Singleton identity:** Bare-launch uses a fixed instance name `"vortex"` with port 7370.
- **Nil orchestrator:** Server accepts `orch == nil`; job-related endpoints return empty/404.
- **Unified detach:** All hosts (bare and config-based) survive window close. Explicit `vortex stop` or `vortex instance quit` kills the host.
- **Reconnect:** Second `vortex` invocation detects running singleton via instance lock → opens a new window pointed at the existing host.
- **WorkDir for shells in bare mode:** `$HOME` (or current cwd).

## Task List

### Phase 1: Backend — Nil Orchestrator Support

- [ ] Task 1: Make server accept nil orchestrator
- [ ] Task 2: Add bare-launch entry point in CLI

### Checkpoint: Bare Backend
- [ ] `CGO_ENABLED=0 go build ./cmd/vortex/` succeeds
- [ ] `CGO_ENABLED=0 go test ./internal/... -count=1` passes
- [ ] `vortex` (no args) starts server on 7370 with no jobs

### Phase 2: Detach Lifecycle

- [ ] Task 3: Change window-close semantics to detach (not exit)
- [ ] Task 4: Reconnect on second `vortex` invocation
- [ ] Task 5: Add `vortex stop` command

### Checkpoint: Persistent Daemon
- [ ] `vortex` starts host → closing window leaves host running
- [ ] `vortex` again → opens new window connected to same host
- [ ] `vortex stop` terminates the host
- [ ] All existing `vortex run` behavior still works

### Phase 3: Frontend — Disconnection UX

- [ ] Task 6: Host-disconnected banner in webview
- [ ] Task 7: Hide job-related UI when no orchestrator

### Checkpoint: Complete
- [ ] TypeScript compiles: `pnpm exec tsc --noEmit`
- [ ] Go builds and tests pass
- [ ] End-to-end flow works (bare launch → shell → close → reopen → scrollback intact)

---

## Task Details

### Task 1: Make server accept nil orchestrator

**Description:** The `server.New()` constructor currently requires a non-nil `*orchestrator.Orchestrator`. Refactor so that when `orch` is nil, job-related API endpoints return empty arrays or 404, and the server still boots. The `ShellManager` workDir should fall back to a provided string when orchestrator is nil.

**Acceptance criteria:**
- [ ] `server.New()` accepts nil orchestrator without panic
- [ ] `GET /api/terminals` returns `[]` when orch is nil
- [ ] `GET /api/terminals/{id}` returns 404 for any id when orch is nil
- [ ] `GET /events` only serves shell events when orch is nil
- [ ] `POST /api/terminals/{id}/rerun` returns 404 when orch is nil
- [ ] `DELETE /api/processes` is a no-op when orch is nil
- [ ] `GET /api/config-file` returns 404 when no configPath
- [ ] Existing tests still pass

**Verification:**
- [ ] `CGO_ENABLED=0 go test ./internal/server/ -count=1`
- [ ] `CGO_ENABLED=0 go build ./cmd/vortex/`

**Dependencies:** None

**Files likely touched:**
- `internal/server/server.go` (nil-guard in handlers, signature change for New)

**Estimated scope:** Medium (single file, many handler touch-points)

---

### Task 2: Add bare-launch entry point in CLI

**Description:** When `vortex` is invoked with no subcommand and no args (and no `-v` flag), instead of printing help, start a singleton host. Use fixed identity name `"vortex"`, port 7370, no orchestrator, shells-only. If the singleton is already running, forward to it (which triggers show-ui via Task 4). Respects `--dev`, `--headless`, `--port` flags on the root command for bare mode.

**Acceptance criteria:**
- [ ] `vortex` (no args) starts a host process on port 7370
- [ ] Instance name is `"vortex"`, identity lock works
- [ ] Server starts with nil orchestrator
- [ ] Default shell tab opens in the webview (one shell auto-created)
- [ ] `--dev` flag works for development
- [ ] If singleton already locked, forwards show-ui action (handled by Task 4)
- [ ] `vortex run config.vortex` still works exactly as before
- [ ] `vortex -v` still prints version

**Verification:**
- [ ] `CGO_ENABLED=0 go build -o ./vortex-test ./cmd/vortex/ && ./vortex-test --dev`
- [ ] Server listens on 7370, shell API works

**Dependencies:** Task 1

**Files likely touched:**
- `cmd/vortex/cobra.go` (root command RunE logic)
- `cmd/vortex/main.go` (new `runBareMode()` function, parallel to `runWithOptions`)

**Estimated scope:** Medium (2-3 files)

---

### Task 3: Change window-close semantics to detach (not exit)

**Description:** Currently, when `vortex-window` exits, `markClosed()` calls `stop()` which cancels the context and shuts down the entire host. Change this so window close means "detach" — the host stays alive. The `stop()` should only be called on explicit quit (signal, `vortex stop`, or a new "Quit" action from the UI).

**Acceptance criteria:**
- [ ] Closing the native window does NOT terminate the host process
- [ ] Host process continues serving HTTP and maintaining shell PTYs
- [ ] `Ctrl+C` / SIGTERM still stops the host (for dev mode ergonomics)
- [ ] `vortex instance quit <name>` still stops the host
- [ ] Instance registry updates UI state to "closed" when window closes

**Verification:**
- [ ] Start `vortex` → close window → `curl http://127.0.0.1:7370/api/terminals` still responds
- [ ] `vortex instance list` shows the instance with ui_state "closed"

**Dependencies:** Task 2

**Files likely touched:**
- `cmd/vortex/ui_lifecycle.go` (change `stopOnClose` default)
- `cmd/vortex/main.go` (pass `stopOnClose: false` to `ui.Open`)
- `cmd/vortex/main.go` (event loop no longer exits when UI closes)

**Estimated scope:** Small (2 files, targeted changes)

---

### Task 4: Reconnect on second `vortex` invocation

**Description:** When `vortex` (bare) is invoked and the singleton lock is already held, instead of printing "forwarded config", send a `show-ui` handoff action. The running host receives it, opens a new window pointed at its HTTP server. The second CLI process exits immediately.

**Acceptance criteria:**
- [ ] Second `vortex` invocation with running singleton opens a new window
- [ ] If window is already open, focuses the existing window
- [ ] CLI exits cleanly with a message like "Connected to running Vortex instance"
- [ ] Works for both bare singleton and config-based instances

**Verification:**
- [ ] Start `vortex --dev` → in another terminal `vortex` → see "Connected" message
- [ ] `vortex instance list` shows ui_state "open" after reconnect

**Dependencies:** Task 3

**Files likely touched:**
- `cmd/vortex/cobra.go` or `cmd/vortex/main.go` (bare-mode lock-miss path)
- `cmd/vortex/handoff.go` (handle show-ui action for bare instances)

**Estimated scope:** Small (1-2 files)

---

### Task 5: Add `vortex stop` command

**Description:** Add a top-level `vortex stop [name]` command. With no name, it stops the bare singleton (`"vortex"`). With a name, it stops that named instance. This is sugar over `vortex instance quit`.

**Acceptance criteria:**
- [ ] `vortex stop` sends quit to the singleton instance
- [ ] `vortex stop my-project` sends quit to the named instance
- [ ] Prints confirmation or error if instance not running
- [ ] Host process terminates gracefully (shells closed, PTYs cleaned up)

**Verification:**
- [ ] `vortex --dev` → another terminal: `vortex stop` → host exits
- [ ] Exit code 0 on success

**Dependencies:** Task 3

**Files likely touched:**
- `cmd/vortex/cobra.go` (add stopCommand)
- `cmd/vortex/instance_cmds.go` (reuse `runQuitCommand`)

**Estimated scope:** XS-Small (1-2 files, trivial wiring)

---

### Task 6: Host-disconnected banner in webview

**Description:** When the SSE connection drops and cannot reconnect (host crashed or was stopped), show a non-dismissible banner in the webview: "Host disconnected — run `vortex` to restart". This replaces the current behavior where the webview just stops updating silently.

**Acceptance criteria:**
- [ ] SSE disconnect triggers a "disconnected" state in the app
- [ ] Banner is visible, non-dismissible, and covers the terminal area
- [ ] Includes the text and a visual indicator (icon or color)
- [ ] If SSE reconnects (e.g., brief network blip), banner disappears

**Verification:**
- [ ] `pnpm exec tsc --noEmit`
- [ ] Manual: kill the Go server → banner appears in browser

**Dependencies:** Tasks 1-4 (host infrastructure)

**Files likely touched:**
- `cmd/vortex-ui/web/app.ts` (SSE error handling, disconnected state, banner render)

**Estimated scope:** Small (1 file)

---

### Task 7: Hide job-related UI when no orchestrator

**Description:** When the server has no orchestrator (bare mode), the frontend should not show job groups, the config preview button, or job-related controls. The API returns empty terminal list and no config — the UI should respond by only showing the Shell group.

**Acceptance criteria:**
- [ ] In bare mode, group bar only shows "Shell" (no other groups)
- [ ] Config preview button is hidden when API returns no config
- [ ] Tab bar only shows shell tabs
- [ ] No "empty state" flicker — Shell group is active by default

**Verification:**
- [ ] `pnpm exec tsc --noEmit`
- [ ] Manual: start bare mode → UI shows only shells

**Dependencies:** Task 6

**Files likely touched:**
- `cmd/vortex-ui/web/app.ts` (conditional rendering based on terminal list)

**Estimated scope:** Small (1 file)

---

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Orphan host processes confuse users | Medium | `vortex stop` + `vortex instance list` for discoverability; clean PID tracking |
| Port 7370 conflict with other software | Low | Allow `--port` override; document the port |
| Nil orchestrator panics in untested code paths | High | Comprehensive nil-guards + test with nil orch in server_test.go |
| Window reconnect loses xterm.js state (cursor, alternate screen) | Medium | Ring buffer replay handles this; test with vim/htop sessions |

## Open Questions
- Should bare mode auto-create one shell tab on first start, or show an empty shell group with the "+" button?
- Should `vortex stop` also work as `vortex quit` alias for discoverability?
