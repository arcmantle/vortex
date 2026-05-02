import type { VortexTheme } from './theme.js';

/** Apply a theme's colors as CSS custom properties on the target element. */
export function applyTheme(theme: VortexTheme, target: HTMLElement): void {
  for (const [key, value] of Object.entries(theme.colors)) {
    target.style.setProperty(`--vx-${key}`, value);
  }
  // Background image properties
  if (theme.backgroundImage) {
    target.style.setProperty('--vx-bg-image', `url("${theme.backgroundImage}")`);
  } else {
    target.style.setProperty('--vx-bg-image', 'none');
  }
  target.style.setProperty('--vx-bg-opacity', String(theme.backgroundOpacity ?? 0.15));
  target.style.setProperty('--vx-bg-blur', `${theme.backgroundBlur ?? 12}px`);
}

export type { VortexTheme } from './theme.js';
export { dark } from './dark.js';
export { light } from './light.js';
export { ThemeManager } from './manager.js';
export type { ThemeMode, ThemeName } from './manager.js';
export { extractThemeFromImage } from './extract.js';
