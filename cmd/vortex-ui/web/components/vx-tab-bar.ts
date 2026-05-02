import { LitElement, html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import type { ShellInfo, ShellProfile, TerminalInfo } from '../types.js';
import { shellIcon } from '../lib/shell-icons.js';
import './vx-dropdown.js';

@customElement('vx-tab-bar')
export class VxTabBar extends LitElement {
  static styles = css`
    :host {
      display: flex;
      align-items: stretch;
      flex-wrap: wrap;
      overflow: visible;
      min-height: 33px;
      width: 100%;
    }

    button {
      display: flex;
      align-items: center;
      gap: 6px;
      padding: 8px 14px;
      background: transparent;
      border: none;
      border-bottom: 2px solid transparent;
      color: var(--vx-text-muted);
      cursor: pointer;
      white-space: nowrap;
      font-size: 13px;
    }
    button.active {
      color: var(--vx-text-primary);
      font-weight: 500;
      background: var(--vx-surface-2);
      border-bottom-color: var(--vx-accent);
    }
    button:hover:not(.active) {
      background: var(--vx-surface-2);
    }

    .shell-close {
      margin-left: 8px;
      font-size: 15px;
      line-height: 1;
      opacity: 0.4;
      cursor: pointer;
    }
    .shell-close:hover {
      opacity: 1;
      color: var(--vx-error);
    }

    .shell-icon {
      width: 14px;
      height: 14px;
      vertical-align: -2px;
      margin-right: 4px;
      flex-shrink: 0;
    }

    .status-dot {
      display: inline-block;
      width: 7px;
      height: 7px;
      border-radius: 50%;
      vertical-align: middle;
    }
    .pending  { background: var(--vx-text-muted); }
    .running  { background: var(--vx-success); }
    .success  { background: var(--vx-info); }
    .failure  { background: var(--vx-error); }
    .skipped  { background: var(--vx-border-default); border: 1px solid var(--vx-border-strong); }

    .new-shell-split {
      display: inline-flex;
      position: relative;
    }

    .new-shell-btn {
      padding: 8px 10px;
      background: transparent;
      border: none;
      color: var(--vx-text-muted);
      cursor: pointer;
      font-size: 16px;
      font-weight: 300;
      line-height: 1;
      border-radius: 3px 0 0 3px;
    }
    .new-shell-btn:hover {
      color: var(--vx-text-secondary);
      background: var(--vx-surface-2);
    }

    .new-shell-dropdown-btn {
      padding: 8px 6px;
      background: transparent;
      border: none;
      border-left: 1px solid var(--vx-border-default);
      color: var(--vx-text-muted);
      cursor: pointer;
      font-size: 10px;
      line-height: 1;
      border-radius: 0 3px 3px 0;
    }
    .new-shell-dropdown-btn:hover {
      color: var(--vx-text-secondary);
      background: var(--vx-surface-2);
    }

    .shell-picker-item {
      display: flex;
      align-items: center;
      gap: 8px;
      width: 100%;
      padding: 8px 12px;
      background: transparent;
      border: none;
      color: var(--vx-text-secondary);
      cursor: pointer;
      font-size: 13px;
      text-align: left;
    }
    .shell-picker-item:hover {
      background: var(--vx-border-default);
    }

    .shell-picker-color {
      width: 10px;
      height: 10px;
      border-radius: 50%;
      flex-shrink: 0;
    }

    .shell-picker-name {
      white-space: nowrap;
    }

    .spacer {
      flex: 1;
    }
  `;

  @property({ type: String }) mode: 'shell' | 'job' = 'shell';
  @property({ type: Array }) shells: ShellInfo[] = [];
  @property({ type: Array }) terminals: TerminalInfo[] = [];
  @property({ type: String }) activeId = '';
  @property({ type: Array }) profiles: ShellProfile[] = [];

  @state() private _showPicker = false;

  private _dispatch(name: string, detail?: unknown) {
    this.dispatchEvent(new CustomEvent(name, { detail, bubbles: true, composed: true }));
  }

  render() {
    return this.mode === 'shell' ? this._renderShellTabs() : this._renderJobTabs();
  }

  private _renderShellTabs() {
    return html`
      ${this.shells.map(
        (s) => html`
          <button
            class=${s.id === this.activeId ? 'active' : ''}
            @click=${() => this._dispatch('tab-select', s.id)}
            style=${s.color ? `border-bottom-color: ${s.color}` : ''}
            title=${s.label}
          >
            <span class="status-dot running" style=${s.color ? `background: ${s.color}` : ''}></span>
            ${s.label}
            <span class="shell-close" @click=${(e: Event) => { e.stopPropagation(); this._dispatch('tab-close', s.id); }} title="Close shell">&times;</span>
          </button>
        `
      )}
      <span class="new-shell-split">
        <button class="new-shell-btn" @click=${() => this._dispatch('shell-create')} title="New shell (default)">+</button>
        <button class="new-shell-dropdown-btn" @click=${(e: Event) => { e.stopPropagation(); this._showPicker = !this._showPicker; }} title="Choose shell">&#9662;</button>
        <vx-dropdown .open=${this._showPicker} @close=${() => { this._showPicker = false; }}>
          ${this.profiles.map((p) => html`
            <button class="shell-picker-item" @click=${() => { this._showPicker = false; this._dispatch('shell-create', p.id); }}>
              <span class="shell-picker-color" style="background: ${p.color || '#888'}"></span>
              ${shellIcon(p.icon)}
              <span class="shell-picker-name">${p.name}</span>
            </button>
          `)}
        </vx-dropdown>
      </span>
      <span class="spacer"></span>
    `;
  }

  private _renderJobTabs() {
    return html`
      ${this.terminals.map(
        (t) => html`
          <button
            class=${t.id === this.activeId ? 'active' : ''}
            @click=${() => this._dispatch('tab-select', t.id)}
          >
            <span class="status-dot ${t.status}"></span>
            ${t.label}
          </button>
        `
      )}
    `;
  }
}
