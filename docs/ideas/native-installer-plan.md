# Implementation Plan: Vortex Native Installer

## Overview

Build a native installer experience for macOS and Windows that makes Vortex launchable from Spotlight/Start Menu. macOS ships as a `.dmg` with drag-to-Applications containing a `Vortex.app` bundle that self-bootstraps on first launch. Windows ships as a standalone installer `.exe` with a webview progress UI. Both platforms get an uninstaller (same binary, `--uninstall` mode).

## Architecture Decisions

- **Single installer binary** serves as both installer and uninstaller via `--uninstall` flag
- **macOS `.app` bundle uses a login-shell launcher** to inherit user's full environment (PATH, homebrew, nvm, etc.)
- **First-launch bootstrap on macOS** — the `.app` itself downloads binaries on first run, rather than a separate installer app
- **Windows installer is a Go webview app** reusing the existing `internal/webview` package
- **Install paths unchanged** — `~/.local/bin/` (macOS) and `%LOCALAPPDATA%\Programs\Vortex\` (Windows) remain canonical
- **No code signing** — users accept Gatekeeper/SmartScreen click-through
- **Existing `internal/release` package** is reused for download, checksum, and path management logic

## Task List

### Phase 1: macOS App Bundle (Foundation)

- [ ] Task 1: Create `.app` bundle structure and launcher script
- [ ] Task 2: Generate `vortex.icns` from existing `assets/icon.svg`
- [ ] Task 3: First-launch bootstrap logic in Go

### Checkpoint: macOS App Works
- [ ] Can drag `Vortex.app` to `/Applications`
- [ ] Spotlight finds "Vortex"
- [ ] First launch shows progress → installs binaries → launches terminal UI
- [ ] Subsequent launches go straight to terminal UI
- [ ] Full user env (PATH, etc.) available in spawned terminal sessions

### Phase 2: macOS DMG Packaging

- [ ] Task 4: DMG creation script with drag-to-Applications background

### Checkpoint: macOS Distribution Ready
- [ ] `.dmg` opens with app icon + arrow + Applications alias
- [ ] Drag-install → launch → full flow works end-to-end

### Phase 3: Windows Installer

- [ ] Task 5: Windows installer binary with webview progress UI
- [ ] Task 6: Start Menu shortcut + Add/Remove Programs registration

### Checkpoint: Windows Works
- [ ] Running installer downloads binaries, creates shortcuts, registers uninstall
- [ ] Start Menu search finds "Vortex"
- [ ] Launch from Start Menu opens the terminal UI

### Phase 4: Uninstaller (Both Platforms)

- [ ] Task 7: Uninstall mode with "remove config?" UI

### Checkpoint: Uninstall Works
- [ ] macOS: removes `.app` from `/Applications`, binaries from `~/.local/bin/`
- [ ] Windows: removes binaries, shortcut, registry entry
- [ ] "Also remove config" checkbox removes `~/.config/vortex` / `%APPDATA%\Vortex`
- [ ] Windows: accessible from Add/Remove Programs

### Phase 5: Already-Installed Detection

- [ ] Task 8: Detect existing installation, offer reinstall/upgrade/cancel

### Phase 6: CI Integration

- [ ] Task 9: Add DMG creation to release workflow (macOS)
- [ ] Task 10: Add installer `.exe` to release workflow (Windows)

### Checkpoint: Complete
- [ ] Release workflow produces `.dmg` + Windows installer `.exe`
- [ ] All acceptance criteria met
- [ ] End-to-end flow tested on both platforms

---

## Task 1: Create `.app` bundle structure and launcher script

**Description:** Create the macOS `.app` bundle directory structure that the installer will place in `/Applications`. This includes `Info.plist`, the launcher script, and placeholder for the icon. The launcher uses `#!/bin/zsh --login` to inherit the user's shell environment before exec'ing the vortex binary.

**Acceptance criteria:**
- [ ] `Vortex.app/Contents/MacOS/vortex-launcher` is an executable script that runs `~/.local/bin/vortex run --ui` under a login shell
- [ ] `Vortex.app/Contents/Info.plist` contains correct bundle identifier, version, icon reference, and LSUIElement settings
- [ ] `Vortex.app/Contents/Resources/` directory exists for the icon
- [ ] Running `open Vortex.app` launches vortex (when binaries are already installed)

**Verification:**
- [ ] `plutil -lint Vortex.app/Contents/Info.plist` passes
- [ ] `open /Applications/Vortex.app` launches the terminal UI
- [ ] Spotlight indexes and finds "Vortex"

