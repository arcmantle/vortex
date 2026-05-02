import type { VortexTheme } from './theme.js';

export const dark: VortexTheme = {
  name: 'Dark',
  author: 'Vortex',
  type: 'dark',
  colors: {
    // Surfaces
    'surface-0': '#1e1e1e',
    'surface-1': '#252526',
    'surface-2': '#2d2d2d',
    'surface-3': '#333333',
    'surface-4': '#2a2a2a',
    'surface-5': '#444444',

    // Text
    'text-primary': '#d4d4d4',
    'text-secondary': '#cccccc',
    'text-muted': '#888888',
    'text-inverse': '#1e1e1e',

    // Borders
    'border-subtle': '#2d2d2d',
    'border-default': '#3c3c3c',
    'border-strong': '#555555',

    // Interactive
    'accent': '#0078d4',
    'accent-hover': '#1a8ae8',
    'accent-active': '#005a9e',
    'accent-muted': '#094771',

    // Status
    'success': '#3fb950',
    'error': '#f14c4c',
    'warning': '#cca700',
    'info': '#58a6ff',

    // Scrollbar
    'scrollbar-thumb': '#42424280',
    'scrollbar-thumb-hover': '#555555b3',
    'scrollbar-thumb-active': '#666666cc',

    // Terminal ANSI
    'ansi-black': '#1e1e1e',
    'ansi-red': '#f14c4c',
    'ansi-green': '#23d18b',
    'ansi-yellow': '#f5f543',
    'ansi-blue': '#3b8eea',
    'ansi-magenta': '#d670d6',
    'ansi-cyan': '#29b8db',
    'ansi-white': '#cccccc',
    'ansi-bright-black': '#666666',
    'ansi-bright-red': '#f14c4c',
    'ansi-bright-green': '#23d18b',
    'ansi-bright-yellow': '#f5f543',
    'ansi-bright-blue': '#3b8eea',
    'ansi-bright-magenta': '#d670d6',
    'ansi-bright-cyan': '#29b8db',
    'ansi-bright-white': '#ffffff',
  },
};
