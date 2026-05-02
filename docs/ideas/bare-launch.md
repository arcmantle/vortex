# Standalone Terminal Mode (Bare Launch)

## Problem Statement
How might we let users launch Vortex as a persistent terminal manager
with zero configuration — so it functions as a daily-driver terminal app
that survives window closes?

## Recommended Direction
**Thin Daemon / Singleton Host.**

`vortex` with no arguments starts (or reconnects to) a single persistent
host process per user. The host keeps shell PTYs alive independently of
the window. Closing the window detaches; reopening reconnects with full
scrollback replay. The `.vortex` config system becomes opt-in via
`vortex run <config>`.

This works because:
1. The architecture already separates `vortex` (host) from `vortex-window` (view).
2. The terminal ring buffer + `OutputAndSubscribe()` already replays history
   on new SSE connections — reconnect gets scrollback for free.
3. The named-instance lock system can trivially support a fixed "singleton"
   instance for bare-launch mode.
4. The shell group (profiles, tabs, picker) is already fully built.

## Key Decisions
- **Port:** 7370 for the singleton; config instances keep their deterministic port derivation.
- **Config instances surviving:** Yes — all hosts survive window close. Unified "detach/reconnect" model everywhere.
- **Crash recovery:** Window detects host gone (SSE drops), shows "host disconnected" banner. User must manually run `vortex` / `vortex run` again.

## Key Assumptions to Validate
- [ ] Users accept a background process — needs clean discoverability
      (validate: `vortex status` command + tray icon on macOS?)
- [ ] 4MB ring buffer is sufficient scrollback for daily use
      (validate: measure typical terminal session output over 8hrs)
- [ ] Single singleton instance per user is the right model
      (validate: does anyone need multiple independent bare sessions?)

## MVP Scope

**In:**
- `vortex` (no args) → start/connect singleton host, open window with
  default shell tab
- All hosts stay alive on window close (unified detach model)
- `vortex` again → find running host via instance lock, open new window
  connected to same host
- SSE reconnect replays scrollback (already works)
- `vortex stop` → kill the singleton host explicitly
- Server runs with nil orchestrator (shells-only mode)

**Out (for now):**
- Attaching/detaching .vortex configs at runtime
- CWD-derived instance names / multiple bare sessions
- Tray icon / OS-level integration
- Session resume across host restarts (cold persistence)

## Not Doing (and Why)
- **Config-as-overlay (attach at runtime)** — Great idea but separate feature;
  bare launch should ship independently without coupling to it.
- **CWD workspaces** — Adds naming complexity. Singleton is simpler to reason
  about and covers the daily-driver use case.
- **Cold persistence (survive reboot)** — PTY state can't truly serialize.
  tmux does this with session files but it's lossy. Not worth the complexity.
- **Tray icon** — Platform-specific UI work that can follow later once the
  daemon model is validated.

## Open Questions
- Should `vortex stop` kill only the singleton, or accept a name to stop any instance?
- Do we need a `vortex list` to show all running instances (singleton + named)?
