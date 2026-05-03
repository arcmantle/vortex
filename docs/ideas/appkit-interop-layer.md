# AppKit Interop Layer + Persistent Window

## Problem Statement

How might we give `vortex-window` native macOS app behavior (hide-on-close, dock persistence, Cmd+Q) through a clean Go interop layer that cooperates with the webview library's run loop — without disrupting the existing architecture on Windows/Linux?

## Recommended Direction

Build a standalone `internal/appkit/` package that provides a Go event-channel API over Objective-C AppKit lifecycle delegates. On macOS, `vortex-window` becomes persistent — it hides its window on red X instead of exiting, stays alive for dock reopen, and handles Cmd+Q gracefully. On other platforms, behavior is unchanged.

The key insight is that macOS dock-click reopen (`applicationShouldHandleReopen:`) only fires if the process is still running. Therefore, the window process *must* stay alive when hidden — making "persistent window" a prerequisite for proper macOS behavior, not just a nice-to-have.

The parent↔child protocol gains two new messages (`HIDDEN`, `SHOW`) and the parent's `ui_lifecycle.go` learns a new state (`hidden` — child alive, window not visible). This is entirely darwin-gated; Windows and Linux continue with the current spawn/exit pattern.

## Key Assumptions to Validate

- [ ] We can set `NSWindowDelegate` on webview's window between `webview.New()` and `w.Run()` — if the library sets its own delegate internally, we'll need method swizzling or delegate chaining. Test by inspecting `[mainWindow delegate]` after `New()`.
- [ ] The webview library's `w.Run()` run loop processes our dispatched show/hide messages correctly — validate that `dispatch_async(dispatch_get_main_queue(), ...)` works from Go goroutines while `w.Run()` is blocking.
- [ ] Hiding the window (orderOut) without calling `w.Terminate()` doesn't cause the webview to deallocate or stop responding — test that a hidden window can be shown again with `makeKeyAndOrderFront:`.

## MVP Scope

**In:**
- `internal/appkit/` package with darwin `.m` + Go wrapper, no-op on other platforms
- Red X hides window instead of closing (emits `WindowHidden` event)
- Dock click unhides window (emits `ReopenRequest` event)
- Cmd+Q emits `QuitRequest` event (lets parent decide)
- `applicationShouldTerminateAfterLastWindowClosed` returns NO
- `vortex-window` handles `SHOW` command (unhide) and emits `HIDDEN` (on hide)
- `ui_lifecycle.go` handles `hidden` state — sends `SHOW` instead of spawning a new process

**Out (future):**
- Custom menu bar items (File, Edit, View, Help)
- Dock badges or progress indicators
- Touch Bar integration
- System notifications from the window process
- Windows system tray integration (separate effort)

## Not Doing (and Why)

- **Custom application menus** — The lifecycle behavior is the immediate user pain; menus are additive and can be layered on later using the same `internal/appkit/` infrastructure.
- **Windows minimize-to-tray** — Different UX expectation on Windows (apps close when you close them). Can be explored independently.
- **Replacing the webview library** — Working *with* webview's run loop is simpler and lower risk than replacing it with something that natively supports lifecycle hooks.
- **Making vortex-window the "main" application** — The console-subsystem orchestrator (`vortex`) must remain the parent process for Windows PTY/ConPTY reasons. The window is still a child.
- **darwinkit/macdriver dependency** — Adds complexity coordinating with webview's run loop. A thin ObjC shim is more predictable.

## Open Questions

- Does `webview_go` already set an `NSWindowDelegate` on its window? If so, we need to chain delegates rather than replace.
- Should `Cmd+Q` quit just the window (hide) or signal the parent to shut down entirely? Current thinking: signal the parent via `CLOSE`, let the parent decide based on its own quit policy.
- When the window is hidden and the parent wants the child to fully exit (e.g., server shutting down), should we add an `EXIT` command distinct from `CLOSE`? Or is closing stdin (EOF) sufficient?

---

# Implementation Plan

## Overview

Add macOS-native lifecycle behavior to `vortex-window` through a new `internal/appkit/` interop package and protocol extensions. The window hides on red X, the app stays in the dock, dock-click unhides, Cmd+Q signals the parent.

## Architecture Decisions

- **Standalone `internal/appkit/` package** — not coupled to `internal/webview/`. The webview package doesn't need to know about lifecycle; the consumer (`vortex-window/main.go`) coordinates.
- **Channel-based event delivery** — Go-idiomatic, composable, no callback registration order issues.
- **Darwin-only via build tags** — `_darwin.go` / `_darwin.m` files. Other platforms get no-op stubs that return a closed channel.
- **Protocol extension, not replacement** — `HIDDEN` and `SHOW` are additive; old parent/child pairs remain compatible (unknown messages are ignored).

