import { LitElement, html, css, nothing } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { extractThemeFromImage } from '../themes/extract.js';
import type { VortexTheme } from '../themes/theme.js';

const API_BASE = '';

const THEMES = [
  { id: 'dark', label: 'Dark' },
  { id: 'light', label: 'Light' },
  { id: 'monokai', label: 'Monokai' },
  { id: 'solarized-dark', label: 'Solarized Dark' },
  { id: 'solarized-light', label: 'Solarized Light' },
  { id: 'nord', label: 'Nord' },
  { id: 'dracula', label: 'Dracula' },
  { id: 'catppuccin-mocha', label: 'Catppuccin' },
  { id: 'system', label: 'System' },
];

@customElement('vx-theme-picker')
export class VxThemePicker extends LitElement {
  static styles = css`
    :host { display: block; }

    .theme-picker {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
    }

    .theme-card {
      padding: 8px 16px;
      border-radius: 6px;
      border: 1px solid var(--vx-border-default);
      background: var(--vx-surface-1);
      color: var(--vx-text-secondary);
      cursor: pointer;
      font-size: 12px;
      transition: border-color 0.15s, background 0.15s;
    }
    .theme-card:hover {
      border-color: var(--vx-border-strong);
      background: var(--vx-surface-2);
    }
    .theme-card.active {
      border-color: var(--vx-accent);
      background: var(--vx-surface-2);
      color: var(--vx-text-primary);
    }

    .section-title {
      font-size: 12px;
      font-weight: 600;
      color: var(--vx-text-secondary);
      margin-bottom: 8px;
    }

    .hint {
      font-size: 11px;
      color: var(--vx-text-muted);
      margin-top: 6px;
      display: block;
    }

    .bg-section { margin-top: 16px; }

    .bg-input-row {
      display: flex;
      gap: 8px;
      align-items: center;
    }
    .bg-input-row input[type="text"] {
      flex: 1;
      padding: 6px 10px;
      border-radius: 4px;
      border: 1px solid var(--vx-border-default);
      background: var(--vx-surface-1);
      color: var(--vx-text-primary);
      font-size: 12px;
    }
    .bg-input-row input[type="text"]:focus {
      border-color: var(--vx-accent);
      outline: none;
    }

    .bg-preview {
      margin-top: 8px;
      border-radius: 6px;
      overflow: hidden;
      border: 1px solid var(--vx-border-default);
      max-height: 120px;
    }
    .bg-preview img {
      width: 100%;
      height: 120px;
      object-fit: cover;
      display: block;
    }

    .btn-sm {
      padding: 5px 12px;
      border-radius: 4px;
      border: 1px solid var(--vx-border-default);
      background: var(--vx-surface-2);
      color: var(--vx-text-secondary);
      font-size: 11px;
      cursor: pointer;
      white-space: nowrap;
    }
    .btn-sm:hover {
      background: var(--vx-surface-3);
      border-color: var(--vx-border-strong);
    }
    .btn-sm.accent {
      background: var(--vx-accent);
      border-color: var(--vx-accent);
      color: var(--vx-text-inverse);
    }
    .btn-sm.accent:hover {
      background: var(--vx-accent-hover);
    }
    .btn-sm:disabled {
      opacity: 0.5;
      cursor: not-allowed;
    }

    .generate-section {
      margin-top: 16px;
      padding: 12px;
      border: 1px dashed var(--vx-border-default);
      border-radius: 6px;
      background: var(--vx-surface-1);
    }

    .editor-section { margin-top: 16px; }

    .color-grid {
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
      gap: 8px;
      margin-top: 8px;
    }
    .color-item {
      display: flex;
      align-items: center;
      gap: 8px;
    }
    .color-item input[type="color"] {
      width: 28px;
      height: 28px;
      border: 1px solid var(--vx-border-default);
      border-radius: 4px;
      padding: 0;
      cursor: pointer;
      background: none;
    }
    .color-item label {
      font-size: 11px;
      color: var(--vx-text-muted);
      flex: 1;
      min-width: 0;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .editor-actions {
      display: flex;
      gap: 8px;
      margin-top: 12px;
    }
  `;

  @property({ type: String }) theme = 'dark';
  @property({ type: String }) backgroundImage = '';
  @property({ type: String }) token = '';

  @state() private _bgInputValue = '';
  @state() private _generatingTheme = false;
  @state() private _editingTheme = false;
  @state() private _editColors: Record<string, string> = {};

  connectedCallback(): void {
    super.connectedCallback();
    this._bgInputValue = this.backgroundImage;
  }

  willUpdate(changed: Map<string, unknown>): void {
    if (changed.has('backgroundImage') && !this._editingTheme) {
      this._bgInputValue = this.backgroundImage;
    }
  }

  private _authHeaders(): HeadersInit {
    if (!this.token) return {};
    return { 'Authorization': `Bearer ${this.token}` };
  }

  private _selectTheme(id: string): void {
    this.dispatchEvent(new CustomEvent('theme-change', { detail: { theme: id }, bubbles: true, composed: true }));
  }

  private _applyBackgroundImage(): void {
    const url = this._bgInputValue.trim();
    this.dispatchEvent(new CustomEvent('background-change', { detail: { backgroundImage: url }, bubbles: true, composed: true }));
  }

  private _clearBackgroundImage(): void {
    this._bgInputValue = '';
    this.dispatchEvent(new CustomEvent('background-change', { detail: { backgroundImage: '' }, bubbles: true, composed: true }));
  }

