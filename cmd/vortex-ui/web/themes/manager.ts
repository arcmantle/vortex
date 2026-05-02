import type { VortexTheme } from './theme.js';
import { applyTheme } from './index.js';
import { dark } from './dark.js';
import { light } from './light.js';
import { monokai } from './monokai.js';
import { solarizedDark } from './solarized-dark.js';
import { solarizedLight } from './solarized-light.js';
import { nord } from './nord.js';
import { dracula } from './dracula.js';
import { catppuccinMocha } from './catppuccin-mocha.js';

export type ThemeName = string;
export type ThemeMode = ThemeName | 'system';

const BUILTIN_THEMES: Map<string, VortexTheme> = new Map([
  ['dark', dark],
  ['light', light],
  ['monokai', monokai],
  ['solarized-dark', solarizedDark],
  ['solarized-light', solarizedLight],
  ['nord', nord],
  ['dracula', dracula],
  ['catppuccin-mocha', catppuccinMocha],
]);

/**
 * Manages theme selection, application, and system preference tracking.
 * Dispatches 'vx-theme-changed' on the target element whenever the active theme changes.
 */
export class ThemeManager {
  private _target: HTMLElement;
  private _mode: ThemeMode = 'dark';
  private _mql: MediaQueryList;
  private _mqlHandler: () => void;
  private _activeTheme: VortexTheme = dark;

  constructor(target: HTMLElement) {
    this._target = target;
    this._mql = window.matchMedia('(prefers-color-scheme: dark)');
    this._mqlHandler = () => {
      if (this._mode === 'system') this._applySystemTheme();
    };
    this._mql.addEventListener('change', this._mqlHandler);
  }

  /** Register an additional theme (e.g. from custom JSON). */
  registerTheme(name: string, theme: VortexTheme): void {
    BUILTIN_THEMES.set(name, theme);
  }

  /** Get names of all registered themes. */
  getAvailableThemes(): { name: string; theme: VortexTheme }[] {
    return [...BUILTIN_THEMES.entries()].map(([name, theme]) => ({ name, theme }));
  }

  /** Get the currently active theme. */
  get activeTheme(): VortexTheme {
    return this._activeTheme;
  }

  /** Get the current mode (theme name or 'system'). */
  get mode(): ThemeMode {
    return this._mode;
  }

  /** Set theme by name or 'system' to follow OS preference. */
  setTheme(mode: ThemeMode): void {
    this._mode = mode;
    if (mode === 'system') {
      this._applySystemTheme();
    } else {
      const theme = BUILTIN_THEMES.get(mode);
      if (theme) this._apply(theme);
    }
  }

  /** Load custom themes from the server API and register them. */
  async loadCustomThemes(apiBase: string, headers: HeadersInit = {}): Promise<void> {
    try {
      const res = await fetch(`${apiBase}/api/themes`, { headers });
      if (!res.ok) return;
      const data = await res.json() as { id: string; data: VortexTheme }[];
      for (const entry of data) {
        if (entry.id && entry.data?.name && entry.data?.colors) {
          this.registerTheme(entry.id, entry.data);
        }
      }
    } catch { /* ignore */ }
  }

  /** Clean up event listeners. */
  destroy(): void {
    this._mql.removeEventListener('change', this._mqlHandler);
  }

  private _applySystemTheme(): void {
    const theme = this._mql.matches ? dark : light;
    this._apply(theme);
  }

  private _apply(theme: VortexTheme): void {
    this._activeTheme = theme;
    applyTheme(theme, this._target);
    // Also set on document element for global styles (scrollbars in index.html)
    applyTheme(theme, document.documentElement);
    this._target.dispatchEvent(new CustomEvent('vx-theme-changed', {
      detail: { theme },
      bubbles: true,
      composed: true,
    }));
  }
}
