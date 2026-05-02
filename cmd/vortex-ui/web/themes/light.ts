import type { VortexTheme } from './theme.js';

export const light: VortexTheme = {
  name: 'Light',
  author: 'Vortex',
  type: 'light',
  colors: {
    // Surfaces
    'surface-0': '#ffffff',
    'surface-1': '#f3f3f3',
    'surface-2': '#e8e8e8',
    'surface-3': '#d4d4d4',
    'surface-4': '#eaeaea',
    'surface-5': '#d0d0d0',

    // Text
    'text-primary': '#1e1e1e',
    'text-secondary': '#333333',
    'text-muted': '#777777',
    'text-inverse': '#ffffff',

    // Borders
    'border-subtle': '#e8e8e8',
    'border-default': '#d4d4d4',
    'border-strong': '#b0b0b0',

    // Interactive
    'accent': '#0078d4',
    'accent-hover': '#106ebe',
    'accent-active': '#005a9e',
    'accent-muted': '#cce4f7',

    // Status
    'success': '#1a7f37',
    'error': '#cf222e',
    'warning': '#9a6700',
    'info': '#0550ae',

    // Scrollbar
    'scrollbar-thumb': '#c1c1c180',
    'scrollbar-thumb-hover': '#a0a0a0b3',
    'scrollbar-thumb-active': '#888888cc',

    // Terminal ANSI
    'ansi-black': '#1e1e1e',
    'ansi-red': '#cd3131',
    'ansi-green': '#14871d',
    'ansi-yellow': '#b5850e',
    'ansi-blue': '#0451a5',
    'ansi-magenta': '#bc05bc',
    'ansi-cyan': '#0598bc',
    'ansi-white': '#e5e5e5',
    'ansi-bright-black': '#666666',
    'ansi-bright-red': '#cd3131',
    'ansi-bright-green': '#14ce14',
    'ansi-bright-yellow': '#b5ba00',
    'ansi-bright-blue': '#0451a5',
    'ansi-bright-magenta': '#bc05bc',
    'ansi-bright-cyan': '#0598bc',
    'ansi-bright-white': '#a5a5a5',
  },
};
