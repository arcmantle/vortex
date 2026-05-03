# Implementation Plan: windowlifecycle extraction

## Overview
Extract `internal/appkit` into a standalone `windowlifecycle/` Go module, matching the established pattern of `windowfocus/` and `windowicon/`. Pure structural refactor — zero behavioral change.

## Architecture Decisions
- **Package name `windowlifecycle`**, file prefix `lifecycle_` — matches how `windowfocus` uses `focus_` and `windowicon` uses `icon_`
- **Module path `arcmantle/windowlifecycle`** with `replace` directive, same as the other two sub-modules
- **Add `lifecycle_nocgo.go`** — currently missing; matches the safety pattern in `windowfocus` and `windowicon`
- **Public API via unexported dispatch functions** — `windowfocus` and `windowicon` use a thin public→unexported indirection (`ShowApp()` → `showApp()`). Appkit currently puts logic directly in the exported functions. Adopt the same indirection for consistency.

## Task List

### Phase 1: Create the new module

- [ ] Task 1: Create `windowlifecycle/go.mod` and `lifecycle.go`
- [ ] Task 2: Create `lifecycle_darwin.go` and `lifecycle_darwin.m`
- [ ] Task 3: Create `lifecycle_nocgo.go` and `lifecycle_other.go`

### Checkpoint: New module compiles
- [ ] `cd windowlifecycle && go build .` passes
- [ ] `cd windowlifecycle && GOOS=linux go build .` passes
- [ ] `cd windowlifecycle && CGO_ENABLED=0 go build .` passes

### Phase 2: Wire up and remove old

- [ ] Task 4: Add `replace` directive and `require` in root `go.mod`
- [ ] Task 5: Update `cmd/vortex-window` imports
- [ ] Task 6: Delete `internal/appkit/`

### Checkpoint: Full build
- [ ] `go build ./...` passes (darwin)
- [ ] `GOOS=linux go build ./cmd/vortex/` passes
- [ ] `go vet ./...` clean

---

## Task 1: Create `windowlifecycle/go.mod` and `lifecycle.go`

**Description:** Create the module definition and the platform-agnostic public API file containing the `Event` type, constants, `Config` struct, and public function signatures that dispatch to unexported functions.

**Acceptance criteria:**
- [ ] `windowlifecycle/go.mod` exists with `module arcmantle/windowlifecycle` and `go 1.23`
- [ ] `windowlifecycle/lifecycle.go` has package `windowlifecycle`, exports `Event`, `WindowHidden`, `ReopenRequest`, `QuitRequest`, `Config`, `Configure()`, `InstallWindowDelegate()`, `ShowWindow()`, and `Event.String()`
- [ ] Public functions delegate to unexported `configure()`, `installWindowDelegate()`, `showWindow()`

**Verification:**
- [ ] File compiles in isolation (syntax-only): `cd windowlifecycle && gofmt -e lifecycle.go`

**Dependencies:** None

**Files touched:**
- `windowlifecycle/go.mod` (new)
- `windowlifecycle/lifecycle.go` (new)

**Estimated scope:** Small

---

## Task 2: Create `lifecycle_darwin.go` and `lifecycle_darwin.m`

**Description:** Port the darwin+cgo implementation from `internal/appkit/appkit_darwin.go` and `appkit_darwin.m` into the new module. The Go file implements the unexported dispatch functions (`configure`, `installWindowDelegate`, `showWindow`) and the `//export` callbacks. The `.m` file is copied as-is (no changes needed — the C function names and Go callback names are unchanged).

**Acceptance criteria:**
- [ ] `windowlifecycle/lifecycle_darwin.go` has `//go:build darwin && cgo`, implements `configure()`, `installWindowDelegate()`, `showWindow()`, and the three `//export goAppkit*` callbacks
- [ ] `windowlifecycle/lifecycle_darwin.m` is identical to the current `appkit_darwin.m`
- [ ] CGO directives: `-x objective-c`, `-framework Cocoa`

**Verification:**
- [ ] `cd windowlifecycle && go build .` passes on macOS with CGO

**Dependencies:** Task 1

**Files touched:**
- `windowlifecycle/lifecycle_darwin.go` (new)
- `windowlifecycle/lifecycle_darwin.m` (new)

**Estimated scope:** Small

---

## Task 3: Create `lifecycle_nocgo.go` and `lifecycle_other.go`

