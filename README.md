# Vortex

A process orchestrator that runs multiple jobs, manages their dependencies, and streams live output to a native webview terminal UI.

![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white)
![TypeScript](https://img.shields.io/badge/TypeScript-5.8-3178C6?logo=typescript&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-yellow)

## Features

- **Dependency-aware execution** — jobs declare what they `needs`, with conditions (`success`, `failure`, `always`) inspired by GitHub Actions
- **Live terminal output** — each job streams stdout/stderr into its own xterm.js terminal panel via SSE
- **Native webview** — opens as a standalone desktop window (Edge WebView2 on Windows, WKWebView on macOS, WebKitGTK on Linux)
- **Job groups** — organize related jobs under collapsible groups in the UI
- **Named instances** — every YAML config declares a required `name`, and Vortex routes restart / quit commands to that specific running instance
- **Persistent jobs** — mark long-running jobs with `restart: false` to keep them alive across config reloads
- **Embedded UI** — production builds embed the frontend into the Go binary (zero external files)

## Quick Start

### Config file

```yaml
# vortex.yaml
name: dev

jobs:
  - id: build
    label: Build
    command: go build ./...
    group: ci

  - id: test
    label: Test
    command: go test ./...
    group: ci
    needs: [build]

  - id: deploy
    label: Deploy
    command: ./deploy.sh
    needs: [test]
    if: success
```

```sh
vortex vortex.yaml
```

## Config Reference

Top-level config fields:

| Field  | Type     | Description                                                                  |
|--------|----------|------------------------------------------------------------------------------|
| `name` | `string` | **Required.** Instance name used to target restarts and `vortex <name> --quit`. |
| `jobs` | `Job[]`  | **Required.** Jobs to run in this instance.                                  |

Each job supports:

| Field     | Type       | Description                                                             |
|-----------|------------|-------------------------------------------------------------------------|
| `id`      | `string`   | **Required.** Unique identifier, used in `needs` references.            |
| `label`   | `string`   | Display name in the UI. Defaults to `id`.                               |
| `command` | `string`   | **Required.** Shell command to execute (space-separated with args).     |
| `group`   | `string`   | Optional group name — jobs in the same group are visually grouped.      |
| `needs`   | `string[]` | IDs of jobs that must complete before this one starts.                  |
| `if`      | `string`   | When to run: `success` (default), `failure`, or `always`.               |
| `restart` | `bool`     | Whether to kill and re-launch on restart. Defaults to `true`.           |

## CLI Flags

```
vortex [flags] config.yaml
vortex instances [name]
vortex <name> --quit
vortex <name> --kill
vortex <name> show-ui
vortex <name> hide-ui
vortex upgrade
```

| Flag       | Default | Description                                             |
|------------|---------|---------------------------------------------------------|
| `--config` | —       | Path to YAML config file                                |
| `--port`   | derived from `name` | Override the deterministic HTTP port for this instance |
| `--headless` | `false` | Run normally without opening the native webview       |
| `--dev`    | `false` | Development mode: skip the native webview and use the browser/Vite workflow |
| `--quit`   | `false` | Ask the named Vortex instance to shut down and exit     |
| `--kill`   | `false` | Ask the named Vortex instance to terminate its managed child processes |

`name` is mandatory. Unnamed configs fail validation.

By default, Vortex derives both the handoff port and the HTTP/UI port from the config name, so different named configs can run at the same time without manual port management.

To stop a running instance from the CLI:

```sh
go run ./cmd/vortex dev --quit
```

To terminate all child processes managed by a running instance without shutting down the Vortex controller:

```sh
go run ./cmd/vortex dev --kill
```

To start a config without opening the native window immediately:

```sh
go run -tags embed_ui ./cmd/vortex --headless --config mock/dev.yaml
```

To surface the native webview later for that running instance:

```sh
go run ./cmd/vortex dev show-ui
```

To dismiss the native webview later without stopping the running instance:

```sh
go run ./cmd/vortex dev hide-ui
```

`show-ui` is intended for non-`--dev` instances. If an instance was started with `--dev`, there is no native webview to surface later.

`hide-ui` is non-destructive: it hides the native window but leaves the Vortex instance and its managed jobs running.

Use `--headless` for normal no-window operation. Keep `--dev` for the development workflow where the Vite dev server proxies to Vortex and you work in the browser.

To list running instances and the process IDs they currently manage:

```sh
go run ./cmd/vortex instances
go run ./cmd/vortex instances dev
go run ./cmd/vortex instances --json
go run ./cmd/vortex instances --prune
```

The `instances` output includes each instance `mode` as one of `dev`, `headless`, or `windowed`, and each live `ui` state as one of `open`, `hidden`, or `none`.

Use `instances --prune` to explicitly remove stale registry entries for instances that are no longer reachable.

It also includes:
- `started`: when the instance was first registered
- `updated`: the last metadata/lifecycle update, currently refreshed on restart and UI visibility changes
- `last_control`: the last explicit control action time, currently refreshed on kill actions
- `generation`: the orchestrator restart generation for the running instance
- `reachable`: in `--json` output, whether the instance API was reachable when queried

To restart an already-running instance, rerun any YAML config that declares the same `name`:

```sh
go run ./cmd/vortex --config mock/dev.yaml
```

Inline `label:command` mode is no longer supported. Use a YAML config with a top-level `name` instead.

To upgrade to the latest GitHub release and install it into a managed location:

```sh
vortex upgrade
```

To check whether a newer release is available without changing anything:

```sh
vortex upgrade --check
```

Managed install locations:

- macOS/Linux: `~/.local/bin/vortex`
- Windows: `%LOCALAPPDATA%\Programs\Vortex\vortex.exe`

The `upgrade` command will:

- download the latest release asset for your current OS/architecture
- verify the downloaded binary against the release SHA-256 checksum file
- stop a running Vortex instance before replacing the installed binary
- place the binary into the managed install location if it is not already there
- on macOS, make the installed binary executable and remove the `com.apple.quarantine` attribute
- attempt to add that install directory to your user PATH automatically

If your shell was updated, open a new terminal session so the new PATH is loaded.

For CI or smoke tests, you can serve the embedded UI without opening a native window:

```sh
go run -tags embed_ui ./cmd/vortex --headless
```

## Building

### Prerequisites

- Go 1.24+
- Node.js 20+ / pnpm
- C compiler (CGo is required for the webview binding)
  - **Windows:** MSVC or MinGW — WebView2 headers are bundled
  - **macOS:** Xcode command-line tools
  - **Linux:** `libwebkit2gtk-4.1-dev`

### Development

```sh
# Start the Go server + Vite dev server
go run ./cmd/vortex --dev --config mock/dev.yaml &
cd cmd/vortex-ui/web && pnpm install && pnpm dev
```

The Vite dev server proxies API calls to the Go server. Run vortex with `--dev` to skip the webview and use the browser at `http://localhost:5173`.

To test the native window from source, build the frontend and use the embedded-UI tag:

```sh
cd cmd/vortex-ui/web
pnpm install
pnpm build

cd ../../..
go run -tags embed_ui ./cmd/vortex --config mock/dev.yaml
```

### Production binary

```sh
# Build the frontend
cd cmd/vortex-ui/web
pnpm install
pnpm build

# Build the Go binary with embedded UI
go build -tags embed_ui -ldflags "-H=windowsgui" -o vortex.exe ./cmd/vortex
```

On Windows, `-H=windowsgui` builds a GUI subsystem binary so the launching terminal is freed immediately. On macOS/Linux, omit that flag:

```sh
go build -tags embed_ui -o vortex ./cmd/vortex
```

If you download the Darwin release binary directly from GitHub, make it executable and remove the quarantine attribute before first launch:

```sh
chmod +x ./vortex-darwin-arm64
xattr -d com.apple.quarantine ./vortex-darwin-arm64 2>/dev/null || true
```

The `embed_ui` build tag embeds the compiled frontend into the binary. Without it, the UI is not served (useful for dev mode).

## Architecture

```
cmd/vortex/          CLI entry point, UI embedding
cmd/vortex-ui/web/   Lit + TypeScript frontend (xterm.js terminals)
internal/
  config/            YAML config loading and validation
  instance/          Named-instance lock, deterministic port derivation, handoff registry
  orchestrator/      Dependency-aware job graph execution
  terminal/          Process lifecycle, output buffering, ring buffer
  server/            HTTP API, SSE streaming, static file serving
  webview/           Platform-specific native webview wrappers
```

**Data flow:** Orchestrator → Terminal (captures stdout/stderr into ring buffer) → SSE endpoint → xterm.js in the webview.

## License

MIT
