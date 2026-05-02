# Implementation Plan: Vortex Theme Support

## Overview

Migrate all hardcoded colors to CSS custom properties (`--vx-*`), build a theme loading system, ship Dark + Light built-in themes, wire up xterm.js reactive theming, and add a theme picker to Settings. Foundation for future custom themes, background images, and color extraction.

## Architecture Decisions

- **CSS custom properties on `:host` of `<vortex-app>`** — cascade into all child shadow DOMs automatically
- **Theme definition file**: `cmd/vortex-ui/web/themes/` directory with `.ts` exports (built-in) and JSON schema for custom themes
- **xterm.js bridge**: Derive terminal theme object from CSS variable values at apply-time (read computed styles)
- **`prefers-color-scheme` listener**: MediaQueryList listener auto-switches when theme is set to "system"
- **Persistence**: `theme` field in existing settings config (Go backend)

## Dependency Graph

```
Token definition (theme.ts types + default values)
    │
    ├── index.html migration (global scrollbar vars)
    │
    ├── app.ts migration (root host vars + component colors)
    │       │
    │       └── child component migrations (terminal, settings, config-preview)
    │
    ├── Built-in theme files (dark.ts, light.ts)
    │       │
    │       └── Theme loading + application logic
    │               │
    │               ├── xterm.js bridge (reactive terminal re-theme)
    │               │
    │               ├── prefers-color-scheme listener
    │               │
    │               └── Settings UI (theme picker)
    │
    └── Go backend (persist theme choice)
```

---

## Task List

### Phase 1: Token Foundation

#### Task 1: Define theme token types and dark theme defaults

**Description:** Create the theme type definition and a default dark theme that maps to the current hardcoded colors. This is the contract all other work builds on.

**Acceptance criteria:**
- [ ] `cmd/vortex-ui/web/themes/theme.ts` exports `VortexTheme` interface with all 43 token fields
- [ ] `cmd/vortex-ui/web/themes/dark.ts` exports a `VortexTheme` object matching current hardcoded colors exactly
- [ ] `cmd/vortex-ui/web/themes/index.ts` exports `applyTheme(theme: VortexTheme)` that sets CSS vars on a target element
- [ ] TypeScript compiles clean

**Verification:**
- [ ] `npx tsc --noEmit` passes
- [ ] Importing `dark` theme and calling `applyTheme` sets all `--vx-*` vars on a DOM element

**Dependencies:** None

**Files likely touched:**
- `cmd/vortex-ui/web/themes/theme.ts` (new)
- `cmd/vortex-ui/web/themes/dark.ts` (new)
- `cmd/vortex-ui/web/themes/index.ts` (new)

**Estimated scope:** Small (3 new files, no existing code changed)

---

#### Task 2: Create light theme

**Description:** Define a light theme with appropriate color inversions for all 43 tokens. Doesn't need to be applied yet — just defined.

**Acceptance criteria:**
- [ ] `cmd/vortex-ui/web/themes/light.ts` exports a `VortexTheme` object
- [ ] All surface colors are light (white/light gray range)
- [ ] Text colors are dark
- [ ] ANSI colors are adjusted for light background readability
- [ ] Status colors maintain WCAG AA contrast on light surfaces

**Verification:**
- [ ] TypeScript compiles clean
- [ ] All 43 fields present (type-checked)

**Dependencies:** Task 1

**Files likely touched:**
- `cmd/vortex-ui/web/themes/light.ts` (new)

**Estimated scope:** XS (1 new file)

---

### Phase 2: Component Migration

#### Task 3: Migrate `index.html` and `app.ts` to CSS variables

**Description:** Replace all hardcoded hex values in `index.html` global styles and `app.ts` static styles with `var(--vx-*)` references. Add fallback values matching dark theme. Apply the dark theme on the `:host` of `<vortex-app>` at startup.

