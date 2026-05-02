import { LitElement, html, css, nothing } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import './vx-font-picker.js';
import './vx-theme-picker.js';
import './vx-profile-list.js';

const API_BASE = '';

type SettingsTab = 'appearance' | 'font' | 'shells';

@customElement('vortex-settings')
export class VortexSettings extends LitElement {
  static styles = css`
    :host {
      display: grid;
      grid-template-rows: auto auto 1fr;
      background: var(--vx-surface-0);
      overflow: hidden;
    }

    ::-webkit-scrollbar { width: 8px; height: 8px; }
    ::-webkit-scrollbar-track { background: transparent; }
    ::-webkit-scrollbar-thumb { background: var(--vx-scrollbar-thumb); border-radius: 4px; }
    ::-webkit-scrollbar-thumb:hover { background: var(--vx-scrollbar-thumb-hover); }
    ::-webkit-scrollbar-corner { background: transparent; }

    .header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 0 12px;
      height: 33px;
      box-sizing: border-box;
      background: var(--vx-surface-1);
      border-bottom: 1px solid var(--vx-border-default);
    }

    .title {
      font-size: 13px;
      font-weight: 500;
      color: var(--vx-text-secondary);
    }

    .actions button {
      padding: 4px 10px;
      border: 1px solid var(--vx-border-strong);
      border-radius: 4px;
      background: var(--vx-surface-3);
      color: var(--vx-text-secondary);
      font-size: 12px;
      cursor: pointer;
    }
    .actions button:hover {
      background: var(--vx-surface-5);
    }

    .tabs {
      display: flex;
      background: var(--vx-surface-1);
      border-bottom: 1px solid var(--vx-border-default);
      padding: 0 12px;
      gap: 0;
    }

    .tabs button {
      padding: 6px 14px;
      border: none;
      border-bottom: 2px solid transparent;
      background: transparent;
      color: var(--vx-text-muted);
      font-size: 12px;
      cursor: pointer;
      transition: color 0.1s, border-color 0.1s;
    }
    .tabs button:hover {
      color: var(--vx-text-secondary);
    }
    .tabs button.active {
      color: var(--vx-text-primary);
      border-bottom-color: var(--vx-accent);
    }

    .subtabs {
      display: flex;
      gap: 0;
      margin-bottom: 12px;
      border-bottom: 1px solid var(--vx-border-subtle);
    }

    .subtabs button {
      padding: 4px 12px;
      border: none;
      border-bottom: 2px solid transparent;
      background: transparent;
      color: var(--vx-text-muted);
      font-size: 11px;
      cursor: pointer;
    }
    .subtabs button:hover {
      color: var(--vx-text-secondary);
    }
    .subtabs button.active {
      color: var(--vx-text-primary);
      border-bottom-color: var(--vx-accent);
    }

    .body {
      overflow-y: auto;
      padding: 12px;
      display: flex;
      flex-direction: column;
      gap: 16px;
    }

    /* Shared field styles */
    .section-title {
      font-size: 11px;
      color: var(--vx-text-muted);
      text-transform: uppercase;
      letter-spacing: 0.05em;
      margin-bottom: 4px;
    }

    .field {
      display: flex;
      flex-direction: column;
      gap: 4px;
    }

    .field label {
      font-size: 12px;
      color: var(--vx-text-muted);
    }

    .field input, .field select {
      padding: 5px 8px;
      background: var(--vx-surface-2);
      border: 1px solid var(--vx-border-strong);
      border-radius: 4px;
      color: var(--vx-text-primary);
      font-size: 13px;
      font-family: inherit;
    }
    .field input:focus, .field select:focus {
      outline: none;
      border-color: var(--vx-accent);
    }

    .field-row {
      display: grid;
      grid-template-columns: 1fr 80px;
      gap: 12px;
      align-items: start;
    }

    .field-row .field {
      display: flex;
      flex-direction: column;
      gap: 4px;
    }

    .hint {
      font-size: 11px;
      color: var(--vx-text-muted);
      margin-top: 2px;
    }
  `;

  @property({ type: String }) token = '';
  @property({ type: String }) tab: SettingsTab = 'appearance';

  // General settings state
  @state() private _fontFamily = '';
  @state() private _fontSize = 13;
  @state() private _theme = 'dark';
  @state() private _backgroundImage = '';

  @state() private _activeTab: SettingsTab = 'appearance';

