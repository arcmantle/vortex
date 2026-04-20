# Go Runtime

Vortex lets you run Go directly from your config file. You can write quick inline scripts, or set up a **shared runtime** so that multiple jobs can reuse the same variables, functions, and dependencies.

Under the hood, Vortex generates a Go project and runs it with `go run .`. Go's build cache means the first run compiles the binary, but subsequent runs reuse the cached build and start instantly.

## Running a Simple Script

The quickest way to run Go is with `shell: go` and an inline `command`:

```yaml
name: dev

jobs:
  - id: hello
    shell: go
    command: fmt.Println("hello from vortex")
```

Your command becomes the body of `func main()`. You don't need to write import statements — Vortex auto-detects standard library usage from your code.

<details>
<summary><strong>Using your own func main()</strong></summary>

For full control, include your own `func main()` and imports:

```yaml
jobs:
  - id: custom
    shell: go
    command: |
      import (
          "fmt"
          "os"
      )

      func main() {
          fmt.Println("HOME:", os.Getenv("HOME"))
      }
```

When Vortex detects `func main()` in the command, it uses the command as-is (after `package main`).

</details>

## Sharing Code Across Jobs

When you have multiple jobs that need the same setup — shared variables or helper functions — declare a **`go` block** at the top level and connect jobs to it with `use`:

```yaml
name: dev

go:
  vars:
    apiBase: http://localhost:3000

  functions:
    logBanner: |
      func logBanner(text string) {
      	fmt.Printf("== %s ==\n", text)
      }

jobs:
  - id: check-api
    shell: go
    use: go
    command: |
      logBanner(fmt.Sprintf("Checking %s", apiBase))
      resp, err := http.Get(apiBase)
      if err != nil {
      	log.Fatal(err)
      }
      fmt.Println("Status:", resp.Status)

  - id: check-health
    shell: go
    use: go
    command: |
      logBanner("Health check")
      resp, err := http.Get(apiBase + "/health")
      if err != nil {
      	log.Fatal(err)
      }
      fmt.Println("OK:", resp.StatusCode == 200)
```

Both jobs can use `logBanner` and `apiBase` without repeating them.

**Two things are required** for a job to use the shared runtime:
1. `shell: go` — tells Vortex to generate a Go project
2. `use: go` — connects the job to the shared `go` block

<details>
<summary><strong>How this works under the hood</strong></summary>

When a job has `use: go`, Vortex generates a Go project in `~/.cache/vortex/go-runtime/`:

- **go.mod** — module declaration with `require` entries for external dependencies
- **shared.go** — package-level `var` declarations and function definitions
- **main.go** — the job's command wrapped in `func main()`

All files share `package main`, so everything in `shared.go` and source files is directly accessible in the job command.

Standard library imports are auto-detected in both `shared.go` and `main.go`.

If you change the `go` block and reload the config, all connected jobs restart automatically.

</details>

## Auto-Import Detection

Vortex scans your code for standard library package references and automatically adds the right import statements. You don't need to write imports for common packages:

```yaml
jobs:
  - id: example
    shell: go
    use: go
    command: |
      fmt.Println("hello")           # auto-imports "fmt"
      resp, _ := http.Get(apiBase)   # auto-imports "net/http"
      log.Println(resp.Status)       # auto-imports "log"
```

<details>
<summary><strong>Full list of auto-detected packages</strong></summary>

| Usage in Code | Import Added |
|---------------|-------------|
| `fmt.` | `"fmt"` |
| `os.` | `"os"` |
| `strings.` | `"strings"` |
| `strconv.` | `"strconv"` |
| `filepath.` | `"path/filepath"` |
| `time.` | `"time"` |
| `io.` | `"io"` |
| `log.` | `"log"` |
| `http.` | `"net/http"` |
| `json.` | `"encoding/json"` |
| `bytes.` | `"bytes"` |
| `sort.` | `"sort"` |
| `sync.` | `"sync"` |
| `context.` | `"context"` |
| `errors.` | `"errors"` |
| `regexp.` | `"regexp"` |
| `math.` | `"math"` |
| `bufio.` | `"bufio"` |

For packages not in this list, use source files with their own import statements.

</details>

## Sources

The `sources` field lets you write your code in real `.go` files on disk (with full gopls support) and make their functions available in all connected jobs:

```yaml
name: dev

go:
  sources:
    - ./lib/http_helpers.go

  vars:
    apiBase: http://localhost:3000

jobs:
  - id: smoke
    shell: go
    use: go
    command: |
      data, err := FetchJSON(apiBase + "/health")
      if err != nil {
      	log.Fatal(err)
      }
      fmt.Println(PrettyFormat(data))
```

Source files are copied into the generated project. Since they share `package main`, any exported functions and types are directly available in job commands.

### Example Source File

```go
// lib/http_helpers.go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func FetchJSON(url string) (map[string]any, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result, nil
}
```

<details>
<summary><strong>Important: source files must use package main</strong></summary>

Source files are compiled together with the generated code, so they must declare `package main`. Functions should be exported (capitalized) to be callable from job commands.

Inline functions defined in the `functions` field can be unexported (lowercase) since they're in the same package.

</details>

### When to Use Sources vs. Inline Functions

**Use source files** when:
- Functions are complex (more than ~10 lines) or need their own imports
- You want full IDE support (gopls, jump-to-definition, testing)
- Code is shared with other Go projects

**Use inline functions** when:
- Helpers are short (1–5 lines)
- They only use auto-detected standard library packages
- You want the config to be self-contained

You can combine both:

```yaml
go:
  sources:
    - ./lib/complex_logic.go

  functions:
    wrap: |
      func wrap(text string) string {
      	return "[" + text + "]"
      }
```

## Module Dependencies

The `imports` field adds external packages to the generated `go.mod`:

```yaml
go:
  imports:
    - path: github.com/fatih/color
      version: v1.16.0
    - path: github.com/go-resty/resty/v2
      version: v2.11.0
```

Dependencies are downloaded automatically on first run and cached by the Go module system.

> **Note**: standard library packages don't need to be listed — they're auto-detected.

## Variables

Variables declared in `vars` are directly accessible in all connected jobs:

```yaml
go:
  vars:
    apiBase: http://localhost:3000
    port: 3000
    debug: true
    ratio: 0.75
```

<details>
<summary><strong>How variable types are inferred</strong></summary>

Types are mapped from YAML:
- Strings → `string`
- Integers → `int`
- Booleans → `bool`
- Floating point → `float64`

Vars become package-level `var` declarations in `shared.go` and are directly accessible since everything shares `package main`.

</details>

## Functions

Functions let you define reusable helpers directly in the config:

```yaml
go:
  functions:
    logBanner: |
      func logBanner(text string) {
      	fmt.Printf("\n== %s ==\n\n", text)
      }

    measureTime: |
      func measureTime(label string, fn func()) {
      	start := time.Now()
      	fn()
      	fmt.Printf("%s: %v\n", label, time.Since(start))
      }
```

**Rules for functions:**
- Must be valid Go function declarations (start with `func`)
- Can reference auto-detected standard library packages, shared vars, and other functions
- The YAML key is used for conflict detection with vars

## Inline Go Without a Shared Runtime

Jobs with `shell: go` but **without** `use: go` also run via `go run .` — they just don't get shared vars or functions:

```yaml
name: dev

jobs:
  - id: quick-check
    shell: go
    command: |
      resp, err := http.Get("http://localhost:3000/health")
      if err != nil {
      	log.Fatal(err)
      }
      fmt.Println("Status:", resp.Status)
```

Standard library imports are still auto-detected.

## Module Name

The `module` field controls the Go module path:

```yaml
go:
  module: mycompany/tooling  # defaults to vortex/runtime
```

<details>
<summary><strong>When this matters</strong></summary>

This sets the `module` line in the generated `go.mod`. In practice it rarely matters since the generated code isn't imported by anything else. It can be useful for organization or if you have replace directives that depend on the module path.

</details>

## Tips

- **Fast rebuilds** — Go's build cache (`GOCACHE`) is shared across all runs; only changed source files trigger recompilation
- **Hot reload** — changing the `go` block restarts all connected jobs automatically
- **Concurrency** — Go's goroutines and channels work normally in job commands
- **Cross-platform** — works on macOS, Linux, and Windows wherever `go` is installed
- **No `use`** — jobs with `shell: go` but no `use: go` run on their own project with no shared runtime
