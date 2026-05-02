import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';

@customElement('vx-group-bar')
export class VxGroupBar extends LitElement {
  static styles = css`
    :host {
      display: grid;
      grid-template-columns: 1fr auto;
      background: var(--vx-surface-0);
      border-bottom: 1px solid var(--vx-border-subtle);
      overflow-x: auto;
      min-height: 33px;
    }

    .groups {
      display: grid;
      grid-auto-flow: column;
      grid-auto-columns: max-content;
    }

    .groups button {
      padding: 6px 16px;
      background: transparent;
      border: none;
      border-bottom: 2px solid transparent;
      color: var(--vx-text-muted);
      cursor: pointer;
      white-space: nowrap;
      font-size: 11px;
      text-transform: uppercase;
      letter-spacing: 0.06em;
    }
    .groups button.active {
      color: var(--vx-text-secondary);
      border-bottom-color: var(--vx-border-strong);
    }
    .groups button:hover:not(.active) {
      background: var(--vx-surface-4);
      color: var(--vx-text-secondary);
    }

    .actions {
      display: flex;
      align-items: stretch;
    }

    .action-btn {
      display: grid;
      place-items: center;
      padding: 0 10px;
      background: transparent;
      border: none;
      border-left: 1px solid var(--vx-border-subtle);
      color: var(--vx-text-muted);
      cursor: pointer;
      transition: color 0.15s, background 0.15s;
    }
    .action-btn:hover {
      color: var(--vx-text-secondary);
      background: var(--vx-surface-4);
    }
    .action-btn.active {
      color: var(--vx-accent);
    }
    .action-btn svg {
      width: 14px;
      height: 14px;
      fill: none;
      stroke: currentColor;
    }
  `;

  @property({ type: Array }) groups: string[] = [];
  @property({ type: String }) activeGroup = '';
  @property({ type: Boolean }) shellActive = false;
  @property({ type: Boolean }) configActive = false;
  @property({ type: Boolean }) settingsActive = false;
  @property({ type: Boolean }) showConfigToggle = false;

  private _dispatch(name: string, detail?: unknown) {
    this.dispatchEvent(new CustomEvent(name, { detail, bubbles: true, composed: true }));
  }

  render() {
    return html`
      <div class="groups">
        ${this.groups.map(
          (g) => html`
            <button
              class=${!this.configActive && !this.settingsActive && g === this.activeGroup ? 'active' : ''}
              @click=${() => this._dispatch('group-select', g)}
            >${g || '(default)'}</button>
          `
        )}
      </div>
      <div class="actions">
        <button
          class="action-btn ${this.shellActive ? 'active' : ''}"
          @click=${() => this._dispatch('shell-select')}
          title="Shell"
        ><svg viewBox="0 0 16 16" stroke-width="1.2" fill="none" stroke="currentColor"><polyline points="4,4 8,8 4,12"/><line x1="9" y1="12" x2="14" y2="12"/></svg></button>
        ${this.showConfigToggle ? html`
          <button class="action-btn ${this.configActive ? 'active' : ''}" @click=${() => this._dispatch('config-toggle')} title="Toggle config file preview"><svg viewBox="0 0 16 16" stroke-width="1.2" stroke-linejoin="round"><path d="M2 4.5V13a1 1 0 0 0 1 1h10a1 1 0 0 0 1-1V6a1 1 0 0 0-1-1H8L6.5 3.5H3A1 1 0 0 0 2 4.5z"/></svg></button>
        ` : ''}
        <button
          class="action-btn ${this.settingsActive ? 'active' : ''}"
          @click=${() => this._dispatch('settings-toggle')}
          title="Settings"
        ><svg viewBox="0 0 16 16" stroke-width="1.2" fill="none" stroke="currentColor" stroke-linejoin="round"><path d="M9.405 1.05c-.413-1.4-2.397-1.4-2.81 0l-.1.34a1.464 1.464 0 0 1-2.105.872l-.31-.17c-1.283-.698-2.686.705-1.987 1.987l.169.311c.446.82.023 1.841-.872 2.105l-.34.1c-1.4.413-1.4 2.397 0 2.81l.34.1a1.464 1.464 0 0 1 .872 2.105l-.17.31c-.698 1.283.705 2.686 1.987 1.987l.311-.169a1.464 1.464 0 0 1 2.105.872l.1.34c.413 1.4 2.397 1.4 2.81 0l.1-.34a1.464 1.464 0 0 1 2.105-.872l.31.17c1.283.698 2.686-.705 1.987-1.987l-.169-.311a1.464 1.464 0 0 1 .872-2.105l.34-.1c1.4-.413 1.4-2.397 0-2.81l-.34-.1a1.464 1.464 0 0 1-.872-2.105l.17-.31c.698-1.283-.705-2.686-1.987-1.987l-.311.169a1.464 1.464 0 0 1-2.105-.872l-.1-.34zM8 10.93a2.929 2.929 0 1 0 0-5.86 2.929 2.929 0 0 0 0 5.858z"/></svg></button>
      </div>
    `;
  }
}
