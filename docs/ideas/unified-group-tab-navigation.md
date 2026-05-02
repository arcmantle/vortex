# Unified Group+Tab Navigation

## Problem Statement

How might we unify Vortex's URL navigation so that every view — jobs, shells, settings, editor — uses the same `group` + `tab` grammar, eliminating the inconsistent `panel` param and the branching logic it requires?

## Recommended Direction

Replace the current dual-system (`group`+`tab` for terminals, `panel` for overlays) with a single model:

- **Every view is a group.** System groups are prefixed with `@` to avoid collisions with user-defined process groups.
- **Every sub-view is a tab.** `tab` is optional — when omitted, the group picks its own default.
- **`panel` is removed entirely.** No special booleans (`_showGeneralSettings`, `_showShellSettings`, `_showConfigPreview`, `_settingsSubtab`). Just `_activeGroup` + `_activeId`.

### URL Schema

| View                  | URL                              |
|-----------------------|----------------------------------|
| Job                   | `?group=build&tab=server`        |
| Shell                 | `?group=shell&tab=abc123`        |
| Settings: Appearance  | `?group=@settings&tab=appearance`|
| Settings: Font        | `?group=@settings&tab=font`      |
| Settings: Shells      | `?group=@settings&tab=shells`    |
| Editor: Config        | `?group=@editor&tab=config`      |

Visiting `?group=@settings` (no tab) defaults to `appearance`. Visiting `?group=@editor` defaults to `config`.

### Navigation behavior

- **Opening a system group (`@settings`, `@editor`) uses `pushState`** — so the browser back button closes the overlay and returns to the previous terminal view.
- **Closing an overlay restores the previous `group`+`tab`** — the app remembers the last terminal state before the overlay was opened.
- **Terminal-to-terminal navigation stays `replaceState`** — no history pollution when switching between job/shell tabs.

### `_syncURL()` becomes trivial

```ts
private _syncURL(push = false): void {
  const params = new URLSearchParams();
  if (this._token) params.set('token', this._token);
  const group = this._activeGroup === SHELL_GROUP ? 'shell' : this._activeGroup;
  if (group) params.set('group', group);
  if (this._activeId) params.set('tab', this._activeId);
  const qs = params.toString();
  const url = qs ? `?${qs}` : window.location.pathname;
  if (push) history.pushState(null, '', url);
  else history.replaceState(null, '', url);
}
```

No branching. One path. The `push` flag is only true when entering a system group.

## Key Assumptions to Validate

- [ ] No user will name a process group starting with `@` — validate by checking the config parser rejects `@` in group names
- [ ] Settings will stay flat (appearance, font, shells) and won't need a 3rd depth level — revisit if settings grows complex
- [ ] The `@` character is URL-safe without encoding — it is (RFC 3986 sub-delim), but confirm no proxy/middleware strips it

## MVP Scope

1. Remove all `panel`-related state (`_showGeneralSettings`, `_showShellSettings`, `_showConfigPreview`, `_settingsSubtab`)
2. Replace with unified `_activeGroup` / `_activeId` routing — when group starts with `@`, render the corresponding overlay component
3. Simplify `_syncURL()` to always write `group` + `tab`
4. Update `connectedCallback()` URL parsing to use the new schema
5. Update the settings component to receive its tab via `_activeId` instead of separate props
6. Add a guard in the config parser: reject group names starting with `@`

## Not Doing (and Why)

- **3-level depth (subtab)** — current settings are flat enough; adding a third level now is premature complexity
- **Hash-based routing** — `replaceState` works fine, no reason to add a router library
- **Backward-compatible `panel` fallback** — URLs are ephemeral (replaceState, no bookmarks), migration cost is zero
- **Type-discriminated param (`?type=overlay`)** — adds a third param for zero practical benefit

## Decided

- **Back button closes overlays** — navigating to `@settings`/`@editor` uses `pushState`. Back button pops back to the previous terminal view.
- **Restore previous group+tab on close** — the app stores `_previousGroup` / `_previousId` before entering a system group, and restores them on close or `popstate`.
