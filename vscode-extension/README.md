# Vortex Language Support

Full language support for [Vortex](https://github.com/arcmantle/vortex) configuration files (`.vortex`).

Write your orchestration configs with **syntax highlighting**, **intellisense**, and **YAML validation** — all from a single extension.

---

## Features

### Syntax Highlighting

Embedded code blocks are highlighted using full TextMate grammars, matching your current VS Code color theme.

Supported languages:

- **JavaScript** / TypeScript
- **Go**
- **C#**

The surrounding YAML structure gets its own highlighting too — keys, strings, comments, block scalar indicators — so the entire file looks right.

### Intellisense

Rich editor features for all three embedded languages:

| Feature | JavaScript | Go | C# |
|---------|:---:|:---:|:---:|
| **Hover** (type info, docs) | ✓ | ✓ | ✓ |
| **Completions** | ✓ | ✓ | ✓ |
| **Go to Definition** | ✓ | ✓ | ✓ |
| **Signature Help** | ✓ | ✓ | ✓ |

Intellisense is aware of your shared runtime configuration — **vars**, **functions**, and **imports** defined at the top level are available inside job commands, just like they are at runtime.

Each language backend starts lazily. If your config only uses Go, the JavaScript and C# backends never load.

### YAML Validation & Completions

A bundled YAML language server provides:

- **Schema validation** against the official Vortex JSON schema
- **YAML key completions** for all config fields (`name`, `jobs`, `needs`, `shell`, etc.)
- **Hover docs** for YAML keys
- **Formatting**

YAML features are automatically suppressed inside embedded code blocks so they don't interfere with language-specific intellisense.

### File Icon

`.vortex` files get a custom file icon in the explorer and editor tabs, with separate light and dark theme variants.

---

## Supported Runtimes

The extension understands all five Vortex shared runtimes:

| Runtime | `shell` value | Shared block | Intellisense backend |
|---------|---------------|--------------|----------------------|
| Node.js | `node` | `node:` | TypeScript LanguageService |
| Bun | `bun` | `bun:` | TypeScript LanguageService |
| Deno | `deno` | `deno:` | TypeScript LanguageService |
| Go | `go` | `go:` | [gopls](https://pkg.go.dev/golang.org/x/tools/gopls) |
| C# | `csharp` | `csharp:` | [Roslyn](https://github.com/dotnet/roslyn) |

### Prerequisites

- **JavaScript**: No extra setup needed — uses VS Code's built-in TypeScript engine.
- **Go**: Requires [gopls](https://pkg.go.dev/golang.org/x/tools/gopls) installed (`go install golang.org/x/tools/gopls@latest`).
- **C#**: Requires the [C# extension](https://marketplace.visualstudio.com/items?itemName=ms-dotnettools.csharp) installed (the Roslyn server is bundled with it).

---

## Commands

| Command | Description |
|---------|-------------|
| **Vortex: View Assembled Source** | Opens the assembled source file for the current `.vortex` file in a side editor. Useful for debugging intellisense behavior. |

Access it from the Command Palette or the editor title bar icon when editing a `.vortex` file.

---

## How It Works

When you open a `.vortex` file, the extension:

1. **Parses** the YAML structure to find embedded code regions (`command:`, `functions:`, `vars:`)
2. **Assembles** a complete, valid source file for the detected language — including imports, variables, and function definitions from the shared runtime block
3. **Maps positions** bidirectionally between the `.vortex` file and the assembled source using a source map
4. **Routes** editor requests (hover, completions, etc.) through the source map to the appropriate language server
5. **Highlights** code tokens using [Shiki](https://shiki.style/) with the Oniguruma engine, matching your active color theme

All of this happens transparently. You just write your `.vortex` file and get full editor support.

---

## Example

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
  - id: api
    label: API Server
    command: go run ./cmd/server

  - id: check-api
    shell: node
    use: node
    command: |
      logBanner(`Checking ${apiBase}`)
      const resp = await fetch(apiBase)
      console.log(resp.status)

  - id: test
    command: go test ./...
    needs: [api]
```

Inside the `command:` block, you get completions for `logBanner`, `apiBase`, and all Node.js globals — with full type information and hover docs.

---

## Requirements

- VS Code **1.85** or later

For Go and C# intellisense, see [Prerequisites](#prerequisites) above.

---

## Links

- [Vortex on GitHub](https://github.com/arcmantle/vortex)
- [Config Reference](https://github.com/arcmantle/vortex#config-reference)
- [JSON Schema](https://raw.githubusercontent.com/arcmantle/vortex/master/schemas/vortex.schema.json)