**Acceptance criteria:**
- [ ] `index.html` uses `var(--vx-surface-0)` for body background, `var(--vx-scrollbar-thumb)` for scrollbar
- [ ] `app.ts` static styles have zero hardcoded hex colors (all replaced with `var(--vx-*)`)
- [ ] `app.ts` `connectedCallback` calls `applyTheme(dark)` on `this` (the host element)
- [ ] UI looks identical to before (dark theme values match previous hardcoded colors)

**Verification:**
- [ ] Visual comparison: app looks the same as before in the browser
- [ ] `grep -c '#[0-9a-fA-F]' app.ts` returns 0 (no hex in styles)
- [ ] Build succeeds

**Dependencies:** Task 1

**Files likely touched:**
- `cmd/vortex-ui/web/index.html`
- `cmd/vortex-ui/web/app.ts`

**Estimated scope:** Medium (2 files, ~28 color replacements in app.ts alone)

---

#### Task 4: Migrate `vortex-terminal.ts` to CSS variables

**Description:** Replace hardcoded colors in terminal component styles AND the xterm.js theme object with CSS variable references. The xterm.js `theme` option requires JS values, so read computed styles from the host element.

**Acceptance criteria:**
- [ ] Static styles use `var(--vx-*)` for host background and scrollbar
- [ ] xterm.js `Terminal` options `theme` object reads from CSS custom properties via `getComputedStyle`
- [ ] When theme variables change, terminal re-applies theme (reactive update)
- [ ] Terminal looks identical to before with dark theme applied

**Verification:**
- [ ] Terminal background matches `--vx-surface-0`
- [ ] Terminal text matches `--vx-text-primary`
- [ ] Scrollbar thumb matches `--vx-scrollbar-thumb`
- [ ] Build succeeds

**Dependencies:** Task 3 (needs vars applied on host)

**Files likely touched:**
- `cmd/vortex-ui/web/components/vortex-terminal.ts`

**Estimated scope:** Small (1 file, 4 CSS colors + xterm theme bridge)

---

#### Task 5: Migrate `vortex-settings.ts` to CSS variables

**Description:** Replace all 22 hardcoded hex values in the settings component with `var(--vx-*)` references.

**Acceptance criteria:**
- [ ] Zero hardcoded hex colors in static styles
- [ ] All interactive states (hover, active, focus) use appropriate semantic tokens
- [ ] Font picker dropdown, overlays, and form fields all themed
- [ ] Settings panel looks identical with dark theme

**Verification:**
- [ ] Visual comparison: settings panel unchanged
- [ ] `grep '#[0-9a-fA-F]' vortex-settings.ts` finds no matches in css block
- [ ] Build succeeds

**Dependencies:** Task 3

**Files likely touched:**
- `cmd/vortex-ui/web/components/vortex-settings.ts`

**Estimated scope:** Medium (1 file, 22 replacements)

---

#### Task 6: Migrate `vortex-config-preview.ts` to CSS variables

**Description:** Replace all 8 hardcoded hex values in the config preview component.

**Acceptance criteria:**
- [ ] Zero hardcoded hex colors in static styles
- [ ] Config preview panel looks identical with dark theme

**Verification:**
- [ ] Visual comparison: config preview unchanged
- [ ] Build succeeds

**Dependencies:** Task 3

**Files likely touched:**
- `cmd/vortex-ui/web/components/vortex-config-preview.ts`

**Estimated scope:** XS (1 file, 8 replacements)

---

#### Task 7: Migrate legacy settings components (general + shell)

**Description:** Replace hardcoded colors in `vortex-general-settings.ts` and `vortex-shell-settings.ts`. These may be superseded but are still in the codebase.

**Acceptance criteria:**
- [ ] Zero hardcoded hex colors in both files' static styles
- [ ] Build succeeds

**Verification:**
- [ ] Build succeeds
- [ ] No visual regressions if components are rendered

**Dependencies:** Task 3

**Files likely touched:**
- `cmd/vortex-ui/web/components/vortex-general-settings.ts`
- `cmd/vortex-ui/web/components/vortex-shell-settings.ts`

**Estimated scope:** Small (2 files, ~25 replacements total)

---

