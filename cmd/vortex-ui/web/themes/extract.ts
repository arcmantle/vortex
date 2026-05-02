import type { VortexTheme } from './theme.js';

/**
 * Extract dominant colors from an image and generate a VortexTheme.
 * Uses median-cut quantization on a downsampled canvas for performance.
 */
export async function extractThemeFromImage(
  imageUrl: string,
  options: { name?: string; type?: 'dark' | 'light' } = {}
): Promise<VortexTheme> {
  const img = await loadImage(imageUrl);
  const palette = extractPalette(img, 8);
  const type = options.type ?? detectType(palette);
  return buildTheme(palette, { name: options.name ?? 'Custom', type, imageUrl });
}

function loadImage(url: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.crossOrigin = 'anonymous';
    img.onload = () => resolve(img);
    img.onerror = () => reject(new Error('Failed to load image'));
    img.src = url;
  });
}

type RGB = [number, number, number];

function extractPalette(img: HTMLImageElement, count: number): RGB[] {
  const canvas = document.createElement('canvas');
  const size = 64; // downsample for speed
  canvas.width = size;
  canvas.height = size;
  const ctx = canvas.getContext('2d')!;
  ctx.drawImage(img, 0, 0, size, size);
  const { data } = ctx.getImageData(0, 0, size, size);

  const pixels: RGB[] = [];
  for (let i = 0; i < data.length; i += 4) {
    // Skip mostly transparent pixels
    if (data[i + 3] < 128) continue;
    pixels.push([data[i], data[i + 1], data[i + 2]]);
  }

  return medianCut(pixels, count);
}

function medianCut(pixels: RGB[], depth: number): RGB[] {
  if (depth === 0 || pixels.length === 0) {
    return [average(pixels)];
  }

  const ranges = [0, 1, 2].map(ch => {
    const vals = pixels.map(p => p[ch]);
    return Math.max(...vals) - Math.min(...vals);
  });

  const channel = ranges.indexOf(Math.max(...ranges));
  pixels.sort((a, b) => a[channel] - b[channel]);

  const mid = Math.floor(pixels.length / 2);
  return [
    ...medianCut(pixels.slice(0, mid), depth - 1),
    ...medianCut(pixels.slice(mid), depth - 1),
  ];
}

function average(pixels: RGB[]): RGB {
  if (pixels.length === 0) return [128, 128, 128];
  const sum = pixels.reduce(
    (acc, p) => [acc[0] + p[0], acc[1] + p[1], acc[2] + p[2]],
    [0, 0, 0] as [number, number, number]
  );
  return [
    Math.round(sum[0] / pixels.length),
    Math.round(sum[1] / pixels.length),
    Math.round(sum[2] / pixels.length),
  ];
}

function luminance(c: RGB): number {
  return 0.299 * c[0] + 0.587 * c[1] + 0.114 * c[2];
}

function detectType(palette: RGB[]): 'dark' | 'light' {
  const avg = palette.reduce((sum, c) => sum + luminance(c), 0) / palette.length;
  return avg < 128 ? 'dark' : 'light';
}

function hex(c: RGB): string {
  return '#' + c.map(v => v.toString(16).padStart(2, '0')).join('');
}

function darken(c: RGB, amount: number): RGB {
  return c.map(v => Math.max(0, Math.round(v * (1 - amount)))) as RGB;
}

function lighten(c: RGB, amount: number): RGB {
  return c.map(v => Math.min(255, Math.round(v + (255 - v) * amount))) as RGB;
}

function buildTheme(
  palette: RGB[],
  opts: { name: string; type: 'dark' | 'light'; imageUrl: string }
): VortexTheme {
  // Sort palette by luminance
  const sorted = [...palette].sort((a, b) => luminance(a) - luminance(b));

  let base: RGB, text: RGB, accent: RGB;
  if (opts.type === 'dark') {
    base = sorted[0]; // darkest
    text = sorted[sorted.length - 1]; // lightest
    accent = sorted[Math.floor(sorted.length * 0.6)]; // mid-bright, vivid
  } else {
    base = sorted[sorted.length - 1]; // lightest
    text = sorted[0]; // darkest
    accent = sorted[Math.floor(sorted.length * 0.4)]; // mid-dark, vivid
  }

  const surface0 = base;
  const surface1 = opts.type === 'dark' ? lighten(base, 0.05) : darken(base, 0.03);
  const surface2 = opts.type === 'dark' ? lighten(base, 0.1) : darken(base, 0.06);
  const surface3 = opts.type === 'dark' ? lighten(base, 0.15) : darken(base, 0.09);
  const surface4 = opts.type === 'dark' ? lighten(base, 0.2) : darken(base, 0.12);
  const surface5 = opts.type === 'dark' ? lighten(base, 0.25) : darken(base, 0.15);

  const textPrimary = text;
  const textSecondary = opts.type === 'dark' ? darken(text, 0.15) : lighten(text, 0.15);
  const textMuted = opts.type === 'dark' ? darken(text, 0.5) : lighten(text, 0.5);

  return {
    name: opts.name,
    author: 'Generated',
    type: opts.type,
    backgroundImage: opts.imageUrl,
    backgroundOpacity: 0.12,
    backgroundBlur: 16,
    colors: {
      'surface-0': hex(surface0),
      'surface-1': hex(surface1),
      'surface-2': hex(surface2),
      'surface-3': hex(surface3),
      'surface-4': hex(surface4),
      'surface-5': hex(surface5),

      'text-primary': hex(textPrimary),
      'text-secondary': hex(textSecondary),
      'text-muted': hex(textMuted),
      'text-inverse': hex(surface0),

      'border-subtle': hex(surface2),
      'border-default': hex(surface4),
      'border-strong': hex(surface5),

      'accent': hex(accent),
      'accent-hover': hex(lighten(accent, 0.15)),
      'accent-active': hex(darken(accent, 0.15)),
      'accent-muted': `rgba(${accent[0]},${accent[1]},${accent[2]},0.2)`,

      'success': '#a6e3a1',
      'error': '#f38ba8',
      'warning': '#f9e2af',
      'info': '#89b4fa',

      'scrollbar-thumb': hex(surface4),
      'scrollbar-thumb-hover': hex(surface5),
      'scrollbar-thumb-active': hex(textMuted),

      'ansi-black': hex(surface0),
      'ansi-red': '#f38ba8',
      'ansi-green': '#a6e3a1',
      'ansi-yellow': '#f9e2af',
      'ansi-blue': '#89b4fa',
      'ansi-magenta': '#cba6f7',
      'ansi-cyan': '#94e2d5',
      'ansi-white': hex(textPrimary),
      'ansi-bright-black': hex(surface5),
      'ansi-bright-red': '#f38ba8',
      'ansi-bright-green': '#a6e3a1',
      'ansi-bright-yellow': '#f9e2af',
      'ansi-bright-blue': '#89b4fa',
      'ansi-bright-magenta': '#cba6f7',
      'ansi-bright-cyan': '#94e2d5',
      'ansi-bright-white': '#ffffff',
    },
  };
}
