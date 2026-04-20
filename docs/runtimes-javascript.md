# JavaScript Runtimes (Node, Bun, Deno)

Vortex lets you run JavaScript (and TypeScript) directly from your config file. You can write quick one-off scripts, or set up a **shared runtime** so that multiple jobs can reuse the same imports, variables, and helper functions.

Three runtimes are supported: **Node**, **Bun**, and **Deno**. They all work the same way in Vortex — just swap the name.

## Running a Simple Script

The quickest way to run JavaScript is with `shell: node` (or `bun` / `deno`) and an inline `command`:

```yaml
name: dev

jobs:
  - id: hello
    shell: node
    command: console.log("hello from vortex")
```

This runs via `node -e` (or the equivalent for Bun/Deno). No files are generated.

## Sharing Code Across Jobs

When you have multiple jobs that need the same setup — shared variables, imports, or helper functions — declare a **runtime block** at the top level and connect jobs to it with `use`:

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

  - id: check-health
    shell: node
    use: node
    command: |
      logBanner("Health check")
      const resp = await fetch(apiBase + "/health")
      console.log(resp.ok ? "OK" : "FAIL")
```

Both jobs can use `logBanner` and `apiBase` without repeating them.

**Two things are required** for a job to use the shared runtime:
1. `shell: node` — tells Vortex which interpreter to use
2. `use: node` — connects the job to the shared `node` block

<details>
<summary><strong>How this works under the hood</strong></summary>

When a job has `use: node`, Vortex generates ESM wrapper files in `~/.cache/vortex/node-runtime/`:

- **shared.mjs** — re-exports all imports, vars, and functions from the runtime block
- **{job-id}.mjs** — imports everything from `shared.mjs`, then runs the job's `command`

This gives you full ESM support: top-level `await`, `import` from npm packages, and `node:` built-in modules all work.

If you change the runtime block and reload the config, all connected jobs restart automatically to pick up the changes.

</details>

## Sources

The `sources` field lets you write your code in real files on disk (with full editor support) and make their exports available in all connected jobs:

```yaml
name: dev

node:
  sources:
    - ./lib/http-helpers.mjs

  vars:
    apiBase: http://localhost:3000

jobs:
  - id: smoke
    shell: node
    use: node
    command: |
      const data = await httpHelpers.fetchJson(apiBase + '/health')
      console.log(data)
```

Each source file becomes a **namespace** — a container for all the file's exports. The namespace name is derived from the filename using camelCase:

| File Path | Available As |
|-----------|-------------|
| `./lib/http-helpers.mjs` | `httpHelpers` |
| `./lib/format.mjs` | `format` |
| `./scripts/run-migrations.js` | `runMigrations` |
| `./utils.mjs` | `utils` |

You access exports through the namespace with dot notation: `httpHelpers.fetchJson(...)`, `format.pretty(...)`.

### Example Source File

```javascript
// lib/http-helpers.mjs
export async function fetchJson(url) {
  const resp = await fetch(url)
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
  return resp.json()
}

export function buildUrl(base, path) {
  return new URL(path, base).toString()
}
```

## Imports

The `imports` field adds ESM import statements to the shared runtime. There are four ways to import:

### Named imports — pick specific exports

```yaml
imports:
  - from: node:path
    names: [basename, dirname, join]
```

Use them directly: `basename('/tmp/file.txt')`

### Default import — import the module's default export

```yaml
imports:
  - from: node:fs/promises
    default: fs
```

Use it as: `fs.readFile(...)`

### Namespace import — import everything under one name

```yaml
imports:
  - from: node:os
    namespace: os
```

Use it as: `os.hostname()`

### Aliased imports — rename exports

```yaml
imports:
  - from: node:path
    named:
      basename: base
      dirname: dir
```

Use them as `base(...)` and `dir(...)`

<details>
<summary><strong>What you can import</strong></summary>

The `from` field accepts:
- **Node built-ins**: `node:path`, `node:fs/promises`, `node:os`, etc.
- **npm packages**: `express`, `chalk`, `lodash` (resolved from `node_modules`)
- **Local files**: `./lib/helpers.mjs`, `../utils.js`
- **URLs** (Deno only): `https://deno.land/std/path/mod.ts`
- **npm: specifiers** (Deno only): `npm:chalk`

Each import must use exactly one of `default`, `namespace`, `names`, or `named`.

</details>

## Variables

Variables declared in `vars` are available as constants in all connected jobs:

```yaml
node:
  vars:
    apiBase: http://localhost:3000
    port: 3000
    debug: true
    tags:
      - alpha
      - beta
```

All YAML types are preserved: strings, numbers, booleans, arrays, and objects.

## Functions

Functions let you define reusable helpers directly in the config:

```yaml
node:
  functions:
    logBanner: |
      export function logBanner(text) {
        console.log(`\n== ${text} ==\n`)
      }

    retry: |
      export async function retry(fn, attempts = 3) {
        for (let i = 0; i < attempts; i++) {
          try { return await fn() }
          catch (e) { if (i === attempts - 1) throw e }
        }
      }
```

**Rules for functions:**
- Each function **must** use `export function`
- The function name **must** match the YAML key

## TypeScript

All three runtimes support TypeScript — both as source files and inline in commands.

### Using TypeScript Source Files

Use `.ts` or `.mts` files in the `sources` field. Vortex auto-detects TypeScript from the file extension:

```yaml
name: dev

node:
  sources:
    - ./lib/math-helper.ts

jobs:
  - id: compute
    shell: node
    use: node
    command: |
      const result = mathHelper.add(3, 4)
      console.log(result)
```

```typescript
// lib/math-helper.ts
export function add(a: number, b: number): number {
  return a + b;
}
```

### Writing Inline TypeScript

Set `typescript: true` to write TypeScript directly in functions and job commands:

```yaml
name: dev

node:
  typescript: true
  functions:
    greet: |
      export function greet(name: string): string {
        return `hello ${name}`;
      }

jobs:
  - id: smoke
    shell: node
    use: node
    command: |
      const msg: string = greet("world")
      console.log(msg)
```

### When Is TypeScript Enabled?

TypeScript mode turns on automatically when:
- Any source file has a `.ts`, `.mts`, or `.cts` extension
- You set `typescript: true` explicitly (needed for inline-only TS with no `.ts` sources)

<details>
<summary><strong>How TypeScript works per runtime</strong></summary>

| Runtime | What Happens |
|---------|-------------|
| **Node** | Vortex generates `.mts` wrapper files, then uses esbuild (built into the Vortex binary) to bundle them into a single `.mjs` file. Type annotations are stripped — no downleveling. The bundled `.mjs` is then run with `node`. |
| **Bun** | Vortex generates `.mts` wrapper files. Bun runs TypeScript natively — no bundling needed. |
| **Deno** | Vortex generates `.mts` wrapper files. Deno runs TypeScript natively — no bundling needed. |

For Node, bare package imports (like `express` or `chalk`) are kept external in the bundle and resolved at runtime from `node_modules`. Only the local shared code and source files are inlined.

</details>

## Using Bun or Deno

Bun and Deno work exactly the same way — just swap the runtime name:

**Bun:**

```yaml
name: dev

bun:
  sources:
    - ./lib/helpers.ts
  vars:
    apiBase: http://localhost:3000

jobs:
  - id: smoke
    shell: bun
    use: bun
    command: |
      console.log(helpers.greet("world"))
```

**Deno:**

```yaml
name: dev

deno:
  imports:
    - from: https://deno.land/std@0.220.0/path/mod.ts
      names: [basename]
  vars:
    apiBase: http://localhost:3000

jobs:
  - id: smoke
    shell: deno
    use: deno
    command: console.log(basename("/tmp/demo.txt"))
```

<details>
<summary><strong>Deno-specific features</strong></summary>

Deno supports importing from URLs and `npm:` specifiers in the `from` field:

```yaml
deno:
  imports:
    - from: https://deno.land/std@0.220.0/path/mod.ts
      names: [basename]
    - from: npm:chalk
      default: chalk
```

Shared-runtime Deno jobs are executed with `deno run --allow-all`.

</details>

## Using Multiple Runtimes

You can use different runtimes in the same config. Each job only sees its own runtime's shared code:

```yaml
name: dev

node:
  vars:
    port: 3000

bun:
  vars:
    port: 3001

jobs:
  - id: node-task
    shell: node
    use: node
    command: console.log(`node on port ${port}`)

  - id: bun-task
    shell: bun
    use: bun
    command: console.log(`bun on port ${port}`)
```

## When to Use Sources vs. Inline Functions

**Use source files** when:
- Code is complex (more than ~10 lines)
- You want editor support (syntax highlighting, linting, type checking)
- Code is shared with other tools, not just Vortex

**Use inline functions** when:
- Helpers are short (1–5 lines)
- They're Vortex-specific glue code
- You want the entire config to be self-contained

You can combine both:

```yaml
node:
  sources:
    - ./lib/complex-logic.mjs

  functions:
    wrap: |
      export function wrap(text) {
        return `[${text}]`
      }
```

## Tips

- **Hot reload** — changing a runtime block restarts all connected jobs automatically
- **Top-level `await`** — works in all three runtimes
- **npm packages** — use `from: "package-name"` in imports (Node/Bun resolve from `node_modules`, Deno uses `npm:` specifiers)
- **No `use`** — jobs with `shell: node` but no `use: node` run via simple `node -e` with no shared runtime or file generation
- **Error handling** — if a job's command throws, the job exits with a non-zero code
