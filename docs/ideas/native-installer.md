# Vortex Native Installer

## Problem Statement

How might we make Vortex launchable as a native application (Spotlight, Start Menu) on macOS and Windows so it can serve as a terminal replacement — without requiring code signing or developer program memberships?

## Recommended Direction

A Discord-style minimal installer distributed as `.dmg` (macOS) and `.exe` (Windows).

### macOS

The `.dmg` opens with a drag-to-Applications background (app icon + arrow + Applications alias). It contains `Vortex.app` directly — not a separate "installer" app.

On **first launch**, if the binaries aren't yet in `~/.local/bin/`, `Vortex.app` shows an install progress UI (branded, minimal — progress bar only) to download `vortex` and `vortex-window` from GitHub Releases, verify checksums, and place them. Subsequent launches go straight to the terminal.

The `.app` launcher uses a login shell invocation to inherit the user's full environment (PATH, homebrew, nvm, pyenv, etc.):

```bash
#!/bin/zsh --login
exec ~/.local/bin/vortex run --ui
```

### Windows

A standalone `Install Vortex.exe` with an embedded webview showing a branded progress screen. It:

- Downloads `vortex.exe` + `vortex-window.exe` from GitHub Releases
- Verifies checksums
- Places binaries in `%LOCALAPPDATA%\Programs\Vortex\`
- Creates a Start Menu shortcut
- Registers in Add/Remove Programs (HKCU uninstall registry)
- Places `uninstall.exe` alongside the binaries
- Offers to launch Vortex on completion

### Already Installed

If the installer detects an existing installation, it shows: "Vortex is already installed. Reinstall / Upgrade / Cancel."

### Uninstaller

Same binary as the installer, invoked with `--uninstall` (or as the copied `uninstall.exe` on Windows). Shows a small UI with a checkbox: "Also remove configuration and data." Removes:

- Binaries from install path
- `.app` bundle from `/Applications` (macOS) / Start Menu shortcut + registry (Windows)
- Optionally: `~/.config/vortex` / `%APPDATA%\Vortex` config and data

### Self-Updates

After installation, the app self-updates via the existing `vortex upgrade` mechanism. The installer is one-and-done. The `.app` wrapper and Windows shortcuts point to fixed paths that `vortex upgrade` replaces in-place.

## Key Assumptions to Validate

- [ ] **Users tolerate unsigned app warnings** — Ship to beta users, measure if anyone bounces at Gatekeeper/SmartScreen. macOS: right-click → Open. Windows: "More info" → "Run anyway."
- [ ] **`.app` bundle with login shell launcher works reliably** — Test Spotlight launch, double-click, `open -a Vortex` across macOS versions (13+)
- [ ] **Install path stability** — `~/.local/bin/vortex` and `%LOCALAPPDATA%\Programs\Vortex\vortex.exe` are the canonical paths. `vortex upgrade` must never change them.
- [ ] **DMG creation works in CI without signing** — `hdiutil create` on GitHub Actions macOS runner, unsigned.
- [ ] **First-launch bootstrap UX is acceptable** — Users dragging to Applications then waiting for a download on first launch isn't confusing.

## MVP Scope

**In:**

- macOS `.app` bundle with `Info.plist`, `vortex.icns`, login-shell launcher script
- macOS first-launch bootstrap: download progress UI → install binaries → launch
- macOS `.dmg` with drag-to-Applications background image
- Windows installer `.exe` with webview progress UI
- Windows Start Menu shortcut + Add/Remove Programs registration
- Uninstaller (same binary, `--uninstall` mode) with "remove config?" checkbox
- CI jobs to produce `.dmg` and Windows installer on release
- Detection of existing installation (reinstall/upgrade/cancel)

**Out (for now):**

- Silent/CLI install mode (add later via flags)
- Custom install location picker
- Linux packaging (different problem, different solution)
- Auto-launch on login
- Homebrew Cask / WinGet manifests

## Not Doing (and Why)

- **Code signing / notarization** — Requires Apple Developer Program ($99/yr) + Windows EV cert. Not worth it at current scale. Users accept the click-through for developer tools.
- **macOS .pkg installer** — Unsigned `.pkg` gets *harder* Gatekeeper treatment than unsigned `.app` in a `.dmg`. Counterproductive.
- **Multi-step wizard** — Discord proves one-click install works. Options add support surface without adding value.
- **Homebrew Cask / WinGet** — Good future distribution channel but doesn't solve "launchable from Spotlight/Start Menu" on its own. Add later.
- **Electron-based installer** — Would introduce a second runtime. Go+webview is already proven in this codebase.
- **Linux** — Different ecosystem (`.desktop` files, XDG, multiple distros). Separate effort.

## Open Questions

- What icon to use for the `.app` and `.dmg`? (Need a `.icns` file for macOS, `.ico` for Windows)
- Should `vortex upgrade` also update the `.app` bundle's `Info.plist` version string?
- What's the minimum macOS version to target? (Impacts webview capabilities in the bootstrap UI)
- Should the Windows installer require admin? (Leaning no — per-user install to `%LOCALAPPDATA%` avoids UAC)