**Dependencies:** None

**Files likely touched:**
- New: `packaging/macos/Vortex.app/Contents/Info.plist`
- New: `packaging/macos/Vortex.app/Contents/MacOS/vortex-launcher`
- New: `packaging/macos/create-app-bundle.sh` (script to assemble the bundle from template)

**Estimated scope:** Small (2-3 files)

---

## Task 2: Generate `vortex.icns` from existing icon

**Description:** Convert the existing `assets/icon.svg` to macOS `.icns` format (multiple resolutions) and Windows `.ico` format. These will be embedded in the `.app` bundle and the Windows installer respectively.

**Acceptance criteria:**
- [ ] `vortex.icns` exists with standard sizes (16, 32, 128, 256, 512, 1024px)
- [ ] `vortex.ico` exists with standard Windows sizes (16, 32, 48, 256px)
- [ ] A script or makefile target can regenerate these from the SVG

**Verification:**
- [ ] `.app` bundle shows the icon in Finder and Dock
- [ ] `iconutil --convert icns` validates the iconset

**Dependencies:** None (parallel with Task 1)

**Files likely touched:**
- New: `packaging/icons/vortex.icns`
- New: `packaging/icons/vortex.ico`
- New: `scripts/generate-icons.sh`

**Estimated scope:** Small (script + generated files)

---

## Task 3: First-launch bootstrap logic in Go

**Description:** Add logic to the `vortex` binary (or a small companion binary embedded in the `.app`) that detects whether binaries are installed at `~/.local/bin/`. If not, it shows a webview with a progress bar, downloads the release binaries (reusing `internal/release`), installs them, then restarts itself as the full app. This is the "first launch" experience when a user drags to Applications but hasn't run the old CLI installer.

**Acceptance criteria:**
- [ ] On first launch (no binary at `~/.local/bin/vortex`), shows a branded progress UI
- [ ] Downloads `vortex` + `vortex-window` from the GitHub release matching the app's embedded version
- [ ] Verifies checksums before placing binaries
- [ ] After install, launches the main terminal UI automatically
- [ ] On subsequent launches, skips bootstrap entirely (fast startup)
- [ ] If already installed, detects and goes straight to terminal UI

**Verification:**
- [ ] Remove `~/.local/bin/vortex`, launch app → see progress → binaries installed → terminal opens
- [ ] Launch again → terminal opens immediately (no bootstrap UI)

**Dependencies:** Task 1 (needs the `.app` bundle to launch from)

**Files likely touched:**
- New: `cmd/vortex/bootstrap.go` (or `cmd/vortex/bootstrap_darwin.go`)
- Modified: `cmd/vortex/main.go` or entry point to check for bootstrap condition
- Reuses: `internal/release` (download, checksum, install logic)

**Estimated scope:** Medium (3-4 files)

---

## Task 4: DMG creation script with drag-to-Applications background

**Description:** Create a script that packages the `Vortex.app` bundle into a `.dmg` with a custom background image showing a drag-to-Applications arrow. The script uses `hdiutil` (macOS-only) and can run in CI.

**Acceptance criteria:**
- [ ] Produces `Vortex-{version}.dmg`
- [ ] Opening the DMG shows the background image with Vortex.app and Applications alias
- [ ] Window is sized appropriately, icons positioned correctly
- [ ] DMG is compressed (UDZO or ULFO)

**Verification:**
- [ ] `hdiutil verify Vortex-{version}.dmg` passes
- [ ] Mount, drag, launch flow works end-to-end
- [ ] Script runs on macOS GitHub Actions runner

**Dependencies:** Task 1, Task 2 (needs complete `.app` with icon)

**Files likely touched:**
- New: `scripts/create-dmg.sh`
- New: `packaging/macos/dmg-background.png` (background image)
- New: `packaging/macos/dmg-settings.py` or AppleScript for icon positioning

**Estimated scope:** Medium (3-4 files)

---

## Task 5: Windows installer binary with webview progress UI

