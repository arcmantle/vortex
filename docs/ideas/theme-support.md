# Theme Support for Vortex GUI

## Problem Statement
How might we let power users personalize the entire Vortex GUI — app chrome and terminal — through a system that ships great defaults but allows full customization?

## Recommended Direction

**Semantic CSS token system** with built-in presets, JSON override, and background-image-derived palettes.

The foundation is ~30 CSS custom properties on the root `:host` element using the `--vx-` prefix, organized into semantic groups (surfaces, text, borders, interactive, status, terminal ANSI). Every hardcoded hex value in every component gets replaced with a `var(--vx-*)` reference.

Themes are JSON files mapping token names to color values, with metadata (name, author, dark/light classification). Vortex ships 6-8 polished built-in themes. Users can create custom themes via JSON (stored in `~/.config/vortex/themes/`). The Settings panel gets a "Theme" section in the General tab showing live-preview swatches.

A **system** option follows OS dark/light preference via `prefers-color-scheme`, automatically switching between a configured light and dark theme.

For xterm.js (which uses canvas, not CSS), the theme JSON is also read into a JS object and passed as `Terminal.options.theme` reactively. Same source of truth, two delivery mechanisms.

### Background Image + Color Derivation

Users can set a background image for the app. When an image is selected, Vortex extracts dominant colors via canvas-based color quantization (median-cut algorithm) and generates a full theme palette from those colors — mapping extracted hues to semantic tokens using perceptual lightness sorting. The derived theme can be used as-is or saved/tweaked as a custom theme JSON.

This is a "delight" feature — purely optional but visually striking. The background image is rendered behind the UI with appropriate opacity/blur so text remains readable.

## Token Set

### Prefix: `--vx-`

### Surfaces (6)
- `--vx-surface-0` — deepest background (terminal, main area)
- `--vx-surface-1` — primary panels
- `--vx-surface-2` — elevated panels, cards
- `--vx-surface-3` — tooltips, dropdowns, overlays
- `--vx-surface-4` — hover states on surfaces
- `--vx-surface-5` — active/pressed states

### Text (4)
- `--vx-text-primary` — main content text
- `--vx-text-secondary` — labels, descriptions
- `--vx-text-muted` — hints, placeholders
- `--vx-text-inverse` — text on accent backgrounds

### Borders (3)
- `--vx-border-subtle` — soft dividers
- `--vx-border-default` — standard borders
- `--vx-border-strong` — emphasized borders, focus rings

### Interactive (4)
- `--vx-accent` — primary accent (buttons, links, active indicators)
- `--vx-accent-hover` — accent hover state
- `--vx-accent-active` — accent pressed state
- `--vx-accent-muted` — subtle accent (badges, highlights)

### Status (4)
- `--vx-success` — green (running, passed)
- `--vx-error` — red (failed, stopped)
- `--vx-warning` — yellow/orange (pending, slow)
- `--vx-info` — blue (informational)

### Scrollbar (3)
- `--vx-scrollbar-thumb` — default thumb
- `--vx-scrollbar-thumb-hover` — hover
- `--vx-scrollbar-thumb-active` — dragging

### Terminal ANSI (16)
- `--vx-ansi-black`, `--vx-ansi-red`, `--vx-ansi-green`, `--vx-ansi-yellow`
- `--vx-ansi-blue`, `--vx-ansi-magenta`, `--vx-ansi-cyan`, `--vx-ansi-white`
- `--vx-ansi-bright-black`, `--vx-ansi-bright-red`, `--vx-ansi-bright-green`, `--vx-ansi-bright-yellow`
- `--vx-ansi-bright-blue`, `--vx-ansi-bright-magenta`, `--vx-ansi-bright-cyan`, `--vx-ansi-bright-white`

### Background Image (3)
- `--vx-bg-image` — url() or none
- `--vx-bg-opacity` — opacity of the image layer (0-1)
- `--vx-bg-blur` — blur radius for readability

**Total: ~43 tokens**

## Theme File Format