  private async _generateThemeFromImage(type: 'dark' | 'light'): Promise<void> {
    if (!this.backgroundImage) return;
    this._generatingTheme = true;
    try {
      const theme = await extractThemeFromImage(this.backgroundImage, { name: `Generated ${type}`, type });
      const id = `generated-${type}-${Date.now()}`;
      await fetch(`${API_BASE}/api/themes`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', ...this._authHeaders() },
        body: JSON.stringify({ id, data: theme }),
      });
      this._selectTheme(id);
    } catch { /* ignore */ }
    this._generatingTheme = false;
  }

  private _startEditing(): void {
    const style = getComputedStyle(this);
    const tokens = [
      'surface-0', 'surface-1', 'surface-2', 'surface-3', 'surface-4', 'surface-5',
      'text-primary', 'text-secondary', 'text-muted', 'text-inverse',
      'border-subtle', 'border-default', 'border-strong',
      'accent', 'accent-hover', 'accent-active',
      'success', 'error', 'warning', 'info',
      'scrollbar-thumb', 'scrollbar-thumb-hover', 'scrollbar-thumb-active',
      'ansi-black', 'ansi-red', 'ansi-green', 'ansi-yellow',
      'ansi-blue', 'ansi-magenta', 'ansi-cyan', 'ansi-white',
      'ansi-bright-black', 'ansi-bright-red', 'ansi-bright-green', 'ansi-bright-yellow',
      'ansi-bright-blue', 'ansi-bright-magenta', 'ansi-bright-cyan', 'ansi-bright-white',
    ];
    const colors: Record<string, string> = {};
    for (const token of tokens) {
      const val = style.getPropertyValue(`--vx-${token}`).trim();
      colors[token] = val || '#888888';
    }
    this._editColors = colors;
    this._editingTheme = true;
  }

  private _applyEditedTheme(): void {
    const host = this.closest('vortex-app') as HTMLElement | null;
    if (host) {
      for (const [key, value] of Object.entries(this._editColors)) {
        host.style.setProperty(`--vx-${key}`, value);
      }
    }
  }

  private async _saveCustomTheme(): Promise<void> {
    const id = `custom-${Date.now()}`;
    const theme: VortexTheme = {
      name: 'Custom Theme',
      author: 'User',
      type: 'dark',
      colors: this._editColors as VortexTheme['colors'],
      backgroundImage: this.backgroundImage || undefined,
    };
    try {
      await fetch(`${API_BASE}/api/themes`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', ...this._authHeaders() },
        body: JSON.stringify({ id, data: theme }),
      });
      this._editingTheme = false;
      this._selectTheme(id);
    } catch { /* ignore */ }
  }

  render() {
    return html`
      <div class="section-title">Theme</div>
      <div class="theme-picker">
        ${THEMES.map(t => html`
          <button
            class="theme-card ${this.theme === t.id ? 'active' : ''}"
            @click=${() => this._selectTheme(t.id)}
          >
            <span>${t.label}</span>
          </button>
        `)}
      </div>
      <span class="hint">Choose a color theme for the application</span>

      <div class="bg-section">
        <div class="section-title">Background Image</div>
        <div class="bg-input-row">
          <input
            type="text"
            placeholder="Paste image URL..."
            .value=${this._bgInputValue}
            @input=${(e: InputEvent) => { this._bgInputValue = (e.target as HTMLInputElement).value; }}
            @keydown=${(e: KeyboardEvent) => { if (e.key === 'Enter') this._applyBackgroundImage(); }}
          />
          <button class="btn-sm accent" @click=${() => this._applyBackgroundImage()}>Apply</button>
          ${this.backgroundImage ? html`
            <button class="btn-sm" @click=${() => this._clearBackgroundImage()}>Clear</button>
          ` : nothing}
        </div>
        ${this.backgroundImage ? html`
          <div class="bg-preview">
            <img src=${this.backgroundImage} alt="Background preview" />
          </div>
        ` : nothing}
        <span class="hint">Set a background image behind the terminal (supports any URL)</span>
      </div>

      ${this.backgroundImage ? html`
        <div class="generate-section">
          <div class="section-title">Generate Theme from Image</div>
          <span class="hint">Extract colors from the background image to auto-generate a matching theme</span>
          <div style="margin-top: 8px; display: flex; gap: 8px;">
            <button class="btn-sm accent" ?disabled=${this._generatingTheme} @click=${() => this._generateThemeFromImage('dark')}>
              ${this._generatingTheme ? 'Generating...' : 'Generate Dark'}
            </button>
            <button class="btn-sm accent" ?disabled=${this._generatingTheme} @click=${() => this._generateThemeFromImage('light')}>
              ${this._generatingTheme ? 'Generating...' : 'Generate Light'}
            </button>
          </div>
        </div>
      ` : nothing}

      <div class="editor-section">
        <div class="section-title">Theme Editor</div>
        ${!this._editingTheme ? html`
          <button class="btn-sm" @click=${() => this._startEditing()}>Customize Current Theme</button>
          <span class="hint">Create a custom theme by editing color tokens</span>
        ` : html`
          <div class="color-grid">
            ${Object.entries(this._editColors).map(([key, value]) => html`
              <div class="color-item">
                <input
                  type="color"
                  .value=${value.startsWith('#') ? value : '#888888'}
                  @input=${(e: InputEvent) => {
                    this._editColors = { ...this._editColors, [key]: (e.target as HTMLInputElement).value };
                  }}
                />
                <label>${key}</label>
              </div>
            `)}
          </div>
          <div class="editor-actions">
            <button class="btn-sm accent" @click=${() => this._saveCustomTheme()}>Save as Custom Theme</button>
            <button class="btn-sm" @click=${() => this._applyEditedTheme()}>Preview</button>
            <button class="btn-sm" @click=${() => { this._editingTheme = false; }}>Cancel</button>
          </div>
        `}
      </div>
    `;
  }
}
