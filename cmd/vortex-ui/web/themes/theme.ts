/** Semantic token names for the Vortex theme system. */
export interface VortexTheme {
  name: string;
  author: string;
  type: 'dark' | 'light';

  colors: {
    // Surfaces
    'surface-0': string;
    'surface-1': string;
    'surface-2': string;
    'surface-3': string;
    'surface-4': string;
    'surface-5': string;

    // Text
    'text-primary': string;
    'text-secondary': string;
    'text-muted': string;
    'text-inverse': string;

    // Borders
    'border-subtle': string;
    'border-default': string;
    'border-strong': string;

    // Interactive
    'accent': string;
    'accent-hover': string;
    'accent-active': string;
    'accent-muted': string;

    // Status
    'success': string;
    'error': string;
    'warning': string;
    'info': string;

    // Scrollbar
    'scrollbar-thumb': string;
    'scrollbar-thumb-hover': string;
    'scrollbar-thumb-active': string;

    // Terminal ANSI
    'ansi-black': string;
    'ansi-red': string;
    'ansi-green': string;
    'ansi-yellow': string;
    'ansi-blue': string;
    'ansi-magenta': string;
    'ansi-cyan': string;
    'ansi-white': string;
    'ansi-bright-black': string;
    'ansi-bright-red': string;
    'ansi-bright-green': string;
    'ansi-bright-yellow': string;
    'ansi-bright-blue': string;
    'ansi-bright-magenta': string;
    'ansi-bright-cyan': string;
    'ansi-bright-white': string;
  };

  // Background image (optional)
  backgroundImage?: string | null;
  backgroundOpacity?: number;
  backgroundBlur?: number;
}
