# Testing the Native Installer

This document covers how to test the macOS `.app` bundle / DMG installer and the Windows GUI installer locally, without needing a GitHub release.

## Quick Start (macOS)

```bash
go run scripts/test-installer.go
```

This will:

1. Rebuild the frontend UI (`pnpm build`)
2. Compile `vortex`, `vortex-window`, `vortex-bootstrap`, `vortex-launcher`
3. Assemble a `Vortex.app` bundle with the binaries embedded
4. Create a DMG
5. **Kill any running vortex processes** (they'd show stale UI otherwise)
6. **Remove your existing `~/.local/bin/vortex` install** so the bootstrap triggers
7. Open the DMG in Finder

Then you:

1. Drag `Vortex.app` to `/Applications` (or anywhere)
2. Launch it
3. The bootstrap progress window appears and installs the binaries to `~/.local/bin/`
4. Vortex starts

## Quick Start (Windows)

```powershell
go run scripts/test-installer.go
```

Same as above but:

- Builds `vortex-install-gui.exe` instead of the macOS bundle
- Removes existing install from `%LOCALAPPDATA%\Programs\Vortex`
- Launches the installer GUI which shows progress and installs the binaries

## Options

| Flag | Description |
|------|-------------|
| `--build` | Build only — don't uninstall or open anything |
| `--clean` | Remove build artifacts from temp directory |

## How It Works

The test script embeds the locally-built binaries inside the `.app` bundle at `Contents/Resources/local-binaries/`. When the bootstrap runs, it checks that path before trying to download from GitHub. This means:

- No GitHub release needed
- No internet access needed
- The full UI flow (progress window, PATH setup) runs exactly as it would for a real user

On Windows, the `VORTEX_BOOTSTRAP_LOCAL` environment variable points the installer at the local binaries directory.

## Architecture

```
Vortex.app/
  Contents/
    Info.plist
    MacOS/
      vortex-launcher    ← Mach-O binary (CFBundleExecutable)
      vortex-bootstrap   ← First-launch installer with progress UI
    Resources/
      vortex.icns
      local-binaries/    ← Only present in test builds
        vortex
        vortex-window
```

**Launch flow:**
1. `vortex-launcher` checks if `~/.local/bin/vortex` exists
2. If yes → exec it via login shell (inherits PATH, homebrew, nvm, etc.)
3. If no → exec `vortex-bootstrap`
4. Bootstrap shows progress webview, copies binaries, configures PATH, launches vortex

## Re-installing After Testing

The test script removes your existing vortex install. After testing, reinstall with:

```bash
# Option 1: self-update
vortex upgrade

# Option 2: rebuild from source
go run build.go -ui -local
```

## Troubleshooting

**"App is damaged" / Gatekeeper warning**: Expected for unsigned local builds. Right-click → Open, or:
```bash
xattr -cr /Applications/Vortex.app
```

**Bootstrap shows error about version**: The test build uses `local-binaries/` — if that directory is missing, the bootstrap falls through to GitHub download mode which requires a real version tag.

**vortex doesn't start after install**: Check `~/.local/bin/vortex` exists and is executable. The launcher uses your login shell (`$SHELL`) to source your profile before exec'ing vortex.