**Description:** Create a Go binary (`cmd/vortex-install-gui/` or restructure `cmd/vortex-install/`) that opens a native webview window showing a branded install progress screen. It downloads binaries, verifies checksums, and places them in `%LOCALAPPDATA%\Programs\Vortex\`. Reuses `internal/release` and `internal/webview` packages.

**Acceptance criteria:**
- [ ] Double-clicking the installer `.exe` shows a branded webview window
- [ ] Progress bar updates as binaries download
- [ ] On completion, shows "Launch Vortex" button
- [ ] Clicking launch opens the installed `vortex.exe run --ui`
- [ ] If Vortex is already installed, shows reinstall/upgrade/cancel options

**Verification:**
- [ ] Run installer on clean Windows → binaries appear in `%LOCALAPPDATA%\Programs\Vortex\`
- [ ] Run installer when already installed → sees reinstall prompt
- [ ] Build succeeds with `-H=windowsgui` ldflags (no console window)

**Dependencies:** Task 2 (needs `.ico` for the exe resource)

**Files likely touched:**
- New or modified: `cmd/vortex-install/` (add webview UI mode)
- New: `cmd/vortex-install/web/` (HTML/CSS/JS for installer UI) or embedded HTML
- Modified: `build.go` (build the GUI installer with `-H=windowsgui`)
- Reuses: `internal/release`, `internal/webview`

**Estimated scope:** Medium-Large (4-6 files)

---

## Task 6: Start Menu shortcut + Add/Remove Programs registration

**Description:** After binaries are installed on Windows, create a Start Menu shortcut pointing to `vortex.exe run --ui` and register an entry in `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\Vortex` so the app appears in Add/Remove Programs. Place a copy of the installer as `uninstall.exe` alongside the binaries.

**Acceptance criteria:**
- [ ] Start Menu contains "Vortex" shortcut after install
- [ ] Shortcut launches `vortex.exe run --ui`
- [ ] "Vortex" appears in Settings → Apps → Installed Apps
- [ ] Uninstall entry points to `uninstall.exe --uninstall`
- [ ] Shortcut has the correct `.ico` icon

**Verification:**
- [ ] Windows search finds "Vortex" after install
- [ ] Click shortcut → terminal UI opens
- [ ] App visible in Add/Remove Programs with correct version and publisher

**Dependencies:** Task 5 (runs as part of install flow)

**Files likely touched:**
- New: `internal/release/shortcut_windows.go` (create .lnk, registry keys)
- Modified: `cmd/vortex-install/main.go` (call shortcut/registry functions after install)

**Estimated scope:** Medium (2-3 files, but Win32 API / COM interaction is fiddly)

---

## Task 7: Uninstall mode with "remove config?" UI

**Description:** Add `--uninstall` mode to the installer binary. When invoked (or when the binary is named/copied as `uninstall.exe`), it shows a small webview window with a checkbox "Also remove configuration and data" and a confirm/cancel button. On confirm, it removes binaries, app bundle/shortcuts, and optionally config.

**Acceptance criteria:**
- [ ] `vortex-install --uninstall` (or `uninstall.exe`) opens uninstall UI
- [ ] Checkbox: "Also remove configuration and data" (unchecked by default)
- [ ] Confirm removes:
  - macOS: `~/.local/bin/vortex`, `~/.local/bin/vortex-window`, `/Applications/Vortex.app`
  - Windows: `%LOCALAPPDATA%\Programs\Vortex\` contents, Start Menu shortcut, registry key
- [ ] If checkbox checked, also removes `~/.config/vortex` (macOS) / `%APPDATA%\Vortex` (Windows)
- [ ] Shows completion message

**Verification:**
- [ ] Run uninstaller → binaries gone, shortcuts gone, registry cleaned
- [ ] Run with checkbox → config dir also gone
- [ ] Cancel → nothing removed

**Dependencies:** Task 5 (shares the webview UI infrastructure), Task 6 (needs to know what to undo on Windows)

**Files likely touched:**
- Modified: `cmd/vortex-install/main.go` (detect `--uninstall` flag or binary name)
- New: `cmd/vortex-install/uninstall.go` (uninstall logic)
- New: embedded HTML for uninstall UI (or extend installer HTML)
- New: `internal/release/uninstall_darwin.go`, `internal/release/uninstall_windows.go`

**Estimated scope:** Medium (4-5 files)

---

## Task 8: Detect existing installation, offer reinstall/upgrade/cancel

**Description:** On launch, the installer checks whether `vortex` is already installed at the expected location. If found, it shows a dialog (in the webview) with three options: Reinstall (overwrite), Upgrade (download latest), Cancel. This replaces the immediate progress-bar flow.

**Acceptance criteria:**
- [ ] Installer detects existing binary at `~/.local/bin/vortex` or `%LOCALAPPDATA%\Programs\Vortex\vortex.exe`
- [ ] Shows version of currently installed Vortex (via `vortex --version` or binary inspection)
- [ ] Reinstall: re-downloads and overwrites at same version as installer
- [ ] Upgrade: fetches latest release instead of pinned version
- [ ] Cancel: exits cleanly

**Verification:**
- [ ] Install Vortex → run installer again → sees "already installed" prompt
- [ ] Click Reinstall → binaries replaced
- [ ] Click Cancel → nothing changed

**Dependencies:** Task 3 (macOS) or Task 5 (Windows) — needs the installer UI to exist

**Files likely touched:**
- Modified: `cmd/vortex-install/main.go` (detection logic before showing progress)
- Modified: installer HTML/JS (add the reinstall/upgrade/cancel view)

**Estimated scope:** Small (2 files, mostly UI state logic)

---

## Task 9: Add DMG creation to release workflow (macOS)

**Description:** Extend `.github/workflows/release.yml` to produce a `Vortex-{version}.dmg` as a release artifact on macOS. The DMG contains the pre-built `Vortex.app` bundle with the embedded version matching the release tag. The app's bootstrap will download the matching release binaries on first launch.

**Acceptance criteria:**
- [ ] macOS build job produces `Vortex-{version}.dmg`
- [ ] DMG is uploaded as a release artifact alongside existing binaries
- [ ] DMG size is reasonable (< 10 MB — it's just the launcher + icon + bootstrap code)

**Verification:**
- [ ] Tag a test release → DMG appears in GitHub Release assets
- [ ] Download DMG on a fresh Mac → drag to Applications → first launch installs → works

**Dependencies:** Task 3, Task 4 (needs bootstrap logic + DMG script)

**Files likely touched:**
- Modified: `.github/workflows/release.yml` (add DMG creation step)
- Modified: `build.go` (possibly add a build target for the app bundle)

**Estimated scope:** Small (1-2 files, but CI debugging may take iteration)

---

## Task 10: Add installer `.exe` to release workflow (Windows)

**Description:** Extend the release workflow to build the GUI installer (`vortex-install-{os}-{arch}.exe`) with webview UI embedded, using `-H=windowsgui` ldflags. This replaces or supplements the current CLI-only `vortex-install` binary.

**Acceptance criteria:**
- [ ] Windows build jobs produce a GUI installer binary
- [ ] Installer binary is uploaded as a release artifact
- [ ] Binary has the embedded version matching the release tag
- [ ] Running the installer on Windows shows the webview UI (not a console window)

**Verification:**
- [ ] Tag a test release → installer `.exe` appears in GitHub Release assets
- [ ] Download on Windows → double-click → see branded install UI → completes successfully

**Dependencies:** Task 5, Task 6 (needs Windows installer fully working)

**Files likely touched:**
- Modified: `.github/workflows/release.yml` (update Windows build step)
- Modified: `build.go` (add `-H=windowsgui` for the installer binary on Windows)

**Estimated scope:** Small (1-2 files)

---

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| macOS Gatekeeper blocks unsigned `.app` harshly | High — users can't open the app | DMG delivery helps; document right-click → Open; consider ad-hoc signing (`codesign -s -`) which is free |
| Windows SmartScreen blocks installer | Medium — users see scary warning | Document click-through; consider self-signing with a free cert |
| Login-shell launcher doesn't pick up all env | Medium — tools missing in terminals | Test with nvm, pyenv, homebrew; add fallback to source `/etc/profile` |
| First-launch bootstrap confusing for users | Low — unusual UX pattern | Clear messaging: "Setting up Vortex for first time..." with progress |
| `hdiutil` not available or broken on GH Actions | Low — macOS runner should have it | Pin runner version; add fallback to `create-dmg` npm package |
| `vortex upgrade` breaks `.app` bundle | Medium — users stuck on old version | Ensure upgrade only replaces binaries in `~/.local/bin/`, not the `.app` itself |

## Open Questions

- Should the `.app` bundle be self-contained (embed binaries) or always bootstrap from GitHub? Embedding increases DMG size but eliminates first-launch download.
- Should ad-hoc codesigning (`codesign -s -`) be used? It's free and reduces Gatekeeper friction slightly.
- What CFBundleIdentifier to use? Suggest `com.arcmantle.vortex`.
- Should the Windows installer use the same webview HTML/CSS as the main Vortex UI, or a separate minimal page?
