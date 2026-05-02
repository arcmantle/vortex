import { LitElement, html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';

// Fallback list for browsers without Local Font Access API
const FALLBACK_FONTS = [
  'JetBrains Mono', 'Fira Code', 'Cascadia Code', 'Cascadia Mono',
  'Source Code Pro', 'IBM Plex Mono', 'Hack', 'Iosevka', 'Inconsolata',
  'SF Mono', 'Menlo', 'Monaco', 'Consolas', 'DejaVu Sans Mono',
  'Ubuntu Mono', 'Roboto Mono', 'Droid Sans Mono', 'Liberation Mono',
  'Courier New', 'Andale Mono', 'PT Mono', 'Noto Sans Mono',
  'Space Mono', 'Victor Mono', 'Monaspace Neon', 'Monaspace Argon',
  'Berkeley Mono', 'Geist Mono', 'Commit Mono', 'Intel One Mono',
];

/** Loaded Google Fonts families persisted in localStorage */
const STORAGE_KEY = 'vx-google-fonts';

function loadGoogleFontsFromStorage(): string[] {
  try {
    return JSON.parse(localStorage.getItem(STORAGE_KEY) || '[]');
  } catch { return []; }
}

function saveGoogleFontsToStorage(fonts: string[]): void {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(fonts));
}

function injectGoogleFont(family: string): void {
  const id = `gfont-${family.replace(/\s+/g, '-').toLowerCase()}`;
  if (document.getElementById(id)) return;
  const link = document.createElement('link');
  link.id = id;
  link.rel = 'stylesheet';
  link.href = `https://fonts.googleapis.com/css2?family=${encodeURIComponent(family)}&display=swap`;
  document.head.appendChild(link);
}

@customElement('vx-font-picker')
export class VxFontPicker extends LitElement {
  static styles = css`
    :host {
      display: block;
      position: relative;
    }

    .input {
      width: 100%;
      padding: 5px 8px;
      background: var(--vx-surface-2);
      border: 1px solid var(--vx-border-strong);
      border-radius: 4px;
      color: var(--vx-text-primary);
      font-size: 13px;
      font-family: inherit;
      box-sizing: border-box;
      cursor: pointer;
    }
    .input:focus {
      outline: none;
      border-color: var(--vx-accent);
    }

    .dropdown {
      position: absolute;
      top: 100%;
      left: 0;
      right: 0;
      margin-top: 4px;
      max-height: 240px;
      overflow-y: auto;
      background: var(--vx-surface-2);
      border: 1px solid var(--vx-border-strong);
      border-radius: 4px;
      z-index: 20;
      box-shadow: 0 4px 12px rgba(0,0,0,0.4);
    }

    .section-label {
      padding: 4px 10px 2px;
      font-size: 10px;
      color: var(--vx-text-muted);
      text-transform: uppercase;
      letter-spacing: 0.05em;
      border-top: 1px solid var(--vx-border-subtle);
    }
    .section-label:first-child {
      border-top: none;
    }

    .item {
      display: block;
      width: 100%;
      padding: 6px 10px;
      border: none;
      background: transparent;
      color: var(--vx-text-primary);
      font-size: 13px;
      text-align: left;
      cursor: pointer;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    .item:hover,
    .item.highlighted {
      background: var(--vx-accent-muted);
    }
    .item.active {
      background: var(--vx-accent);
      color: var(--vx-text-inverse);
    }
    .item.active.highlighted {
      background: var(--vx-accent-active);
    }

    .empty {
      padding: 8px 10px;
      font-size: 12px;
      color: var(--vx-text-muted);
    }

    .gfont-row {
      display: flex;
      gap: 6px;
      padding: 6px 10px;
      border-top: 1px solid var(--vx-border-subtle);
    }
    .gfont-row input {
      flex: 1;
      padding: 4px 6px;
      background: var(--vx-surface-0);
      border: 1px solid var(--vx-border-strong);
      border-radius: 3px;
      color: var(--vx-text-primary);
      font-size: 11px;
    }
    .gfont-row input:focus {
      outline: none;
      border-color: var(--vx-accent);
    }
    .gfont-row button {
      padding: 4px 8px;
      border: 1px solid var(--vx-border-strong);
      border-radius: 3px;
      background: var(--vx-surface-3);
      color: var(--vx-text-secondary);
      font-size: 11px;
      cursor: pointer;
      white-space: nowrap;
    }
    .gfont-row button:hover {
      background: var(--vx-surface-5);
    }
    .gfont-link {
      padding: 4px 8px;
      border: 1px solid var(--vx-border-strong);
      border-radius: 3px;
      background: var(--vx-surface-3);
      color: var(--vx-text-secondary);
      font-size: 11px;
      cursor: pointer;
      white-space: nowrap;
      text-decoration: none;
      display: flex;
      align-items: center;
    }
    .gfont-link:hover {
      background: var(--vx-surface-5);
      color: var(--vx-accent);
    }
  `;