```json
{
  "name": "Nord",
  "author": "Arctic Ice Studio",
  "type": "dark",
  "colors": {
    "surface-0": "#2e3440",
    "surface-1": "#3b4252",
    "surface-2": "#434c5e",
    "surface-3": "#4c566a",
    "surface-4": "#4c566a",
    "surface-5": "#5e6779",
    "text-primary": "#eceff4",
    "text-secondary": "#d8dee9",
    "text-muted": "#7b88a1",
    "text-inverse": "#2e3440",
    "border-subtle": "#3b4252",
    "border-default": "#4c566a",
    "border-strong": "#5e6779",
    "accent": "#88c0d0",
    "accent-hover": "#8fbcbb",
    "accent-active": "#81a1c1",
    "accent-muted": "#88c0d033",
    "success": "#a3be8c",
    "error": "#bf616a",
    "warning": "#ebcb8b",
    "info": "#5e81ac",
    "scrollbar-thumb": "#4c566a80",
    "scrollbar-thumb-hover": "#5e6779b3",
    "scrollbar-thumb-active": "#6e7a8ecc",
    "ansi-black": "#3b4252",
    "ansi-red": "#bf616a",
    "ansi-green": "#a3be8c",
    "ansi-yellow": "#ebcb8b",
    "ansi-blue": "#81a1c1",
    "ansi-magenta": "#b48ead",
    "ansi-cyan": "#88c0d0",
    "ansi-white": "#e5e9f0",
    "ansi-bright-black": "#4c566a",
    "ansi-bright-red": "#bf616a",
    "ansi-bright-green": "#a3be8c",
    "ansi-bright-yellow": "#ebcb8b",
    "ansi-bright-blue": "#81a1c1",
    "ansi-bright-magenta": "#b48ead",
    "ansi-bright-cyan": "#8fbcbb",
    "ansi-bright-white": "#eceff4"
  },
  "backgroundImage": null,
  "backgroundOpacity": 0.15,
  "backgroundBlur": 12
}
```

## Key Assumptions to Validate
- [ ] ~43 tokens are sufficient — test by implementing Dark + Light first, verify no "dead spots"
- [ ] xterm.js re-theme is instant — verify no visible flash when switching with content in buffer
- [ ] JSON schema is authorable — test by hand-writing a custom theme in < 5 minutes
- [ ] Background image color extraction produces usable palettes — test with 10 varied images
- [ ] Background image + blur doesn't impact terminal readability at default opacity

## MVP Scope

**In:**
- Define semantic token set (43 vars)
- Migrate all components from hardcoded hex → `var(--vx-*)`
- 2 built-in themes: **Dark** (current look) and **Light**
- System option: follow `prefers-color-scheme` to auto-switch
- Theme selection persisted in settings (`theme` field in config.json)
- Settings General tab: theme picker with instant preview
- xterm.js bridge: reactive theme object derived from current theme tokens
- Theme JSON schema for editor autocomplete

**Fast-follow:**
- 6 additional built-in themes (Monokai, Solarized Dark/Light, Nord, Dracula, Catppuccin)
- Custom theme loading from `~/.config/vortex/themes/*.json`
- Background image support with opacity/blur controls
- Color extraction from background image → auto-generated theme
- Live theme editor in Settings (color pickers per token)

## Not Doing (and Why)
- **VS Code theme import** — Mapping their 600+ tokens to our 43 is fragile. Users can manually map.
- **Per-job or per-shell themes** — One global theme. Complexity not justified.
- **Palette generation from single accent color** — Handcrafted presets look better. (Background image derivation is different — it's opt-in and fun.)
- **CSS-in-JS migration** — Lit's `css` + CSS custom properties is the right tool.
- **Theme marketplace** — Ship JSON format first, community shares files organically.
- **Animated backgrounds / video** — Performance and distraction concerns. Static images only.

## Implementation Order

1. **Token definition + migration** — Replace all hardcoded hex with `var(--vx-*)`, define default values
2. **Theme loading infrastructure** — JSON parsing, settings persistence, `prefers-color-scheme` listener
3. **Dark + Light themes** — Polish the two built-in options
4. **Settings UI** — Theme picker in General tab
5. **xterm.js bridge** — Reactive terminal re-theming
6. **Additional built-in themes** — Nord, Monokai, etc.
7. **Custom theme loading** — File watching on `~/.config/vortex/themes/`
8. **Background image** — Image layer + opacity/blur controls
9. **Color extraction** — Canvas-based median-cut → palette → theme generation

## Open Questions (Resolved)
- ✅ Token prefix: `--vx-`
- ✅ Theme metadata: yes (name, author, type)
- ✅ System option: yes, follows `prefers-color-scheme`
- ✅ Background images: yes, with color derivation