### Checkpoint: Migration Complete
- [ ] All components use CSS variables exclusively
- [ ] App looks identical to before (dark theme)
- [ ] Zero hardcoded hex values in any component's CSS
- [ ] Build succeeds, no TypeScript errors
- [ ] Manual verification in browser

---

### Phase 3: Theme Infrastructure

#### Task 8: Theme loading and application logic

**Description:** Build the theme manager that loads a theme by name, applies it to the DOM, handles "system" mode (prefers-color-scheme), and dispatches change events.

**Acceptance criteria:**
- [ ] `cmd/vortex-ui/web/themes/manager.ts` exports `ThemeManager` class
- [ ] `ThemeManager.setTheme(name: 'dark' | 'light' | 'system')` applies the correct theme
- [ ] "system" mode: listens to `prefers-color-scheme` media query and auto-switches
- [ ] Dispatches a custom event when theme changes (for xterm.js bridge to react)
- [ ] Built-in themes are registered by name

**Verification:**
- [ ] Calling `setTheme('light')` visually switches the entire UI to light colors
- [ ] Calling `setTheme('system')` respects OS preference
- [ ] TypeScript compiles clean

**Dependencies:** Tasks 1, 2, 3

**Files likely touched:**
- `cmd/vortex-ui/web/themes/manager.ts` (new)
- `cmd/vortex-ui/web/app.ts` (integrate manager)

**Estimated scope:** Medium (1 new file + 1 modified)

---

#### Task 9: xterm.js reactive theme bridge

**Description:** When the theme changes, update all active terminal instances' xterm.js theme objects by reading the new CSS variable values from computed styles.

**Acceptance criteria:**
- [ ] Terminal component listens for theme-change event
- [ ] On theme change, reads all `--vx-ansi-*`, `--vx-surface-0`, `--vx-text-primary` from computed styles
- [ ] Applies new theme to `this._term.options.theme`
- [ ] No visible flash — theme applies cleanly
- [ ] Existing terminal buffer content re-renders in new colors

**Verification:**
- [ ] Switch from dark → light: terminal background goes white, text goes dark
- [ ] ANSI colored output retains correct semantic colors
- [ ] No performance issues with multiple terminals open

**Dependencies:** Tasks 4, 8

**Files likely touched:**
- `cmd/vortex-ui/web/components/vortex-terminal.ts`

**Estimated scope:** Small (1 file)

---

#### Task 10: Go backend — persist theme setting

**Description:** Add `theme` field to the settings struct and API. The field stores the theme name string (e.g., "dark", "light", "system").

**Acceptance criteria:**
- [ ] `Settings` struct has `Theme string` field
- [ ] `GET /api/settings` returns `theme` in response
- [ ] `PUT /api/settings` accepts and persists `theme`
- [ ] Default value is "system" when not set
- [ ] Go builds clean

**Verification:**
- [ ] `curl http://localhost:7370/api/settings` returns theme field
- [ ] `curl -X PUT ... '{"theme":"light"}'` persists to config file
- [ ] `go build ./...` succeeds

**Dependencies:** None (parallel with UI work)

**Files likely touched:**
- `internal/settings/settings.go`
- `internal/server/server.go`

**Estimated scope:** XS (2 files, minor additions)

---

### Checkpoint: Theme Switching Works
- [ ] Can switch between dark/light/system via API
- [ ] Entire UI including terminals updates instantly
- [ ] Theme persists across page reload
- [ ] System mode tracks OS preference changes

---

### Phase 4: Settings UI

#### Task 11: Theme picker in Settings General tab

**Description:** Add a theme selector to the General tab of the settings panel. Shows available themes with small preview swatches. Selection applies immediately and persists.

**Acceptance criteria:**
- [ ] General tab shows "Theme" section with radio/card selection for Dark, Light, System
- [ ] Each option shows a mini color swatch (surface + accent colors)
- [ ] Selecting a theme applies instantly (no save button needed)
- [ ] "System" option shows which theme is currently active based on OS preference
- [ ] Selection persists to backend on change

**Verification:**
- [ ] Click Light → UI goes light immediately
- [ ] Click System → follows OS preference
- [ ] Reload page → selection persisted
- [ ] Visual: picker looks polished and aligned with existing settings UI

