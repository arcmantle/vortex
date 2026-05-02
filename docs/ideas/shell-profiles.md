# Shell Profiles

## Problem Statement
How might we let Vortex users choose from their available system shells with named, visually distinct profiles — picking a default and selecting per-terminal — so every shell tab has clear identity at a glance?

## Recommended Direction
Implement the full Windows Terminal profile model inside Vortex. Auto-detect installed shells on first run, create initial profiles for each, and let users manage them (add, edit, remove, reorder, set default) through a dedicated settings panel in the webview. Each profile carries a name, command, args, accent color, and custom icon image. The "+" button in the Shell group has a split interaction: click spawns the default profile, a dropdown arrow reveals the full profile list.

Shell tabs display the profile's color as an accent (left border or dot color) and the icon as a small glyph next to the label. This makes multiple open shells instantly scannable — you know which is your Fish shell and which is Bash without reading labels.

Profiles persist in `~/.config/vortex/config.json` alongside existing settings. On first launch (or when `shells` key is absent), auto-detection populates the initial list. Users who want more control open the settings panel; users who don't never need to touch it.

### Resolved Decisions
- **Icon size limit:** 50MB hard cap per icon; images are scaled down for display in tabs/UI.
- **Settings panel pattern:** Uses the same overlay mechanism as the config file preview (`vortex-config-preview`).
- **Dropdown order:** Matches the user's profile order in settings. Drag-to-reorder in settings controls what appears first in the picker.

## Key Assumptions to Validate
- [ ] Auto-detection covers common shells reliably — test on macOS (zsh/bash/fish/sh), Linux (bash/zsh/fish/sh), Windows (PowerShell/cmd/WSL/Git Bash). Validate with `/etc/shells` parsing + known path scanning.
- [ ] Custom icon images (small PNGs/SVGs) at display size don't cause performance issues — icons are scaled/thumbnailed for the UI regardless of source size.
- [ ] The settings panel is usable enough that users prefer it over editing JSON directly — validate with a few manual test sessions before polishing.

## MVP Scope

### Backend
- **Shell profile model:** `{id, name, command, args, color, icon, default}` struct in settings
- **Auto-detect:** Parse `/etc/shells` (Unix), scan registry + known paths (Windows); produce initial profiles with sensible names, colors, and built-in icons
- **Settings API:** `GET /api/settings/shells` (list profiles), `PUT /api/settings/shells` (save full profile list), `POST /api/settings/shells/detect` (re-run auto-detection)
- **Shell creation:** `POST /api/shells` accepts optional `profile` param to override default
- **Icon storage:** Icons stored as paths relative to `~/.config/vortex/icons/` or as inline data URIs in settings JSON. Hard cap of 50MB per icon file.

### Frontend
- **"+" button with dropdown:** Click = default profile, chevron/arrow = dropdown showing all profiles with icon + name + color swatch
- **Shell tab accent:** Left border color or colored dot matching the profile's color, profile icon displayed in tab
- **Settings panel:** Overlay (same pattern as config preview). Full CRUD for profiles:
  - List profiles with drag-to-reorder
  - Edit: name, command, args, color (picker), icon (upload or pick from built-ins)
  - Add custom profile
  - Delete profile
  - Set default (star/toggle)
  - "Re-detect shells" button to refresh from system
- **Built-in icons:** Ship a small set of default shell icons (terminal glyph, zsh, bash, fish, PowerShell, cmd) as embedded SVGs; allow user upload for custom ones

### Platform Detection
- **macOS/Linux:** Read `/etc/shells`, check if binaries exist, derive names from basename, assign default colors per shell type
- **Windows:** Check for PowerShell (Core + Desktop), cmd.exe, WSL distros (`wsl --list --quiet`), Git Bash (`C:\Program Files\Git\bin\bash.exe`)

## Not Doing (and Why)
- **Per-profile environment variables** — adds complexity and blurs into "terminal environment" territory; can layer on later without breaking the profile model
- **Per-profile working directory** — shells inherit the project dir; per-profile override is a power-user edge case for v2
- **Startup scripts / init commands** — too close to shell rc-file management; users should configure their shells via `.zshrc`/`.bashrc` instead
- **Per-project shell profiles in `.vortex`** — conflicts with the "shells are separate from config" principle; revisit if users ask for it
- **Font/theme per profile** — terminal emulator scope creep; one theme for all terminals is fine

## Open Questions
- Should the settings panel be accessible from the Shell group only, or from a global gear icon? (Lean toward global — profiles are a cross-session setting)
- Should profile order in the dropdown also determine tab ordering when multiple shells are open? (Probably not — tabs should follow creation order)