## Task List

### Phase 1: AppKit Interop Package

#### Task 1: Create `internal/appkit/` with types and no-op stubs

**Description:** Define the public API — event types, config struct, `Configure()` function. Create the `_other.go` stub that returns a closed channel on non-darwin platforms. This establishes the interface before any ObjC work.

**Acceptance criteria:**
- [ ] `internal/appkit/appkit.go` defines `Event` type, `Config` struct, event constants (`WindowHidden`, `ReopenRequest`, `QuitRequest`)
- [ ] `internal/appkit/appkit_other.go` (build tag `!darwin`) returns a closed `<-chan Event`
- [ ] Package compiles on all platforms (`go build ./internal/appkit/`)

**Verification:**
- [ ] `go build ./...` passes
- [ ] `go vet ./internal/appkit/`

**Dependencies:** None

**Files likely touched:**
- `internal/appkit/appkit.go`
- `internal/appkit/appkit_other.go`

**Estimated scope:** Small (2 files)

---

#### Task 2: Implement darwin AppKit delegates in Objective-C

**Description:** Write the `.m` file that installs an `NSApplicationDelegate` and `NSWindowDelegate` to intercept lifecycle events. Communicate back to Go via C function callbacks. Must be called *after* `webview.New()` creates the NSApplication but *before* `w.Run()` starts the run loop.

**Acceptance criteria:**
- [ ] `internal/appkit/appkit_darwin.m` implements:
  - `applicationShouldTerminateAfterLastWindowClosed:` → returns NO
  - `windowShouldClose:` → hides window, calls Go callback, returns NO
  - `applicationShouldHandleReopen:hasVisibleWindows:` → shows window, calls Go callback
  - `applicationShouldTerminate:` → calls Go callback
- [ ] `internal/appkit/appkit_darwin.go` has CGO directives linking Cocoa, exports Go callbacks
- [ ] `Configure()` installs delegates and returns event channel

**Verification:**
- [ ] `CGO_ENABLED=1 go build ./internal/appkit/` compiles on macOS
- [ ] Manual test: small main.go that creates a webview, configures appkit, prints events

**Dependencies:** Task 1

**Files likely touched:**
- `internal/appkit/appkit_darwin.go`
- `internal/appkit/appkit_darwin.m`

**Estimated scope:** Medium (2 files, ~100 lines ObjC + ~60 lines Go)

---

### Checkpoint: AppKit Package
- [ ] `go build ./...` passes on macOS and Linux
- [ ] Manual test confirms hide-on-close and dock-reopen emit events

---

### Phase 2: vortex-window Integration

#### Task 3: Wire `internal/appkit/` into `vortex-window` (darwin)

**Description:** On darwin, `vortex-window` calls `appkit.Configure()` before the webview runs, then translates appkit events into stdout messages for the parent. Add handling for the new `SHOW` stdin command to unhide the window.

**Acceptance criteria:**
- [ ] On darwin: red X hides window, sends `HIDDEN\n` to stdout (instead of exiting)
- [ ] On darwin: `SHOW` command from stdin unhides the window, sends `READY\n`
- [ ] On darwin: Cmd+Q sends `CLOSE\n` to stdout (existing behavior, but now explicit)
- [ ] On non-darwin: behavior is identical to today (exit on close)
- [ ] `CLOSE` command still terminates the process (full exit)
- [ ] EOF on stdin still terminates the process

**Verification:**
- [ ] `go build ./cmd/vortex-window/` on all platforms
- [ ] Manual test on macOS: close window → process stays alive → send SHOW → window reappears

**Dependencies:** Task 2

**Files likely touched:**
- `cmd/vortex-window/main.go`
- `cmd/vortex-window/lifecycle_darwin.go` (new, darwin-specific init)
- `cmd/vortex-window/lifecycle_other.go` (new, no-op)

**Estimated scope:** Medium (3 files)

---

#### Task 4: Extend `Controller` interface with `Hide()` and `Show()`

**Description:** The `webview.Controller` interface needs `Hide()` and `Show()` methods so `vortex-window` can programmatically hide/show the window (for the `SHOW` stdin command). Implement via `w.Dispatch()` calling `orderOut:`/`makeKeyAndOrderFront:`.

**Acceptance criteria:**
- [ ] `Controller` interface gains `Hide()` and `Show()` methods
- [ ] `nativeController` implements them using webview Dispatch + windowfocus calls
- [ ] Calling `Show()` on a hidden window makes it visible and focused

