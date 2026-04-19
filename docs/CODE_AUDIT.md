# Code Audit — Vortex

**Scope:** Full codebase review (Go backend + TypeScript frontend, Windows + Darwin)  
**Audit passes:** 13 (2026-04-18 through 2026-04-19)

---

## Fixed Issues (100)

### Security

| # | Issue | Files |
|---|-------|-------|
| 4 | **No HTTP API auth** — Added session token auth (crypto/rand 64-char hex). Server middleware validates `Authorization: Bearer` or `token` query param. | `instance.go`, `server.go`, `main.go`, `app.ts`, `vortex-terminal.ts` |
| 5 | **`handleOpenPath` path traversal** — Validates resolved path is within working dir or home dir. | `server.go` |
| 6 | **No handoff auth** — `ServeHandoff()` requires matching token in payloads. | `instance.go` |
| 24 | **Predictable token fallback** — Removed weak `time+pid` fallback. Panics if `crypto/rand` fails. | `instance.go` |
| 25 | **Settings file world-readable** — Uses `0o600` file / `0o700` dir. | `settings.go` |
| 30 | **Node runtime cache file permissions** — Tightened to `0o700` dir / `0o600` files. | `node_runtime.go` |
| 33 | **Timing-unsafe token comparison (HTTP)** — Uses `crypto/subtle.ConstantTimeCompare`. | `server.go` |
| 52 | **Timing-unsafe token comparison (handoff)** — Same fix applied to `ServeHandoff`. | `instance.go` |
| 79 | **`resolveOpenPathTarget` symlink bypass** — Symlinks inside allowed dirs could point outside. Added `filepath.EvalSymlinks` for target, workDir, and homeDir before prefix check. | `server.go` |
| 84 | **Instance registry dir world-readable** — Changed `os.MkdirAll` from `0o755` to `0o700`, consistent with settings and node runtime dirs. | `instance.go` |
| 70 | **`ListMetadata` leaks tokens** — Returned full `Metadata` including `Token` for every instance. Now strips `Token` before returning. | `instance.go` |
| 76 | **`readTokenForInstance` lockless** — Called `GetMetadataLocked` without holding `registryMu`. Changed to use `GetMetadata` which acquires the lock. | `instance.go` |

### Bugs — Process Lifecycle

| # | Issue | Files |
|---|-------|-------|
| 1 | **Fork `append` slice corruption** — Copy `os.Args` before appending `--forked`. | `fork_unix.go`, `fork_windows.go` |
| 10 | **`Kill()` double-kill race** — `sync.Once`-guarded `doKill()`. | `terminal.go` |
| 15 | **No graceful shutdown** — Two-phase: SIGTERM then SIGKILL after 5s. | `terminal.go`, `kill_other.go`, `kill_windows.go`, `orchestrator.go` |
| 27 | **`killProcessTree` returns stale ESRCH** — Fallback path returns actual error. | `kill_other.go` |
| 34 | **Context leak on `startChildProcess` failure** — Added `cancel()` before error return. | `terminal.go` |
| 35 | **Missing `setChildFlags` on Unix (Critical)** — Without `Setpgid`, `kill(-pid)` orphaned grandchildren. | `process_other.go` |
| 36 | **Windows process handle leak in `Wait()`** — Replaced per-path `CloseHandle` with `defer`. | `process_windows.go` |
| 37 | **`stopRunningInstance` false timeout** — Replaced time check with `stopped` boolean. | `upgrade.go` |
| 28 | **Temp script in system temp dir** — Changed `os.CreateTemp("", ...)` to `os.CreateTemp(filepath.Dir(dst), ...)` so the upgrade script is created in the user-owned install directory instead of the system temp dir. | `upgrade.go` |
| 45 | **Shell profile injection in `ensureUnixPath`** — Added validation guard rejecting paths with shell metacharacters (`"'\`$;&|\n`). Defensive against future callers even though current input is always `~/.local/bin`. | `upgrade.go` |
| 38 | **Zombie processes from `cmd.Start()`** — Added `startAndReap()` helper. | `open_external.go` |
| 49 | **`exec.CommandContext` races with kill goroutine** — Go's kill targets PID only, not process group. Switched to `exec.Command`. | `terminal.go` |
| 50 | **`Resize()` missing `exited` guard** — Could write to closed PTY/ConPTY. Added guard + nil closures in `drain()`. | `terminal.go` |
| 42 | **Subscription cancel/drain race** — Simplified `cancel()` closure: removes channel from `t.subs` and closes under lock without draining. Eliminates window where both `cancel()` and `drain()` race to close the same channel. | `terminal.go` |
| 43 | **`WriteInput`/`Resize` TOCTOU race** — Hold `RLock` across closure call (not just snapshot). Prevents `drain()` from closing the PTY fd between lock release and closure invocation. | `terminal.go` |
| 69 | **`Manager.Start()` orphans old terminals** — Added `old.Kill()` before `seedFrom` when replacing an existing terminal. Ensures old process and goroutines are cleaned up. | `terminal.go` |