  @property({ type: String }) value = '';
  @property({ type: String }) placeholder = 'Search fonts...';

  @state() private _open = false;
  @state() private _search = '';
  @state() private _index = -1;
  @state() private _available: string[] = [];
  @state() private _googleFonts: string[] = [];
  @state() private _gfontInput = '';

  connectedCallback(): void {
    super.connectedCallback();
    this._googleFonts = loadGoogleFontsFromStorage();
    // Re-inject previously loaded Google Fonts
    for (const f of this._googleFonts) injectGoogleFont(f);
    void this._detectFonts();
  }

  protected updated(changed: Map<string, unknown>): void {
    if (changed.has('_index') && this._index >= 0) {
      const item = this.shadowRoot?.querySelector('.item.highlighted');
      item?.scrollIntoView({ block: 'nearest' });
    }
  }

  private async _detectFonts(): Promise<void> {
    // Try the Local Font Access API for full system font enumeration
    if ('queryLocalFonts' in window) {
      try {
        const fonts = await (window as unknown as { queryLocalFonts(): Promise<{ family: string }[]> }).queryLocalFonts();
        if (fonts.length > 0) {
          const families = new Set<string>();
          for (const f of fonts) families.add(f.family);
          this._available = [...families].sort((a, b) => a.localeCompare(b));
          return;
        }
      } catch {
        // Permission denied or not supported — fall through to probe
      }
    }

    // Fallback: detect installed fonts by measuring text against two baselines
    const canvas = document.createElement('canvas');
    const ctx = canvas.getContext('2d')!;
    const testStr = 'mmmmmmmmmmlli';
    const size = '72px';

    ctx.font = `${size} monospace`;
    const monoWidth = ctx.measureText(testStr).width;
    ctx.font = `${size} sans-serif`;
    const sansWidth = ctx.measureText(testStr).width;

    const available: string[] = [];
    for (const font of FALLBACK_FONTS) {
      ctx.font = `${size} "${font}", monospace`;
      const w1 = ctx.measureText(testStr).width;
      ctx.font = `${size} "${font}", sans-serif`;
      const w2 = ctx.measureText(testStr).width;
      // If either measurement differs from the baseline, font is installed
      if (w1 !== monoWidth || w2 !== sansWidth) {
        available.push(font);
      }
    }
    this._available = available;
  }

  private _addGoogleFont(): void {
    const family = this._gfontInput.trim();
    if (!family || this._googleFonts.includes(family)) return;
    injectGoogleFont(family);
    this._googleFonts = [...this._googleFonts, family];
    saveGoogleFontsToStorage(this._googleFonts);
    this._gfontInput = '';
  }

  private _select(font: string): void {
    this._open = false;
    this.dispatchEvent(new CustomEvent('font-change', { detail: font, bubbles: true, composed: true }));
  }

  private _onKeydown(e: KeyboardEvent): void {
    if (!this._open) return;
    const filtered = this._allFiltered;
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      this._index = Math.min(this._index + 1, filtered.length - 1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      this._index = Math.max(this._index - 1, 0);
    } else if (e.key === 'Enter' && this._index >= 0 && this._index < filtered.length) {
      e.preventDefault();
      this._select(filtered[this._index]);
    } else if (e.key === 'Escape') {
      this._open = false;
      (e.target as HTMLInputElement).blur();
    }
  }

