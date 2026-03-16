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
- **Single-instance** — launching vortex while it's already running forwards the new config to the existing instance, which restarts with the new job graph
- **Detached execution** — the launching terminal is freed immediately; vortex runs in the background
- **Embedded UI** — production builds embed the frontend into the Go binary (zero external files)

## Quick Start

### Config file

```yaml
# vortex.yaml
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

### Inline mode

```sh
vortex -- "server:go run ./cmd/server" "client:pnpm dev"
```

All inline jobs run in parallel with no dependencies.

## Config Reference

Each job supports:

| Field     | Type       | Description                                                             |
|-----------|------------|-------------------------------------------------------------------------|
| `id`      | `string`   | **Required.** Unique identifier, used in `needs` references.            |
| `label`   | `string`   | Display name in the UI. Defaults to `id`.                               |
| `command` | `string`   | **Required.** Shell command to execute (space-separated with args).      |
| `group`   | `string`   | Optional group name — jobs in the same group are visually grouped.       |
| `needs`   | `string[]` | IDs of jobs that must complete before this one starts.                   |
| `if`      | `string`   | When to run: `success` (default), `failure`, or `always`.               |
| `restart` | `bool`     | Whether to kill and re-launch on restart. Defaults to `true`.           |

## CLI Flags

```
vortex [flags] [config.yaml | -- label:command ...]
```

| Flag       | Default | Description                                             |
|------------|---------|---------------------------------------------------------|
| `--config` | —       | Path to YAML config file                                |
| `--port`   | `7370`  | HTTP port for the API / SSE server                      |
| `--dev`    | `false` | Skip the native webview; use a browser instead          |

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
# Start the Go server with hot-reload + Vite dev server
# (requires Air: go install github.com/air-verse/air@latest)
air &
cd cmd/vortex-ui/web && pnpm install && pnpm dev
```

The Vite dev server proxies API calls to the Go server. Run vortex with `--dev` to skip the webview and use the browser at `http://localhost:5173`.

### Production binary

```sh
# Build the frontend
cd cmd/vortex-ui/web
pnpm install
pnpm build

# Build the Go binary with embedded UI
go build -tags embed_ui -o vortex ./cmd/vortex
```

The `embed_ui` build tag embeds the compiled frontend into the binary. Without it, the UI is not served (useful for dev mode).

## Architecture

```
cmd/vortex/          CLI entry point, detach logic, UI embedding
cmd/vortex-ui/web/   Lit + TypeScript frontend (xterm.js terminals)
internal/
  config/            YAML config loading and validation
  instance/          Single-instance lock via TCP + handoff protocol
  orchestrator/      Dependency-aware job graph execution
  terminal/          Process lifecycle, output buffering, ring buffer
  server/            HTTP API, SSE streaming, static file serving
  webview/           Platform-specific native webview wrappers
```

**Data flow:** Orchestrator → Terminal (captures stdout/stderr into ring buffer) → SSE endpoint → xterm.js in the webview.

## License

MIT