**Verification:**
- [ ] `go build ./...`
- [ ] Manual test: hide → show cycle works

**Dependencies:** Task 1 (interface), Task 3 (consumer)

**Files likely touched:**
- `internal/webview/webview.go`
- `internal/webview/native_controller.go`
- `windowfocus/focus_darwin.go` (may need `showWindow` / `hideWindow` helpers)
- `windowfocus/focus.go` (interface additions)

**Estimated scope:** Small (3-4 files, minimal changes per file)

---

### Checkpoint: Window Persistence
- [ ] On macOS: red X hides, dock click unhides, Cmd+Q closes
- [ ] On other platforms: behavior unchanged
- [ ] `go build ./...` clean on all platforms

---

### Phase 3: Parent Protocol Update

#### Task 5: Teach `ui_lifecycle.go` about the `hidden` state

**Description:** The parent process reads stdout from `vortex-window`. When it sees `HIDDEN`, it marks the UI as hidden (child alive, window not visible). When asked to show the UI and state is `hidden`, it sends `SHOW` instead of spawning a new process. Also handle the case where the hidden child exits unexpectedly.

**Acceptance criteria:**
- [ ] `uiLifecycle` has a `hidden` state (child alive, window not visible)
- [ ] Receiving `HIDDEN\n` on child stdout → state transitions to `hidden`
- [ ] `Open()` when state is `hidden` → sends `SHOW\n` to existing child stdin, returns true
- [ ] `Focus()` when state is `hidden` → sends `SHOW\n` (same as Open in hidden state)
- [ ] Child exit while in `hidden` state → clean up, transition to closed
- [ ] Instance registry reflects `"hidden"` state

**Verification:**
- [ ] Full end-to-end test: `vortex run` → close window → `vortex instance show-ui` → window reappears without new process
- [ ] Process list confirms single `vortex-window` process throughout

**Dependencies:** Task 3

**Files likely touched:**
- `cmd/vortex/ui_lifecycle.go`

**Estimated scope:** Medium (1 file, significant logic changes)

---

#### Task 6: Read child stdout continuously (protocol change)

**Description:** Currently the parent reads stdout only until `READY`, then ignores it. For the `HIDDEN` message to be received, the parent must continue reading stdout for the lifetime of the child process. Refactor `runWindowProcess` to scan stdout continuously in a goroutine.

**Acceptance criteria:**
- [ ] Stdout is read in a goroutine for the child's entire lifetime
- [ ] `READY` is still detected (first message)
- [ ] `HIDDEN` is detected and handled (calls into uiLifecycle state transition)
- [ ] Unknown messages on stdout are logged but not fatal
- [ ] Child exit still detected correctly (cmd.Wait)

**Verification:**
- [ ] Existing behavior unchanged when child sends only `READY` then exits
- [ ] New behavior works when child sends `HIDDEN` after being open

**Dependencies:** Task 5 (state machine), but can be implemented first as groundwork

**Files likely touched:**
- `cmd/vortex/ui_lifecycle.go`

**Estimated scope:** Small-Medium (1 file, refactor stdout reading)

---

### Checkpoint: Full Integration
- [ ] `go build ./...` passes on all platforms
- [ ] `go vet ./...` clean
- [ ] macOS: full hide/show/quit lifecycle works end-to-end
- [ ] Windows/Linux: no behavioral changes
- [ ] `vortex instance show-ui` works on hidden window without spawning new process

---

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| webview_go sets its own NSWindowDelegate | High — our delegate gets overwritten | Inspect at runtime; if set, chain delegates (forward unknown selectors) |
| Hidden window's WKWebView stops updating | Medium — stale content on show | Navigate to current URL on show, or use `[webView reload]` |
| Process stays alive consuming memory | Low — webview is lightweight when hidden | Monitor RSS; if problematic, can terminate webview content while hidden |
| Protocol version mismatch (old parent + new child) | Low — additive messages | Unknown stdout lines are logged, not fatal; SHOW is ignored if not recognized |

## Dependency Graph

```
Task 1 (types + stubs)
    │
    ├── Task 2 (ObjC delegates)
    │       │
    │       └── Task 3 (wire into vortex-window)
    │               │
    │               └── Task 5 (hidden state in parent)
    │
    └── Task 4 (Controller Hide/Show)
            │
            └── Task 3 (uses Hide/Show)

Task 6 (continuous stdout reading) ← groundwork for Task 5
```

**Recommended order:** 1 → 2 → 4 → 6 → 3 → 5
