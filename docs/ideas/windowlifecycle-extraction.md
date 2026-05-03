# windowlifecycle extraction

## Problem Statement
How might we give `internal/appkit` its own Go module so it gets independent build configuration, matching the established pattern of `windowfocus` and `windowicon`?

## Recommended Direction
Mirror-extract `internal/appkit` to a new top-level `windowlifecycle/` directory with its own `go.mod` (module `arcmantle/windowlifecycle`, go 1.23, zero dependencies). Follow the exact same structure as `windowfocus` and `windowicon`:

- `lifecycle.go` — public API (`Configure`, `InstallWindowDelegate`, `ShowWindow`, `Event` type, constants)
- `lifecycle_darwin.go` — CGO + Objective-C implementation (current `appkit_darwin.go` + `appkit_darwin.m`)
- `lifecycle_nocgo.go` — `!cgo` no-ops (currently missing, new)
- `lifecycle_other.go` — `!darwin` stubs (current `appkit_other.go`)

Root `go.mod` gets a `replace arcmantle/windowlifecycle => ./windowlifecycle` directive. `cmd/vortex-window/lifecycle_darwin.go` updates its import from `arcmantle/vortex/internal/appkit` to `arcmantle/windowlifecycle`.

This is a pure structural refactor — zero behavioral change.

## Key Assumptions to Validate
- [ ] The `<-chan Event` pattern remains sufficient as more AppKit events are added — monitor during next few additions
- [ ] No second module outside `arcmantle/vortex` needs to import this today — if that changes, the module path is already clean
- [ ] Adding `_nocgo.go` stubs doesn't mask real build failures — keep CGO as the expected darwin build path

## MVP Scope
- Create `windowlifecycle/` directory with `go.mod`, `lifecycle.go`, platform files
- Move Objective-C source (`appkit_darwin.m`) into the new package
- Add `lifecycle_nocgo.go` stub (missing today)
- Add `replace` directive in root `go.mod`
- Update import in `cmd/vortex-window/lifecycle_darwin.go`
- Delete `internal/appkit/`
- Verify: `go build ./...` passes on darwin, `GOOS=linux go build ./cmd/vortex/` still works

## Not Doing (and Why)
- **Renaming Event/Config types** — unnecessary churn, the names are fine
- **Adding an interface layer** — only one consumer, indirection isn't justified
- **Merging with windowfocus/windowicon** — they're independent concerns with different change frequencies
- **Event bus / multi-subscriber pattern** — over-engineering for a single consumer; revisit when there's a real second subscriber
- **Publishing to a registry** — `replace` directive is the established pattern here; no external consumers exist

## Open Questions
- Should file naming use `lifecycle_` prefix (matching package name) or `windowlifecycle_` prefix? `lifecycle_` matches how `windowfocus` uses `focus_` and `windowicon` uses `icon_`.