  connectedCallback(): void {
    super.connectedCallback();
    this._activeTab = this.tab;
    this._fetchSettings();
  }

  private _authHeaders(): HeadersInit {
    if (!this.token) return {};
    return { 'Authorization': `Bearer ${this.token}` };
  }

  // --- General settings ---
  private async _fetchSettings(): Promise<void> {
    try {
      const res = await fetch(`${API_BASE}/api/settings`, { headers: this._authHeaders() });
      if (!res.ok) return;
      const data = await res.json() as { fontFamily: string; fontSize: number; theme: string; backgroundImage: string };
      this._fontFamily = data.fontFamily || '';
      this._fontSize = data.fontSize || 13;
      this._theme = data.theme || 'dark';
      this._backgroundImage = data.backgroundImage || '';
    } catch { /* ignore */ }
  }

  private async _saveSettings(): Promise<void> {
    try {
      await fetch(`${API_BASE}/api/settings`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json', ...this._authHeaders() },
        body: JSON.stringify({ fontFamily: this._fontFamily, fontSize: this._fontSize, theme: this._theme, backgroundImage: this._backgroundImage }),
      });
      this.dispatchEvent(new CustomEvent('settings-changed', {
        detail: { fontFamily: this._fontFamily, fontSize: this._fontSize, theme: this._theme, backgroundImage: this._backgroundImage },
      }));
    } catch { /* ignore */ }
  }

  private _emitTabChange(): void {
    this.dispatchEvent(new CustomEvent('tab-change', {
      detail: this._activeTab,
    }));
  }

  render() {
    const isGeneral = this._activeTab === 'appearance' || this._activeTab === 'font';
    return html`
      <div class="header">
        <span class="title">Settings</span>
        <div class="actions">
          <button @click=${() => this.dispatchEvent(new CustomEvent('close'))}>Close</button>
        </div>
      </div>
      <div class="tabs">
        <button class=${isGeneral ? 'active' : ''} @click=${() => { this._activeTab = 'appearance'; this._emitTabChange(); }}>General</button>
        <button class=${this._activeTab === 'shells' ? 'active' : ''} @click=${() => { this._activeTab = 'shells'; this._emitTabChange(); }}>Shells</button>
      </div>
      <div class="body">
        ${isGeneral ? this._renderGeneral() : ''}
        ${this._activeTab === 'shells' ? this._renderShells() : ''}
      </div>
    `;
  }

  private _renderGeneral() {
    return html`
      <div class="subtabs">
        <button class=${this._activeTab === 'appearance' ? 'active' : ''} @click=${() => { this._activeTab = 'appearance'; this._emitTabChange(); }}>Appearance</button>
        <button class=${this._activeTab === 'font' ? 'active' : ''} @click=${() => { this._activeTab = 'font'; this._emitTabChange(); }}>Font</button>
      </div>
      ${this._activeTab === 'appearance' ? this._renderAppearance() : nothing}
      ${this._activeTab === 'font' ? this._renderFont() : nothing}
    `;
  }

  private _renderAppearance() {
    return html`
      <vx-theme-picker
        .theme=${this._theme}
        .backgroundImage=${this._backgroundImage}
        .token=${this.token}
        @theme-change=${(e: CustomEvent) => { this._theme = e.detail.theme; void this._saveSettings(); }}
        @background-change=${(e: CustomEvent) => { this._backgroundImage = e.detail.backgroundImage; void this._saveSettings(); }}
      ></vx-theme-picker>
    `;
  }

  private _renderFont() {
    return html`
      <div class="section-title">Terminal Font</div>
      <div class="field-row">
        <div class="field">
          <label>Font Family</label>
          <vx-font-picker
            .value=${this._fontFamily || ''}
            @font-change=${(e: CustomEvent) => { this._fontFamily = e.detail; void this._saveSettings(); }}
          ></vx-font-picker>
          <span class="hint">Applied to all terminals</span>
        </div>
        <div class="field">
          <label>Size</label>
          <input
            type="number"
            min="8"
            max="36"
            .value=${String(this._fontSize)}
            @input=${(e: InputEvent) => { const v = parseInt((e.target as HTMLInputElement).value, 10); if (v > 0 && v < 100) { this._fontSize = v; void this._saveSettings(); } }}
          />
        </div>
      </div>
    `;
  }

  private _renderShells() {
    return html`
      <vx-profile-list .token=${this.token}></vx-profile-list>
    `;
  }
}