  private get _filtered(): string[] {
    const q = this._search.toLowerCase();
    return this._available.filter(f => f.toLowerCase().includes(q));
  }

  private get _filteredGoogle(): string[] {
    const q = this._search.toLowerCase();
    return this._googleFonts.filter(f => f.toLowerCase().includes(q));
  }

  private get _allFiltered(): string[] {
    // Google fonts first, then system fonts (deduplicated)
    const google = this._filteredGoogle;
    const googleSet = new Set(google.map(g => g.toLowerCase()));
    const system = this._filtered.filter(f => !googleSet.has(f.toLowerCase()));
    return [...google, ...system];
  }

  render() {
    const all = this._allFiltered;
    const googleFiltered = this._filteredGoogle;
    const systemFiltered = this._filtered.filter(f => !this._googleFonts.some(g => g.toLowerCase() === f.toLowerCase()));
    const showSections = this._googleFonts.length > 0 && googleFiltered.length > 0;

    return html`
      <input
        class="input"
        type="text"
        .value=${this._open ? this._search : (this.value || '')}
        @focus=${() => {
          this._open = true;
          this._search = '';
          this._index = this.value ? all.indexOf(this.value) : -1;
        }}
        @input=${(e: InputEvent) => { this._search = (e.target as HTMLInputElement).value; this._index = -1; }}
        @blur=${(e: FocusEvent) => {
          const related = e.relatedTarget as Node | null;
          if (related && this.shadowRoot?.contains(related)) return;
          setTimeout(() => { this._open = false; }, 200);
        }}
        @keydown=${this._onKeydown}
        placeholder=${this.placeholder}
        style=${this.value ? `font-family: "${this.value}", monospace` : ''}
      />
      ${this._open ? html`
        <div class="dropdown">
          ${all.length > 0 ? html`
            ${showSections ? html`<div class="section-label">Google Fonts</div>` : ''}
            ${googleFiltered.map((f, i) => html`
              <button
                tabindex="-1"
                class="item ${f === this.value ? 'active' : ''} ${i === this._index ? 'highlighted' : ''}"
                style="font-family: '${f}', monospace"
                @mousedown=${(e: Event) => { e.preventDefault(); this._select(f); }}
              >${f}</button>
            `)}
            ${showSections && systemFiltered.length > 0 ? html`<div class="section-label">System Fonts</div>` : ''}
            ${systemFiltered.map((f, i) => {
              const idx = googleFiltered.length + i;
              return html`
                <button
                  tabindex="-1"
                  class="item ${f === this.value ? 'active' : ''} ${idx === this._index ? 'highlighted' : ''}"
                  style="font-family: '${f}', monospace"
                  @mousedown=${(e: Event) => { e.preventDefault(); this._select(f); }}
                >${f}</button>
              `;
            })}
          ` : html`<div class="empty">No matching fonts found</div>`}
          <div class="gfont-row">
            <input
              type="text"
              placeholder="Add Google Font name..."
              .value=${this._gfontInput}
              @input=${(e: InputEvent) => { this._gfontInput = (e.target as HTMLInputElement).value; }}
              @keydown=${(e: KeyboardEvent) => { if (e.key === 'Enter') { e.preventDefault(); e.stopPropagation(); this._addGoogleFont(); } }}
              @blur=${(e: FocusEvent) => {
                const related = e.relatedTarget as Node | null;
                if (related && this.shadowRoot?.contains(related)) return;
                setTimeout(() => { this._open = false; }, 200);
              }}
            />
            <button @mousedown=${(e: Event) => { e.preventDefault(); this._addGoogleFont(); }}>Add</button>
            <a class="gfont-link" href="https://fonts.google.com/?classification=Monospace" target="_blank" rel="noopener" @mousedown=${(e: Event) => e.stopPropagation()}>Browse</a>
          </div>
        </div>
      ` : ''}
    `;
  }
}