**Description:** Create the `!cgo` stub (new — didn't exist in `internal/appkit`) and the `!darwin` stub (ported from `appkit_other.go`). Both implement the unexported dispatch functions as no-ops.

**Acceptance criteria:**
- [ ] `windowlifecycle/lifecycle_nocgo.go` has `//go:build !cgo`, implements `configure()` returning a closed channel, `installWindowDelegate()` and `showWindow()` as no-ops
- [ ] `windowlifecycle/lifecycle_other.go` has `//go:build !darwin`, implements the same three functions as no-ops (closed channel for configure)
- [ ] Build tags don't overlap with `lifecycle_darwin.go`

**Verification:**
- [ ] `cd windowlifecycle && CGO_ENABLED=0 go build .` passes
- [ ] `cd windowlifecycle && GOOS=linux go build .` passes

**Dependencies:** Task 1

**Files touched:**
- `windowlifecycle/lifecycle_nocgo.go` (new)
- `windowlifecycle/lifecycle_other.go` (new)

**Estimated scope:** XS

---

## Task 4: Add `replace` directive and `require` in root `go.mod`

**Description:** Add `arcmantle/windowlifecycle v0.0.0` to the root module's `require` block and a `replace arcmantle/windowlifecycle => ./windowlifecycle` directive, matching the existing pattern for `windowfocus` and `windowicon`.

**Acceptance criteria:**
- [ ] `go.mod` has `arcmantle/windowlifecycle v0.0.0` in require
- [ ] `go.mod` has `replace arcmantle/windowlifecycle => ./windowlifecycle`

**Verification:**
- [ ] `go mod tidy` runs cleanly

**Dependencies:** Tasks 1-3

**Files touched:**
- `go.mod`

**Estimated scope:** XS

---

## Task 5: Update `cmd/vortex-window` imports

**Description:** Change `cmd/vortex-window/lifecycle_darwin.go` to import `arcmantle/windowlifecycle` instead of `arcmantle/vortex/internal/appkit`. Update all references from `appkit.X` to `windowlifecycle.X`.

**Acceptance criteria:**
- [ ] `lifecycle_darwin.go` imports `arcmantle/windowlifecycle` (not `internal/appkit`)
- [ ] All references updated: `appkit.Configure` → `windowlifecycle.Configure`, `appkit.Config` → `windowlifecycle.Config`, `appkit.InstallWindowDelegate` → `windowlifecycle.InstallWindowDelegate`, `appkit.ShowWindow` → `windowlifecycle.ShowWindow`, `appkit.Event` → `windowlifecycle.Event`, `appkit.WindowHidden` → `windowlifecycle.WindowHidden`, `appkit.ReopenRequest` → `windowlifecycle.ReopenRequest`, `appkit.QuitRequest` → `windowlifecycle.QuitRequest`

**Verification:**
- [ ] `go build ./cmd/vortex-window/` passes on darwin
- [ ] `go vet ./cmd/vortex-window/` clean

**Dependencies:** Task 4

**Files touched:**
- `cmd/vortex-window/lifecycle_darwin.go`

**Estimated scope:** XS

---

## Task 6: Delete `internal/appkit/`

**Description:** Remove the old `internal/appkit/` directory entirely. Verify no remaining references exist.

**Acceptance criteria:**
- [ ] `internal/appkit/` directory no longer exists
- [ ] No Go files import `arcmantle/vortex/internal/appkit`

**Verification:**
- [ ] `grep -r "internal/appkit" --include="*.go" .` returns no results
- [ ] `go build ./...` passes
- [ ] `go vet ./...` clean

**Dependencies:** Task 5

**Files touched:**
- `internal/appkit/appkit.go` (deleted)
- `internal/appkit/appkit_darwin.go` (deleted)
- `internal/appkit/appkit_darwin.m` (deleted)
- `internal/appkit/appkit_other.go` (deleted)

**Estimated scope:** XS

---

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| `//export` callback names collide across packages | High — linker error | Names are unique (`goAppkit*`); no collision with `windowfocus`/`windowicon` exports |
| `!cgo` and `!darwin` build tags overlap | Medium — duplicate symbols | `_nocgo.go` uses `!cgo`, `_other.go` uses `!darwin`; on darwin+cgo only `_darwin.go` compiles. On darwin+!cgo, `_nocgo.go` compiles. On !darwin, `_other.go` compiles. No overlap. |
| Forgetting to update docs/ideas references | Low | Non-code references don't break builds; can update opportunistically |

## Open Questions
- None — all decisions are settled from the ideation phase.