### Bugs — Data Integrity & Concurrency

| # | Issue | Files |
|---|-------|-------|
| 2 | **No dependency cycle detection** — Added DFS-based `detectCycle()` with 4 tests. | `config.go`, `config_test.go` |
| 11 | **Subscriber channel double-close** — Cancel wrapped in `sync.Once`. | `terminal.go` |
| 22 | **`handleKillProcesses` returns 200 on error** — Returns 500 via `writeJSONStatus()`. | `server.go` |
| 23 | **Persistent job spec stale on restart** — `Restart()` updates `old.Spec = spec`. | `orchestrator.go` |
| 62 | **Race on `Job.Spec` during `Restart`** — Made `Job.Spec` private with mutex-protected `Spec()` getter and `updateSpec()` setter. `runJob` snapshots spec once at top. All callers (incl. `server.go`) migrated. | `orchestrator.go`, `server.go` |
| 63 | **Double-close panic on persistent job channels** — Added `sync.Once`-guarded `closeStarted()`/`closeDone()` helpers. `shouldCarryPersistentJob` rejects exited jobs (`Status() != StatusRunning`). | `orchestrator.go`, `orchestrator_test.go` |
| 64 | **`AddAndStart` duplicate IDs** — Added existence check under lock. Rejects duplicate job IDs with a log warning instead of silently overwriting. | `orchestrator.go` |
| 65 | **`Shutdown` skips pending jobs** — Shutdown now closes `started`/`done` channels on pending (no-terminal) jobs via `closeStarted()`/`closeDone()`, unblocking their `runJob` goroutines. | `orchestrator.go` |
| 29 | **No timeout for blocked dependencies** — Added 30-second diagnostic timer in `runJob` dependency wait loop. Logs which dependency is blocking without hard-killing it. | `orchestrator.go` |
| 39 | **`copyFile` no `Sync()` before close** — Added for self-upgrade crash safety. | `upgrade.go` |
| 41 | **`Rerun` doesn't increment gen** — SSE handlers missed rerun-triggered rebuilds. | `orchestrator.go` |
| 51 | **`GetMetadata` missing lock** — Public API now acquires `registryMu.Lock()`. | `instance.go` |
| 53 | **`postHandoff` doesn't drain response body** — Added `io.Copy(io.Discard)`. | `instance.go` |
| 54 | **SSE restart drops new job output** — Always use `OutputAndSubscribe()` on restart. | `server.go` |
| 55 | **`trimTrailingNumber` integer overflow** — Replaced with `strconv.Atoi`. | `server.go` |
| 56 | **`isLocalModuleSpecifier` misses Windows `.\` paths** — Added `filepath.ToSlash`. | `node_runtime.go` |
| 61 | **Non-atomic settings file write** — Changed to temp+sync+rename. | `settings.go` |
| 80 | **`detectCycle` parent-chain infinite loop** — Cycle reconstruction loop could spin forever if parent map had missing key. Added safety bound (`len(jobs)`) and empty-string check. | `config.go` |
| 81 | **`writeSSEChunk` json.Marshal error ignored** — Silently produced `data: null` SSE frames. Now logs error and skips the write. | `server.go` |
| 82 | **Concurrent node runtime `shared.mjs` writes** — Multiple job goroutines wrote `shared.mjs` simultaneously via non-atomic `os.WriteFile`. Changed to temp+rename atomic write. | `node_runtime.go` |
| 88 | **`atomicWriteFile` missing fsync** — Atomic write helper skipped `Sync()` before rename, risking truncated file on crash. Added explicit `f.Sync()` before close+rename. | `node_runtime.go` |
| 89 | **`writeMetadataFile` missing fsync** — Same class as #88. Used `os.WriteFile`+rename without sync. Changed to explicit Create/Write/Sync/Close/Rename pattern consistent with `settings.go` and `node_runtime.go`. | `instance.go` |
| 44 | **`CleanupInactiveMetadata` orphans on kill failure** — Added `log.Printf` warning with instance name, PID, and error when `KillProcessTreeByPID` fails. Metadata removal still proceeds (best-effort). | `instance.go` |
| 72 | **`resolveOSValue` duplicate key overwrite** — Added duplicate detection after case normalization. Returns clear error instead of silently picking nondeterministic winner. | `config.go` |

### Bugs — Frontend

| # | Issue | Files |
|---|-------|-------|
| 13/14 | **`setInterval` leak** — Store handle; clear in `disconnectedCallback()`. | `app.ts` |
| 31 | **`_reportSize` caches before success** — Moved assignment into success path. | `vortex-terminal.ts` |
| 32 | **`_clearTerminal` unwaited promise** — Added `void` operator. | `app.ts` |
| 40 | **Stale `_activeId` after restart** — Reset when ID no longer in list. | `app.ts` |
| 57 | **SSE `onmessage` uncaught exception** — `JSON.parse`/`atob` crash kills stream. Wrapped in try/catch. | `vortex-terminal.ts` |
| 58 | **`onerror` spams "[connection lost]"** — Dedup flag, cleared on message. | `vortex-terminal.ts` |
| 59 | **`_stopTerminal` floating promise** — Added `void` + `.catch()`. | `app.ts` |
| 60 | **Terminal modes leak across tabs** — Changed `clear()` to `reset()`. | `vortex-terminal.ts` |
| 74 | **`_rerunTab` ignores non-OK HTTP** — Added `res.ok` check; returns early on 4xx/5xx. | `app.ts` |
| 83 | **`_selectGroup` stale `activeId`** — Switching to an empty group left `_activeId` pointing at a tab in the old group. Now always resets. | `app.ts` |
| 75 | **`_fetchTerminals` poll race** — Added `_fetchSeq` counter. Each `_fetchTerminals` increments and captures the sequence; stale responses (where seq !== current) are discarded before applying. | `app.ts` |

### Design Improvements

| # | Issue | Files |
|---|-------|-------|
| 3 | **No CORS for dev mode** — Added CORS middleware (dev mode only). | `server.go` |
| 7 | **Port collision undetected** — `NewIdentity()` warns on hash collisions. | `instance.go` |
| 9 | **Windows `CREATE_NO_WINDOW` conflicts with ConPTY** — Removed flag. | `procattr_windows.go` |
| 26 | **No port range validation** — Validates `1 <= port <= 65535` early. | `cobra.go`, `main.go` |
| 12 | **`main.go` 960+ lines** — Extracted into `ui_lifecycle.go` (142 lines), `handoff.go` (105 lines), `instance_cmds.go` (458 lines). Core `main.go` reduced to 391 lines. | `main.go`, `ui_lifecycle.go`, `handoff.go`, `instance_cmds.go` |
| 85 | **Dead `/handoff` route on HTTP server** — `POST /handoff` was registered on the main HTTP server but `onHandoff` was always nil. Removed route, `HandoffHandler` type, and `onHandoff` field. Actual handoff served via `instance.ServeHandoff` on lock listener. | `server.go`, `main.go` |
| 87 | **SSE missing proxy-safe header** — Added `X-Accel-Buffering: no` to SSE endpoint to prevent nginx/proxy response buffering. | `server.go` |
| 90 | **Dead `/handoff` proxy in vite.config.js** — Removed stale proxy entry for `/handoff` that pointed to the Go server after #85 removed the HTTP route. | `vite.config.js` |
| 66 | **Darwin: select loop blocked by webview** — Extracted `eventLoop()` function. On Darwin, event loop runs in goroutine before blocking `ui.Open()`. Show-ui handoffs now processed while window is open. | `main.go` |
| 67 | **Forked child discards all log output** — Added `openLogFile()` helper. Forked child redirects `log.SetOutput()` to `<cacheDir>/vortex/logs/<name>.log` (truncated each launch). | `main.go` |
| 68 | **Webview goroutine leak on window close** — Added `runDone` channel closed after `w.Run()` returns. Context-watcher and overlay goroutines select on `<-runDone` so they exit when the window closes, preventing goroutine leaks and use-after-free. | `native_open.go`, `native_open_windows.go` |
| 78 | **`uiThreadRunner.Post` deadlock** — Two-phase send: attempt non-blocking send under lock, fall back to blocking send without lock. Prevents mutex from being held across a blocking channel send. | `ui_thread_other.go` |
| 46 | **Node runtime wrapper file collision** — `sanitizeFileComponent` now maps `/` and `\` to `--` instead of `_`, so `build/prod` → `build--prod.mjs` no longer collides with `build_prod`. | `node_runtime.go` |
| 47 | **Webview silent init failure** — Added `log.Printf` when `webviewlib.New` / `NewWindow` returns nil so users get feedback when the native window fails to initialize. | `native_open.go`, `native_open_windows.go` |
| 48 | **`serveEmbedded` opens file twice** — Replaced `fsys.Open` probe-then-close with `fs.Stat` to check existence, eliminating the double-open and TOCTOU window. | `server.go` |
| 71 | **`NewIdentity` full registry scan per call** — Moved the port collision warning from `NewIdentity` (called per HTTP request) into `Register` (called once at startup). `NewIdentity` is now pure computation with no I/O. | `instance.go` |
| 73 | **No SSE heartbeat** — Added a 15-second `time.Ticker` to the SSE streaming loop that writes `: keepalive` comments, preventing idle connection drops by proxies and browsers. | `server.go` |
| 77 | **Unbounded GitHub API JSON decode** — Wrapped `resp.Body` in `io.LimitReader(resp.Body, 1<<20)` before JSON decoding, capping the success-path read to 1 MB. | `upgrade.go` |
| 91 | **`uiThreadRunner.Post` send-on-closed-channel panic** — After the non-blocking send fails and the mutex is released, `Close()` could fire, closing the channel before the fallback blocking send. Added `defer recover()` around the unlocked send. | `ui_thread_other.go` |
| 92 | **`sanitizeFileComponent` residual collision** — Fix #46 mapped `/` → `--` but `a/b` and `a--b` still collided. Appended a short FNV hash suffix to the wrapper filename so every distinct job ID gets a unique file. | `node_runtime.go` |
| 93 | **Darwin: `orch.Shutdown()` comment clarified** — Retained synchronous `orch.Shutdown()` on Darwin (needed because `return nil` follows immediately). Improved comment explaining why both calls exist. | `main.go` |
| 94 | **Dead `openExternalURL` wrapper** — Removed unused unexported function that simply called the public `OpenExternalURL`. | `open_external.go` |
| 96 | **Instance CLI JSON responses not body-size-limited** — Added `io.LimitReader(resp.Body, 1<<20)` to both `killInstanceProcesses` and `fetchInstanceTerminals`, consistent with the #77 pattern. | `instance_cmds.go` |
| 99 | **Stale `_activeGroup` after group removal** — Added reset: when the current active group no longer exists in the terminal list, falls back to the first terminal's group. | `app.ts` |
| 100 | **Unnamed-group terminals inaccessible** — Group bar now shows all groups including unnamed (displayed as "(default)"). Changed `_showGroupBar` to trigger on 2+ distinct groups regardless of naming. | `app.ts` |
| 106 | **Darwin graceful shutdown race (caught regression)** — Pass 10 found that removing `orch.Shutdown()` (#93) left child processes orphaned on Darwin. Reverted: kept the synchronous call with an improved comment. | `main.go` |
| 107 | **`hashString` missing zero-padding** — `fmt.Sprintf("%x")` could produce <16 chars for small hashes, making `[:8]` slice panic. Changed to `%016x`. | `node_runtime.go` |
| 109 | **`_activeGroup` initialization conflicts with selectable default group** — `_activeGroup === ''` was used both as "uninitialized" and "user selected default group". Added `_groupInitialized` flag to distinguish the two states. | `app.ts` |
| 110 | **CLI HTTP API calls unauthenticated on non-dev instances** — Fix #70 stripped tokens from `ListMetadata()`, but `killInstanceProcesses` and `fetchInstanceTerminals` used that token-stripped metadata. Changed `killInstanceProcesses` to use `GetMetadata`; `fetchInstanceTerminals` re-reads token when missing. | `instance_cmds.go` |
| 111 | **`atomicWriteFile` deterministic temp name races** — Used fixed `path + ".tmp"` suffix. Concurrent goroutines writing the same target could race on the temp file. Changed to `os.CreateTemp` with random suffix. | `node_runtime.go` |
| 112 | **Unhandled promise rejections in `_rerunTab`, `clearOutput`, `revealTerminalPath`** — Bare `await fetch(...)` called fire-and-forget via `void` produced `unhandledrejection` on network errors. Added `try/catch` consistent with other fetch callers. | `app.ts`, `vortex-terminal.ts` |
| 97 | **`Rerun` + `Restart` race on `o.cfg` generation** — `runJob` read `o.cfg` without lock. Added `cfg` field to `jobLaunch`; `resolveLaunchesLocked` snapshots `o.cfg` under lock and threads it to `runJob`. All call sites (`Start`, `Rerun`, `AddAndStart`) updated. | `orchestrator.go` |
| 108 | **`runJob` reads `o.cfg` without orchestrator lock** — Same fix as #97: `runJob` now receives `cfg` as a parameter instead of accessing `o.cfg`. | `orchestrator.go` |
| 98 | **SSE reconnection replays all output (duplicates)** — Added `onopen` handler to `EventSource` that calls `this._term?.reset()` before any data arrives. On reconnect, the terminal is cleared before the server replays the buffer. | `vortex-terminal.ts` |
| 103 | **`clearOutput()` ignores server response** — Now checks the DELETE response status. On failure, writes dim `[clear buffer failed]` message into the terminal. | `vortex-terminal.ts` |
| 104 | **No user feedback on toolbar action failure** — Added `writeStatus()` method to `VortexTerminal`. `_stopTerminal` and `_rerunTab` now write `[stop failed]` / `[rerun failed]` on error. Extracted `_activeTerminalEl()` helper. | `app.ts`, `vortex-terminal.ts` |

---

## Documented / Known (4)

| # | Issue | Notes |
|---|-------|-------|
| 8 | `registryMu` only protects in-process | By design: TCP `TryLock` ensures single-owner. Permissions hardened to `0o600`. |
| 16 | No config file watcher | Feature request. Reload currently handled via handoff (second `vortex run` POSTs new config). Deferred. |
| 19 | Weak stale process detection on Unix | `kill(pid, 0)` + 7-day cutoff. Acceptable for a dev tool. |
| 21 | Thin test coverage | 4 cycle-detection tests added. Auth/shutdown/subscriber tests need integration harness. |

---

## False Positive / N/A (3)

| # | Issue | Notes |
|---|-------|-------|
| 17 | `cleanupConsole` not wired on Windows | Already correct. |
| 18 | `console_attach.go` build tag | Platform-agnostic code. |
| 20 | `go.mod` specifies `go 1.25.5` | Valid version. |

---

## Dismissed Issues (5)

| # | Severity | Reason |
|---|----------|--------|
| 86 | Info | `resolveRevealPath` / `normalizeTerminalPath` — test-only thin wrappers around `parseTerminalPath`. Serve the test suite; harmless. |
| 95 | Info | Unreachable `len(args) > 0` in Cobra RunE — `cobra.NoArgs` rejects before `RunE`. Belt-and-suspenders; harmless. |
| 101 | Low | `_gen` counter tracked but unused — vestigial field, no impact. |
| 102 | Low | `closeStream()` never called — unused public method on `VortexTerminal`. Harmless API surface. |
| 105 | Info | `tsconfig.json` include lists only entry point — works today via transitive imports. |

---

## Open Issues (0)

All issues resolved.

---

## Totals

| Status | Count |
|--------|-------|
| Fixed | 100 |
| Documented / Known | 4 |
| False Positive / N/A | 3 |
| Dismissed | 5 |
| Open | 0 |
| **Total** | **112** |