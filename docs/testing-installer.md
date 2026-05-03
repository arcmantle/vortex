# Testing the Native Installer

This document covers how to test the macOS `.app` bundle / DMG installer and the Windows GUI installer locally, without needing a GitHub release.

## Quick Start (macOS)

```bash
go run scripts/test-installer.go
```

This will:

1. Rebuild the frontend UI (`pnpm build`)
2. Compile `vortex-host`, `vortex`, `vortex-setup`
3. Assemble a `Vortex.app` bundle with the binaries embedded
4. Create a DMG
5. **Kill any running vortex processes** (they'd show stale UI otherwise)
6. **Remove your existing `~/.local/bin/vortex-host` install** and `/Applications/Vortex.app` so setup triggers
7. Open the DMG in Finder

Then you:

1. Drag `Vortex.app` to `/Applications` (or anywhere)
2. Launch it
3. The setup progress window appears and installs the binaries to `~/.local/bin/`
4. Vortex starts

## Quick Start (Windows)

```powershell
go run scripts/test-installer.go
```

Same as above but:

- Builds `vortex-setup.exe` instead of the macOS bundle
- Removes existing install from `%LOCALAPPDATA%\Programs\Vortex`
- Launches the installer GUI which shows progress and installs the binaries

## Options

| Flag | Description |
|------|-------------|
| `--build` | Build only — don't uninstall or open anything |
| `--clean` | Remove build artifacts from temp directory |

## How It Works

The test script embeds the locally-built binaries inside the `.app` bundle at `Contents/Resources/local-binaries/`. When vortex-setup runs, it checks that path before trying to download from GitHub. This means:

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
      vortex-setup       ← Mach-O binary (CFBundleExecutable + first-launch installer)
    Resources/
      vortex.icns
      local-binaries/    ← Only present in test builds
        vortex-host
        vortex
```

**Launch flow:**
1. `vortex-setup` checks if `~/.local/bin/vortex-host` exists
2. If yes → exec it via login shell (inherits PATH, homebrew, nvm, etc.)
3. If no → show progress webview, copy binaries, configure PATH, launch vortex-host

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

**vortex doesn't start after install**: Check `~/.local/bin/vortex` exists and is executable. `vortex-setup` uses your login shell (`$SHELL`) to source your profile before exec'ing vortex.
