# C# Runtime

Vortex lets you run C# directly from your config file. You can write quick inline scripts, or set up a **shared runtime** so that multiple jobs can reuse the same packages, variables, and helper methods.

Under the hood, Vortex generates a .NET project and runs it with `dotnet run`. The first run takes a few seconds to compile, but subsequent runs reuse the cached build output and are near-instant.

## Running a Simple Script

The quickest way to run C# is with `shell: csharp` and an inline `command`:

```yaml
name: dev

jobs:
  - id: hello
    shell: csharp
    command: Console.WriteLine("hello from vortex");
```

Your command uses [top-level statements](https://learn.microsoft.com/en-us/dotnet/csharp/fundamentals/program-structure/top-level-statements) — no `Main` method or class wrapper needed. `await` works at the top level.

## Sharing Code Across Jobs

When you have multiple jobs that need the same setup — shared packages, variables, or helper methods — declare a **`csharp` block** at the top level and connect jobs to it with `use`:

```yaml
name: dev

csharp:
  usings:
    - System.IO
  vars:
    apiBase: http://localhost:3000
  functions:
    LogBanner: |
      public static void LogBanner(string text)
      {
          Console.WriteLine($"== {text} ==");
      }

jobs:
  - id: check-api
    shell: csharp
    use: csharp
    command: |
      LogBanner($"Checking {apiBase}");
      using var client = new HttpClient();
      var resp = await client.GetAsync(apiBase);
      Console.WriteLine(resp.StatusCode);

  - id: check-paths
    shell: csharp
    use: csharp
    command: |
      Console.WriteLine(Path.GetFileName("/tmp/demo.txt"));
```

Both jobs can use `LogBanner` and `apiBase` without repeating them.

**Two things are required** for a job to use the shared runtime:
1. `shell: csharp` — tells Vortex to generate a .NET project
2. `use: csharp` — connects the job to the shared `csharp` block

<details>
<summary><strong>How this works under the hood</strong></summary>

When a job has `use: csharp`, Vortex generates a .NET project in `~/.cache/vortex/csharp-runtime/`:

- **project.csproj** — targets the specified framework with NuGet package references
- **Shared.cs** — a static `Vortex` class containing vars as `public static readonly` fields and functions as static methods
- **Program.cs** — the job's command as top-level statements

The `Shared.cs` is included in all connected jobs, and `using static Vortex;` is auto-injected so vars and functions are available without a class prefix.

The project preserves `bin/` and `obj/` directories across runs, so only changed files trigger recompilation.

If you change the `csharp` block and reload the config, all connected jobs restart automatically.

</details>

## Sources

The `sources` field lets you write your code in real `.cs` files on disk (with full IntelliSense) and make their types available in all connected jobs:

```yaml
name: dev

csharp:
  sources:
    - ./lib/HttpHelpers.cs

  vars:
    apiBase: http://localhost:3000

jobs:
  - id: smoke
    shell: csharp
    use: csharp
    command: |
      var data = await HttpHelpers.FetchJson(apiBase + "/health");
      Console.WriteLine(data);
```

Source files are copied into the generated project. Since everything compiles together, any `public` classes and methods are directly available in job commands.

### Example Source File

```csharp
// lib/HttpHelpers.cs
using System.Net.Http;
using System.Text.Json;

public static class HttpHelpers
{
    private static readonly HttpClient _client = new();

    public static async Task<JsonDocument> FetchJson(string url)
    {
        var response = await _client.GetAsync(url);
        response.EnsureSuccessStatusCode();
        var stream = await response.Content.ReadAsStreamAsync();
        return await JsonDocument.ParseAsync(stream);
    }
}
```

### When to Use Sources vs. Inline Functions

**Use source files** when:
- Classes are complex with multiple methods
- You want full IDE support (IntelliSense, refactoring, debugging)
- Code is shared with other .NET projects

**Use inline functions** when:
- Helpers are short (1–10 lines)
- They're Vortex-specific glue code
- You want the config to be self-contained

You can combine both:

```yaml
csharp:
  sources:
    - ./lib/ComplexLogic.cs

  functions:
    Wrap: |
      public static string Wrap(string text)
      {
          return $"[{text}]";
      }
```

## Usings

The `usings` field adds `using` directives to the shared source file:

```yaml
csharp:
  usings:
    - System.IO
    - System.Text.Json
    - System.Net.Http
```

<details>
<summary><strong>What's already available without adding usings</strong></summary>

.NET's implicit usings are enabled by default, which includes:
- `System`
- `System.Collections.Generic`
- `System.Linq`
- `System.Threading.Tasks`

You only need to add usings for namespaces not covered by implicit usings.

</details>

## Packages

The `packages` field adds NuGet package references:

```yaml
csharp:
  packages:
    - name: Newtonsoft.Json
      version: "13.0.3"
    - name: Dapper
      version: "2.1.28"
```

NuGet restore happens automatically on the first `dotnet run`. Packages are cached by the .NET SDK as usual.

## Variables

Variables declared in `vars` are available directly in all connected jobs:

```yaml
csharp:
  vars:
    apiBase: http://localhost:3000
    port: 3000
    debug: true
```

<details>
<summary><strong>How variable types are inferred</strong></summary>

Types are mapped from YAML:
- Strings → `string`
- Integers → `int`
- Booleans → `bool`
- Floating point → `double`

Vars become `public static readonly` fields on the generated `Vortex` class, and `using static Vortex;` makes them available without qualification.

</details>

## Functions

Functions let you define reusable methods directly in the config:

```yaml
csharp:
  functions:
    LogBanner: |
      public static void LogBanner(string text)
      {
          Console.WriteLine($"\n== {text} ==\n");
      }

    MeasureTime: |
      public static async Task<T> MeasureTime<T>(string label, Func<Task<T>> action)
      {
          var sw = System.Diagnostics.Stopwatch.StartNew();
          var result = await action();
          sw.Stop();
          Console.WriteLine($"{label}: {sw.ElapsedMilliseconds}ms");
          return result;
      }
```

**Rules for functions:**
- Must be valid C# `public static` method declarations
- Are pasted into the generated `Vortex` class as-is
- The YAML key should match the method name

## Inline C# Without a Shared Runtime

Jobs with `shell: csharp` but **without** `use: csharp` also run via `dotnet run` — they just don't get the shared vars or functions:

```yaml
name: dev

jobs:
  - id: quick-check
    shell: csharp
    command: |
      using var client = new HttpClient();
      var resp = await client.GetStringAsync("http://localhost:3000/health");
      Console.WriteLine(resp);
```

This is useful for one-off scripts that don't need shared state.

## Framework Targeting

The `framework` field controls the .NET target framework:

```yaml
csharp:
  framework: net9.0  # defaults to net8.0
```

<details>
<summary><strong>How this maps to the generated project</strong></summary>

This sets the `<TargetFramework>` in the generated `.csproj`. You need the corresponding .NET SDK installed (e.g. .NET 9.0 SDK for `net9.0`).

</details>

## Tips

- **Fast rebuilds** — the generated project keeps `bin/` and `obj/` cached; only changed files trigger recompilation
- **Hot reload** — changing the `csharp` block restarts all connected jobs automatically
- **Top-level statements** — job commands are top-level C# code; `await` works directly
- **NuGet cache** — packages are cached globally by the .NET SDK, so they're only downloaded once
- **Cross-platform** — works on macOS, Linux, and Windows wherever `dotnet` is installed
- **No `use`** — jobs with `shell: csharp` but no `use: csharp` run on their own project with no shared runtime
