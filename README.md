# Vortex

A process orchestrator that runs multiple jobs, manages their dependencies, and streams live output to a native webview terminal UI.

![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white)
![TypeScript](https://img.shields.io/badge/TypeScript-5.8-3178C6?logo=typescript&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-yellow)

## Install

Download and run the installer for your platform from [GitHub Releases](https://github.com/arcmantle/vortex/releases/latest):

**macOS:**

Download the DMG from [Releases](https://github.com/arcmantle/vortex/releases/latest), open it, and drag **Vortex** to your Applications folder. On first launch, Vortex will install the required binaries automatically.

**Windows:**

```powershell
irm https://github.com/arcmantle/vortex/releases/latest/download/vortex-setup-windows-amd64.exe -OutFile vortex-setup.exe
.\vortex-setup.exe
```

**Linux:**

```sh
curl -fsSL https://github.com/arcmantle/vortex/releases/latest/download/vortex-setup-linux-amd64 -o vortex-setup
chmod +x vortex-setup
./vortex-setup
```

**Update an existing install:**

```sh
vortex upgrade
```

## What Is Vortex?

Vortex lets you define a set of tasks (called **jobs**) in a simple YAML file and run them all at once. Each job gets its own live terminal panel in a desktop window, so you can watch logs, errors, and output in real time.

**Use it when you need to:**
- Start multiple services during development (API server, frontend, database, etc.)
- Run build steps in order (lint → build → test → deploy)
- Automate multi-step workflows with real-time visibility

## Quick Start

### 1. Create a config file

Vortex config files use the `.vortex` extension with YAML syntax:

```yaml
# dev.vortex
name: dev

jobs:
  - id: server
    label: API Server
    command: go run ./cmd/server

  - id: frontend
    label: Frontend
    command: npm run dev

  - id: test
    label: Tests
    command: go test ./...
    needs: [server]
```

### 2. Run it

```sh
vortex run dev.vortex
```

That's it. Vortex opens a native window with a terminal panel for each job, streaming output in real time.

<details>
<summary><strong>Shorthand: you can omit the .vortex extension</strong></summary>

```sh
vortex run dev
```

Vortex looks for `dev.vortex` in the current directory.

</details>

### 3. Create a template config

```sh
vortex init
vortex init my-app
```

This creates a `.vortex` file with a schema comment for editor autocompletion.

## Features

- **Dependency-aware execution** — jobs declare what they `needs`, and Vortex runs them in the right order
- **Live terminal output** — each job gets its own terminal panel with real-time stdout/stderr
- **Clickable links** — URLs open in your browser, file paths open in your editor
- **Native desktop window** — runs as a standalone app (not in the browser)
- **Job groups** — organize related jobs under collapsible headings
- **Named instances** — multiple configs can run side by side without conflicts
- **Persistent jobs** — long-running services survive config reloads with `restart: false`

<details>
<summary><strong>How clickable links work</strong></summary>

When you click an `http://` or `https://` link in the terminal, Vortex opens it in a browser. The browser is resolved in this order:

1. `VORTEX_BROWSER` environment variable
2. `vortex config set browser "firefox"` setting
3. `BROWSER` environment variable
4. OS default browser

When you click a file path, Vortex opens it in an editor:

1. `VORTEX_EDITOR` environment variable
2. `vortex config set editor "code"` setting
3. `VISUAL` environment variable
4. `EDITOR` environment variable
5. OS default file opener

</details>

## Config Reference

Every `.vortex` file has two required fields: `name` and `jobs`.

```yaml
name: my-project    # identifies this running instance

jobs:               # list of tasks to run
  - id: build
    command: go build ./...
```

### Job Fields

| Field     | Required | Description |
|-----------|----------|-------------|
| `id`      | Yes | Unique name for this job. Used in `needs` references. |
| `command` | Yes | What to run. A direct command, or script text when `shell` is set. |
| `label`   | No | Display name in the UI. Defaults to `id`. |
| `shell`   | No | Interpreter for the command (e.g. `bash`, `node`, `python`). |
| `use`     | No | Connect to a shared runtime (`node`, `bun`, `deno`, `csharp`, `go`). |
| `group`   | No | Visual grouping in the UI. Jobs with the same group appear together. |
| `needs`   | No | List of job IDs that must finish before this job starts. |
| `if`      | No | When to run: `success` (default), `failure`, or `always`. |
| `restart` | No | Set to `false` to keep long-running jobs alive across config reloads. |

### Shells

When `shell` is omitted, Vortex splits `command` into words and runs it directly.
When `shell` is set, Vortex passes `command` as a script to that interpreter.

Supported shells: `bash`, `sh`, `zsh`, `fish`, `cmd`, `powershell`, `pwsh`, `python`, `python3`, `node`, `bun`, `deno`, `csharp`, `go`

<details>
<summary><strong>OS-specific commands and shells</strong></summary>

Both `shell` and `command` can be objects with OS-specific values:

```yaml
jobs:
  - id: cross-platform
    shell:
      default: bash
      windows: pwsh
    command:
      default: echo hello from vortex
      windows: Write-Host hello from vortex
```

Supported keys: `darwin`, `linux`, `windows`, `default`

</details>

### Dependencies

Use `needs` to control execution order:

```yaml
jobs:
  - id: build
    command: go build ./...

  - id: test
    command: go test ./...
    needs: [build]       # waits for build to finish

  - id: deploy
    command: ./deploy.sh
    needs: [test]        # waits for test to finish
    if: success          # only runs if test succeeded
```

The `if` field controls when a dependent job runs:
- `success` (default) — run only if all dependencies succeeded
- `failure` — run only if any dependency failed
- `always` — run regardless of dependency outcome

### Groups

Organize related jobs visually:

```yaml
jobs:
  - id: build
    command: go build ./...
    group: ci

  - id: test
    command: go test ./...
    group: ci

  - id: server
    command: go run ./cmd/server
    group: services
```

### Top-Level Fields

| Field    | Type     | Description |
|----------|----------|-------------|
| `name`   | `string` | **Required.** Unique instance name. |
| `node`   | `object` | Shared Node.js runtime. See [JavaScript Runtimes Guide](docs/runtimes-javascript.md). |
| `bun`    | `object` | Shared Bun runtime. See [JavaScript Runtimes Guide](docs/runtimes-javascript.md). |
| `deno`   | `object` | Shared Deno runtime. See [JavaScript Runtimes Guide](docs/runtimes-javascript.md). |
| `csharp` | `object` | Shared C# runtime. See [C# Runtime Guide](docs/runtimes-csharp.md). |
| `go`     | `object` | Shared Go runtime. See [Go Runtime Guide](docs/runtimes-go.md). |
| `jobs`   | `Job[]`  | **Required.** List of jobs to run. |

## Shared Runtimes

Shared runtimes let you define **imports, variables, and helper functions once** and reuse them across multiple jobs. This avoids repeating setup code in every job.

Vortex supports five runtime environments:

| Runtime | Guide | TypeScript |
|---------|-------|------------|
| **Node** | [JavaScript Runtimes Guide](docs/runtimes-javascript.md) | Yes (via esbuild) |
| **Bun** | [JavaScript Runtimes Guide](docs/runtimes-javascript.md) | Yes (native) |
| **Deno** | [JavaScript Runtimes Guide](docs/runtimes-javascript.md) | Yes (native) |
| **C#** | [C# Runtime Guide](docs/runtimes-csharp.md) | — |
| **Go** | [Go Runtime Guide](docs/runtimes-go.md) | — |

### Quick Example (Node)

```yaml
name: dev

node:
  vars:
    apiBase: http://localhost:3000

  functions:
    logBanner: |
      export function logBanner(text) {
        console.log(`== ${text} ==`)
      }

jobs:
  - id: check-api
    shell: node
    use: node
    command: |
      logBanner(`Checking ${apiBase}`)
      const resp = await fetch(apiBase)
      console.log(resp.status)
```

Both `logBanner` and `apiBase` are available in every job that has `use: node`.

<details>
<summary><strong>How shared runtimes work under the hood</strong></summary>

When a job uses a shared runtime, Vortex generates wrapper files in `~/.cache/vortex/{runtime}-runtime/`:

- **JavaScript (Node/Bun/Deno)**: generates ESM `.mjs` (or `.mts` for TypeScript) modules that re-export all shared code, then runs the wrapper with the appropriate runtime binary.
- **C#**: generates a .NET project with a `Shared.cs` class and a `Program.cs` per job, then runs via `dotnet run`.
- **Go**: generates a Go project with `shared.go` and `main.go` per job, then runs via `go run .`.

If you change a runtime block and reload the config, all opted-in jobs restart automatically to pick up the changes — even persistent jobs with `restart: false`.

</details>

For full documentation on each runtime, see the linked guides above.

## CLI Reference

### Running

```sh
vortex run dev.vortex           # run a config file
vortex run dev                  # shorthand (finds dev.vortex)
vortex run --headless dev       # run without opening a window
```

### Creating Configs

```sh
vortex init                     # create a template .vortex file
vortex init my-app              # create my-app.vortex
vortex init --force             # overwrite existing file
```

### Managing Instances

```sh
vortex instance list            # show running instances
vortex instance quit dev        # stop an instance
vortex instance kill dev        # kill child processes only
vortex instance rerun dev build # rerun a job and its dependents
vortex instance show dev        # open the native window
vortex instance hide dev        # close the window (keep running)
```

### Settings

```sh
vortex config list              # show all settings
vortex config set browser firefox
vortex config set editor code
vortex config get editor
vortex config unset browser
```

### Other

```sh
vortex docs                     # open built-in documentation
vortex docs --force             # regenerate docs
vortex upgrade                  # upgrade to latest release
vortex upgrade --check          # check for updates without installing
vortex version                  # print version
vortex help                     # show help
```

<details>
<summary><strong>All CLI flags</strong></summary>

| Flag | Applies To | Description |
|------|------------|-------------|
| `--config` | `run`, `instance quit/kill/show/hide` | Path to a `.vortex` config file |
| `--cwd` | `run` | Working directory for all jobs (defaults to config file directory) |
| `--force` | `init`, `docs` | Overwrite existing files |
| `--port` | `run` | Override the HTTP port |
| `--headless` | `run` | Run without opening a native window |
| `--dev` | `run` | Development mode: skip webview, use browser at localhost |
| `--json` | `instance list` | Output as JSON |
| `--prune` | `instance list` | Remove stale instances |
| `--no-open` | `docs` | Generate docs without opening browser |

</details>

<details>
<summary><strong>Instance management details</strong></summary>

**Restarting**: running `vortex run` with a config that has the same `name` as an already-running instance restarts it in place.

**Ports**: Vortex derives both the handoff port and UI port from the config name, so different named configs run simultaneously without port conflicts.

**`instance list` output** includes:
- `mode`: `dev`, `headless`, or `windowed`
- `ui`: `open`, `hidden`, or `none`
- `started` / `updated` / `last_control` timestamps
- `generation`: orchestrator restart count
- `reachable` (in `--json`): whether the instance API responded

**`instance list --prune`**: removes stale registry entries and makes a best-effort attempt to terminate orphaned processes.

**`show` / `hide`**: `show` surfaces the native webview for headless instances. `hide` closes the window without stopping jobs. These only apply to non-`--dev` instances.

</details>

<details>
<summary><strong>Upgrading details</strong></summary>

`vortex upgrade` downloads the latest GitHub release for your OS/architecture and installs it to:

- macOS/Linux: `~/.local/bin/vortex`
- Windows: `%LOCALAPPDATA%\Programs\Vortex\vortex.exe`

The upgrade process:
- Downloads the release binary and verifies its SHA-256 checksum
- Stops a running instance before replacing the binary
- On macOS: sets executable permissions and removes the quarantine attribute
- Attempts to add the install directory to your PATH

After upgrading, open a new terminal session so the updated PATH takes effect.

</details>

### VS Code Schema

For autocompletion in `.vortex` files, add this to your VS Code settings:

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

<details>
<summary><strong>Pinning to a specific version</strong></summary>

To avoid breaking changes from schema updates, pin to a release tag:

```json
{
  "yaml.schemas": {
    "https://raw.githubusercontent.com/arcmantle/vortex/v1.0.10/schemas/vortex.schema.json": [
      "*.vortex"
    ]
  }
}
```

For SchemaStore submission, a ready-to-submit catalog entry is at `schemas/vortex.schemastore-entry.json`.

</details>

## Building From Source

<details>
<summary><strong>Prerequisites</strong></summary>

- Go 1.24+
- Node.js 20+ / pnpm
- C compiler (CGo is required for the webview binding)
  - **Windows:** MSVC or MinGW — WebView2 headers are bundled
  - **macOS:** Xcode command-line tools
  - **Linux:** `libwebkit2gtk-4.1-dev`

</details>

### Development

```sh
# Start the Go server + Vite dev server
go run ./cmd/vortex --dev --config mock/dev.vortex &
cd cmd/vortex-ui/web && pnpm install && pnpm dev
```

The Vite dev server proxies API calls to the Go backend. Use `--dev` to skip the native window and work in the browser at `http://localhost:5173`.

### Production Binary

```sh
# Build the frontend
cd cmd/vortex-ui/web && pnpm install && pnpm build && cd ../../..

# Build the Go binary (UI is embedded by default)
go build -o vortex ./cmd/vortex
```

<details>
<summary><strong>Windows and macOS notes</strong></summary>

On Windows, add `-ldflags "-H=windowsgui"` to build a GUI binary that doesn't keep a console window open:

```sh
go build -ldflags "-H=windowsgui" -o vortex.exe ./cmd/vortex
```

CLI commands (`help`, `version`, `config`, etc.) still work from the terminal — Vortex reattaches to the parent console when needed.

If you download the macOS release binary from GitHub, make it executable and remove the quarantine attribute:

```sh
chmod +x ./vortex-darwin-arm64
xattr -d com.apple.quarantine ./vortex-darwin-arm64 2>/dev/null || true
```

The UI is embedded by default. To build without it (e.g. for the two-stage release process), use `-tags no_embed_ui`.

</details>

## Architecture

<details>
<summary><strong>Project structure and data flow</strong></summary>

```
cmd/vortex/          CLI entry point, UI embedding
cmd/vortex-ui/web/   Lit + TypeScript frontend (xterm.js terminals)
internal/
  config/            Config loading, validation, runtime code generation
  instance/          Named-instance lock, port derivation, handoff registry
  orchestrator/      Dependency-aware job graph execution
  terminal/          Process lifecycle, output buffering, ring buffer
  server/            HTTP API, SSE streaming, static file serving
  webview/           Platform-specific native webview wrappers
```

**Data flow:** Orchestrator → Terminal (captures stdout/stderr into ring buffer) → SSE endpoint → xterm.js in the webview.

</details>

## License

MIT
