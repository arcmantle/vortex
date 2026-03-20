# Vortex

A process orchestrator that runs multiple jobs, manages their dependencies, and streams live output to a native webview terminal UI.

![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white)
![TypeScript](https://img.shields.io/badge/TypeScript-5.8-3178C6?logo=typescript&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-yellow)

## Features

- **Dependency-aware execution** — jobs declare what they `needs`, with conditions (`success`, `failure`, `always`) inspired by GitHub Actions
- **Live terminal output** — each job streams stdout/stderr into its own xterm.js terminal panel via SSE
- **Clickable terminal links** — `http` and `https` URLs open in your external browser, and file paths open in your editor using `vortex config set editor ...`, `VORTEX_EDITOR`, `VISUAL`, or `EDITOR`
- **Native webview** — opens as a standalone desktop window (Edge WebView2 on Windows, WKWebView on macOS, WebKitGTK on Linux)
- **Job groups** — organize related jobs under collapsible groups in the UI
- **Named instances** — every Vortex config declares a required `name`, and Vortex routes restart / quit commands to that specific running instance
- **Persistent jobs** — mark long-running jobs with `restart: false` to keep them alive across config reloads
- **Embedded UI** — production builds embed the frontend into the Go binary (zero external files)

## Quick Start

### Config file

Vortex config files use the `.vortex` extension and YAML syntax.

```yaml
# dev.vortex
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
    shell: bash
    command: ./deploy.sh --verify
    needs: [test]
    if: success

  - id: smoke-node
    label: Node Smoke
    shell: node
    command: |
      console.log('smoke test starting')
      console.log(process.version)
```

```sh
vortex run dev.vortex
vortex run dev
```

To create a new template config with a schema comment pinned to the version of `vortex` you are running:

```sh
vortex init
vortex init my-app
vortex init configs/dev.vortex
```

`vortex init` writes a `.vortex` file, adds a top-of-file `yaml-language-server` schema comment, and uses the running Vortex version to choose the schema URL:

- `dev` builds point at `master`
- release builds point at the matching `v<version>` tag

Run `vortex help` to see the CLI commands and examples directly in the terminal, or `vortex docs` to open this README as embedded app documentation.

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
| `shell`   | `string \| object` | Optional interpreter for script blocks. Accepts either a plain shell string or an OS selector object with `darwin`, `linux`, `windows`, and `default` keys. |
| `command` | `string \| object` | **Required.** Direct command line when `shell` is omitted, or script text when `shell` is set. Accepts either a plain string or an OS selector object with `darwin`, `linux`, `windows`, and `default` keys. |
| `group`   | `string`   | Optional group name — jobs in the same group are visually grouped.      |
| `needs`   | `string[]` | IDs of jobs that must complete before this one starts.                  |
| `if`      | `string`   | When to run: `success` (default), `failure`, or `always`.               |
| `restart` | `bool`     | Whether to kill and re-launch on restart. Defaults to `true`.           |

### VS Code Schema

This repository includes a JSON schema at `schemas/vortex.schema.json` and wires it up in `.vscode/settings.json` for:

- `dev.vortex`
- `mock/*.vortex`
- `*.vortex`

If you want the same behavior in your own workspace or user settings, add:

```json
{
  "files.associations": {
    "*.vortex": "yaml"
  },
  "yaml.schemas": {
    "https://raw.githubusercontent.com/arcmantle/vortex/master/schemas/vortex.schema.json": [
      "*.vortex"
    ]
  }
}
```

If you want a stable contract instead of following the latest `master` schema, pin the URL to a release tag:

```json
{
  "yaml.schemas": {
    "https://raw.githubusercontent.com/arcmantle/vortex/v1.0.10/schemas/vortex.schema.json": [
      "*.vortex"
    ]
  }
}
```

Using `master` is convenient for active development. Using a tag is safer for teams that want reproducible validation behavior over time.

For SchemaStore submission, the schema itself also needs a stable public URL and a catalog entry with file matches. This repository now includes a ready-to-submit example catalog record at `schemas/vortex.schemastore-entry.json`.

## CLI

```
vortex help
vortex config list
vortex config get [key]
vortex config set <key> <value>
vortex config unset <key>
vortex init [path] [--force]
vortex run [--dev] [--headless] [--port <n>] [--cwd <path>] [--config <path>] [config-file]
vortex docs [--force] [--no-open]
vortex --help
vortex -h
vortex version
vortex --version
vortex -v
vortex instance list [name] [--json] [--prune]
vortex instance quit [name] [--config <path>]
vortex instance kill [name] [--config <path>]
vortex instance rerun <name> <job-id>
vortex instance show-ui [name] [--config <path>]
vortex instance hide-ui [name] [--config <path>]
vortex upgrade [--check]
```

Important command-specific flags:

| Flag | Applies To | Description |
|------|------------|-------------|
| `--config` | `run`, `instance quit`, `instance kill`, `instance show-ui`, `instance hide-ui` | Resolve the config or target instance from a Vortex config file |
| `--cwd` | `run` | Working directory for all jobs. Defaults to the directory containing the `.vortex` file |
| `--force` | `init`, `docs` | Overwrite an existing generated file |
| `--port` | `run` | Override the deterministic HTTP port for the instance |
| `--headless` | `run` | Run normally without opening the native webview |
| `--dev` | `run` | Development mode: skip the native webview and use the browser/Vite workflow |
| `--json` | `instance list` | Emit machine-readable JSON |
| `--prune` | `instance list` | Remove stale instance entries while listing |
| `--no-open` | `docs` | Generate docs without opening a browser |

`name` is mandatory. Unnamed configs fail validation.

Vortex also stores user-level settings in its own config file. The supported keys are `browser` and `editor`.

```sh
vortex config list
vortex config set browser "firefox"
vortex config set editor "code"
vortex config get browser
vortex config get editor
vortex config unset browser
vortex config unset editor
```

When an `http` or `https` terminal link is clicked, Vortex resolves the browser in this order:

- `VORTEX_BROWSER`
- saved `browser` setting from `vortex config set`
- `BROWSER`

If none of those are set, Vortex falls back to the operating system's default browser opener.

When a terminal file path is clicked, Vortex resolves the editor in this order:

- `VORTEX_EDITOR`
- saved `editor` setting from `vortex config set`
- `VISUAL`
- `EDITOR`

If none of those are set, Vortex falls back to the operating system's default file opener.

When `shell` is omitted, Vortex executes `command` directly by splitting it into argv.
When `shell` is set, Vortex passes `command` as a script block to that interpreter.

Both `shell` and `command` can also be OS-specific objects. Vortex resolves them for the current runtime OS using these keys:

- `darwin`
- `linux`
- `windows`
- `default`

Example:

```yaml
jobs:
  - id: cross-platform-smoke
    shell:
      default: bash
      windows: pwsh
    command:
      default: echo hello from vortex
      windows: Write-Host hello from vortex
```

By default, Vortex runs every job with the working directory set to the directory containing the `.vortex` file.
Use `--cwd` to override that for the whole run.

By default, Vortex derives both the handoff port and the HTTP/UI port from the config name, so different named configs can run at the same time without manual port management.

To stop a running instance from the CLI:

```sh
go run ./cmd/vortex instance quit dev
```

To terminate all child processes managed by a running instance without shutting down the Vortex controller:

```sh
go run ./cmd/vortex instance kill dev
```

To rerun a specific job and any downstream jobs that depend on it, without rerunning unrelated jobs:

```sh
go run ./cmd/vortex instance rerun dev run-server-a
```

To open the embedded README documentation in your browser:

```sh
go run ./cmd/vortex docs
```

To regenerate the rendered docs or write them without opening a browser:

```sh
go run ./cmd/vortex docs --force
go run ./cmd/vortex docs --no-open
```

To start a config without opening the native window immediately:

```sh
go run -tags embed_ui ./cmd/vortex run --headless --config mock/dev.vortex
```

To surface the native webview later for that running instance:

```sh
go run ./cmd/vortex instance show-ui dev
```

To dismiss the native webview later without stopping the running instance:

```sh
go run ./cmd/vortex instance hide-ui dev
```

`show-ui` is intended for non-`--dev` instances. If an instance was started with `--dev`, there is no native webview to surface later.

`hide-ui` is non-destructive: it hides the native window but leaves the Vortex instance and its managed jobs running.

Use `--headless` for normal no-window operation. Keep `--dev` for the development workflow where the Vite dev server proxies to Vortex and you work in the browser.

To list running instances and the process IDs they currently manage:

```sh
go run ./cmd/vortex instance list
go run ./cmd/vortex instance list dev
go run ./cmd/vortex instance list --json
go run ./cmd/vortex instance list --prune
```

The `instance list` output includes each instance `mode` as one of `dev`, `headless`, or `windowed`, and each live `ui` state as one of `open`, `hidden`, or `none`.

Use `instance list --prune` to explicitly remove stale registry entries for instances that are no longer reachable.

When pruning a stale instance, Vortex also makes a best-effort attempt to terminate the last recorded controller and managed child processes before removing the registry entry.

It also includes:
- `started`: when the instance was first registered
- `updated`: the last metadata/lifecycle update, currently refreshed on restart and UI visibility changes
- `last_control`: the last explicit control action time, currently refreshed on kill, rerun, and UI visibility actions
- `generation`: the orchestrator restart generation for the running instance
- `reachable`: in `--json` output, whether the instance API was reachable when queried

To restart an already-running instance, rerun any Vortex config that declares the same `name`:

```sh
go run ./cmd/vortex run --config mock/dev.vortex
```

Inline `label:command` mode is no longer supported. Use a Vortex config with a top-level `name` instead.

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
go run -tags embed_ui ./cmd/vortex run --headless
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
go run ./cmd/vortex --dev --config mock/dev.vortex &
cd cmd/vortex-ui/web && pnpm install && pnpm dev
```

The Vite dev server proxies API calls to the Go server. Run vortex with `--dev` to skip the webview and use the browser at `http://localhost:5173`.

To test the native window from source, build the frontend and use the embedded-UI tag:

```sh
cd cmd/vortex-ui/web
pnpm install
pnpm build

cd ../../..
go run -tags embed_ui ./cmd/vortex --config mock/dev.vortex
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

On Windows, `-H=windowsgui` builds a GUI subsystem binary so the launching terminal is freed immediately. If you launch a console build instead, Vortex now re-spawns itself detached as a fallback. On macOS/Linux, you can either use `build.go` or omit that flag with plain `go build`:

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
  config/            Vortex config loading and validation
  instance/          Named-instance lock, deterministic port derivation, handoff registry
  orchestrator/      Dependency-aware job graph execution
  terminal/          Process lifecycle, output buffering, ring buffer
  server/            HTTP API, SSE streaming, static file serving
  webview/           Platform-specific native webview wrappers
```

**Data flow:** Orchestrator → Terminal (captures stdout/stderr into ring buffer) → SSE endpoint → xterm.js in the webview.

## License

MIT