**Dependencies:** Tasks 8, 10

**Files likely touched:**
- `cmd/vortex-ui/web/components/vortex-settings.ts`
- `cmd/vortex-ui/web/themes/manager.ts` (get available themes list)

**Estimated scope:** Medium (2 files)

---

### Checkpoint: MVP Complete
- [ ] Dark + Light + System themes work end-to-end
- [ ] Theme picker in settings is polished
- [ ] All terminals re-theme reactively
- [ ] Setting persists across sessions
- [ ] No hardcoded colors remain
- [ ] Build succeeds, no errors
- [ ] Ready for review before proceeding to Phase 5

---

### Phase 5: Additional Built-in Themes

#### Task 12: Add 6 built-in themes (Monokai, Solarized Dark, Solarized Light, Nord, Dracula, Catppuccin Mocha)

**Description:** Create 6 additional theme files with carefully designed palettes covering popular developer preferences. Register them in the theme manager.

**Acceptance criteria:**
- [ ] Each theme has all 43 tokens defined
- [ ] Each theme has correct `name`, `author`, `type` metadata
- [ ] ANSI colors in each theme match the canonical palette for that theme
- [ ] All 6 appear in the Settings theme picker
- [ ] Each theme is visually distinct and polished

**Verification:**
- [ ] Switch to each theme: no broken/invisible elements
- [ ] Terminal ANSI colors look correct (test with `colortest` or similar)
- [ ] Build succeeds

**Dependencies:** Task 11

**Files likely touched:**
- `cmd/vortex-ui/web/themes/monokai.ts` (new)
- `cmd/vortex-ui/web/themes/solarized-dark.ts` (new)
- `cmd/vortex-ui/web/themes/solarized-light.ts` (new)
- `cmd/vortex-ui/web/themes/nord.ts` (new)
- `cmd/vortex-ui/web/themes/dracula.ts` (new)
- `cmd/vortex-ui/web/themes/catppuccin.ts` (new)
- `cmd/vortex-ui/web/themes/manager.ts` (register themes)

**Estimated scope:** Medium (7 new files + 1 modified, but each file is formulaic)

---

### Phase 6: Custom Themes

#### Task 13: Custom theme loading from JSON files

**Description:** Load user-defined themes from `~/.config/vortex/themes/*.json`. The Go backend reads the directory and serves themes via API. Frontend fetches and registers them alongside built-ins.

**Acceptance criteria:**
- [ ] Go: `GET /api/themes` returns list of available themes (built-in names + custom file names)
- [ ] Go: `GET /api/themes/:name` returns theme JSON for custom themes
- [ ] Frontend: Fetches theme list on load, registers custom themes in manager
- [ ] Custom themes appear in Settings picker alongside built-ins
- [ ] Invalid JSON files are skipped gracefully (log warning, don't crash)

**Verification:**
- [ ] Place a valid `.json` theme file in `~/.config/vortex/themes/`
- [ ] Theme appears in picker and applies correctly
- [ ] Place an invalid file — app doesn't break
- [ ] `go build ./...` succeeds

**Dependencies:** Task 12

**Files likely touched:**
- `internal/server/server.go` (new routes)
- `internal/settings/settings.go` (theme directory path)
- `cmd/vortex-ui/web/themes/manager.ts` (fetch + register)
- `cmd/vortex-ui/web/components/vortex-settings.ts` (dynamic theme list)

**Estimated scope:** Medium (4 files)

---

#### Task 14: Publish theme JSON schema

**Description:** Create a JSON schema for the theme file format so users get autocomplete in their editors.

**Acceptance criteria:**
- [ ] `schemas/vortex-theme.schema.json` defines all 43 color fields + metadata
- [ ] Schema validates the built-in theme JSON exports correctly
- [ ] Custom theme files can reference the schema via `$schema` field

**Verification:**
- [ ] Validate a theme file against schema using `ajv` or similar
- [ ] VS Code provides autocomplete when editing a theme file with `$schema` reference

**Dependencies:** Task 1 (token definition)

**Files likely touched:**
- `schemas/vortex-theme.schema.json` (new)

**Estimated scope:** XS (1 file)

---

### Phase 7: Background Image + Color Extraction

#### Task 15: Background image layer with opacity/blur

**Description:** Add a background image layer to `<vortex-app>` that renders behind all content. Controlled by theme tokens `--vx-bg-image`, `--vx-bg-opacity`, `--vx-bg-blur`. Settings UI gets image picker (file URL input) and opacity/blur sliders.

**Acceptance criteria:**
- [ ] A `::before` pseudo-element (or dedicated div) renders the background image
- [ ] Image respects `object-fit: cover` and fills viewport
- [ ] Opacity and blur are adjustable (default: 0.15 opacity, 12px blur)
- [ ] UI remains fully readable with image active
- [ ] Background image URL persists in settings

**Verification:**
- [ ] Set a background image URL → image appears behind UI with blur
- [ ] Adjust opacity slider → image gets more/less visible
- [ ] Remove image → clean background returns
- [ ] Terminal text remains sharp and readable

**Dependencies:** Task 11

**Files likely touched:**
- `cmd/vortex-ui/web/app.ts` (background layer CSS + element)
- `cmd/vortex-ui/web/components/vortex-settings.ts` (image URL + sliders in General tab)
- `internal/settings/settings.go` (persist image settings)
- `internal/server/server.go` (API fields)

**Estimated scope:** Medium (4 files)

---

#### Task 16: Color extraction from background image

**Description:** When a background image is set, extract dominant colors via canvas + median-cut algorithm. Generate a complete theme from the extracted palette. User can apply the derived theme or tweak and save it.

**Acceptance criteria:**
- [ ] `cmd/vortex-ui/web/themes/extract.ts` exports `extractPalette(imageUrl: string): Promise<string[]>`
- [ ] Extracts 6-8 dominant colors from the image via offscreen canvas
- [ ] `generateThemeFromPalette(colors: string[]): VortexTheme` maps colors to tokens using lightness sorting
- [ ] Settings UI shows "Generate theme from image" button when image is set
- [ ] Generated theme is previewed live and can be saved as custom JSON

**Verification:**
- [ ] Load a vibrant image → extracted colors are visually representative
- [ ] Generated theme has readable contrast (dark text on light surfaces or vice versa)
- [ ] Generated theme applies correctly to all components including terminal
- [ ] Test with 5+ varied images (landscape, abstract, dark photo, light photo, illustration)

**Dependencies:** Task 15

**Files likely touched:**
- `cmd/vortex-ui/web/themes/extract.ts` (new)
- `cmd/vortex-ui/web/components/vortex-settings.ts` (generate button + preview)

**Estimated scope:** Medium (2 files, algorithm complexity)

---

### Checkpoint: Feature Complete
- [ ] All 8 built-in themes work
- [ ] Custom JSON themes load from disk
- [ ] Background image with blur/opacity works
- [ ] Color extraction generates usable themes from images
- [ ] System mode auto-switches
- [ ] Everything persists across sessions
- [ ] All terminals re-theme reactively
- [ ] JSON schema provides editor autocomplete
- [ ] No regressions in existing functionality

---

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| CSS vars don't penetrate xterm.js canvas | High | Use JS bridge: read computed style → set theme object. Already planned. |
| Color extraction produces ugly themes | Medium | Use perceptual lightness (OKLCH) for sorting. Clamp contrast ratios. Allow manual tweaks. |
| Shadow DOM blocks var inheritance | High | CSS custom properties DO inherit through shadow DOM by spec. Verified. |
| Too many tokens → complex JSON authoring | Medium | 43 is tight. Provide schema + examples. Could add "partial theme" support later (inherit missing from dark). |
| Performance: re-theming many terminals | Low | xterm.js re-render is fast for typical buffer sizes. Test with 10+ terminals. |
| Background image sizing on resize | Low | `object-fit: cover` handles this. Test with viewport resize. |

## Open Questions
- None remaining — all resolved in ideation phase.
