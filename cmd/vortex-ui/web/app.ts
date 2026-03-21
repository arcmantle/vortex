import { LitElement, html, css } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { type TerminalInfo } from './components/vortex-terminal.js';
import './components/vortex-terminal.js';

const API_BASE = '';

// ---------------------------------------------------------------------------
// vortex-app — root application shell with group + terminal tab bars
// ---------------------------------------------------------------------------

@customElement('vortex-app')
export class VortexApp extends LitElement {
  static styles = css`
    :host {
      display: grid;
      grid-template-rows: auto 1fr;
      height: 100vh;
      font-family: system-ui, sans-serif;
      background: #1e1e1e;
      color: #d4d4d4;
    }

    .header {
      display: grid;
      grid-template-rows: auto auto;
      background: #252526;
      border-bottom: 1px solid #3c3c3c;
    }

    /* Group tab bar — only visible when there are multiple groups */
    .group-bar {
      display: grid;
      grid-auto-flow: column;
      grid-auto-columns: max-content;
      background: #1e1e1e;
      border-bottom: 1px solid #2d2d2d;
      overflow-x: auto;
    }

    .group-bar button {
      padding: 6px 16px;
      background: transparent;
      border: none;
      border-bottom: 2px solid transparent;
      color: #888;
      cursor: pointer;
      white-space: nowrap;
      font-size: 11px;
      text-transform: uppercase;
      letter-spacing: 0.06em;
    }
    .group-bar button.active {
      color: #ccc;
      border-bottom-color: #555;
    }
    .group-bar button:hover:not(.active) {
      background: #2a2a2a;
      color: #bbb;
    }

    /* Process tab bar */
    .tab-bar {
      display: grid;
      grid-auto-flow: column;
      grid-auto-columns: max-content;
      overflow-x: auto;
    }

    .tab-bar button {
      padding: 8px 18px;
      background: transparent;
      border: none;
      border-bottom: 2px solid transparent;
      color: #9d9d9d;
      cursor: pointer;
      white-space: nowrap;
      font-size: 13px;
    }
    .tab-bar button.active {
      color: #fff;
      border-bottom-color: #0078d4;
    }
    .tab-bar button:hover:not(.active) {
      background: #2d2d2d;
    }

    .tab-bar .close-btn {
      margin-left: 6px;
      padding: 0 4px;
      font-size: 14px;
      line-height: 1;
      color: #666;
      background: transparent;
      border: none;
      border-bottom: none;
      cursor: pointer;
      border-radius: 3px;
    }
    .tab-bar .close-btn:hover {
      color: #fff;
      background: #444;
    }

    .panel {
      position: relative;
      display: grid;
      grid-template-columns: 1fr;
      grid-template-rows: 1fr;
      min-height: 0;
      overflow: hidden;
    }

    .panel-toolbar {
      position: absolute;
      top: 8px;
      right: 12px;
      z-index: 10;
      display: inline-grid;
      grid-auto-flow: column;
      gap: 8px;
      padding: 6px;
      border: 1px solid #454545;
      border-radius: 999px;
      background: rgba(37, 37, 38, 0.92);
      box-shadow: 0 10px 24px rgba(0, 0, 0, 0.25);
    }

    .toolbar-btn {
      width: 28px;
      height: 28px;
      border-radius: 50%;
      border: 1px solid #555;
      background: #333;
      color: #ccc;
      font-size: 16px;
      line-height: 1;
      cursor: pointer;
      display: grid;
      place-items: center;
      opacity: 0.6;
      transition: opacity 0.15s, background 0.15s;
    }
    .toolbar-btn:hover {
      opacity: 1;
      background: #444;
    }

    .status-dot {
      display: inline-block;
      width: 7px;
      height: 7px;
      border-radius: 50%;
      margin-right: 6px;
      vertical-align: middle;
    }
    .pending  { background: #6e7681; }
    .running  { background: #3fb950; }
    .success  { background: #58a6ff; }
    .failure  { background: #f14c4c; }
    .skipped  { background: #3c3c3c; border: 1px solid #555; }

    .panel vortex-terminal {
      min-height: 0;
    }

    .empty {
      display: grid;
      place-items: center;
      color: #555;
    }
  `;

  @state() private _terminals: TerminalInfo[] = [];
  @state() private _activeId = '';
  @state() private _activeGroup = '';
  @state() private _closedIds = new Set<string>();
  @state() private _instanceName = 'Vortex';
  private _gen = -1;
  private _reportedReady = false;

  connectedCallback(): void {
    super.connectedCallback();
    this._fetchTerminals();
    setInterval(() => this._fetchTerminals(), 3000);
  }

  protected firstUpdated(): void {
    if (this._reportedReady) return;
    this._reportedReady = true;
    requestAnimationFrame(() => {
      (window as typeof window & { vortexAppReady?: () => void }).vortexAppReady?.();
    });
  }

  private async _fetchTerminals(): Promise<void> {
    try {
      const res = await fetch(`${API_BASE}/api/terminals`);
      if (!res.ok) return;
      const body = (await res.json()) as {
        instance?: { name?: string };
        gen: number;
        terminals: TerminalInfo[];
      };
      if (body.instance?.name) {
        this._instanceName = body.instance.name;
        document.title = `Vortex - ${body.instance.name}`;
      }
      const terms = body.terminals;
      // Detect orchestrator restart via generation counter — clear closed tabs.
      if (body.gen !== this._gen) {
        if (this._gen !== -1) {
          this._closedIds = new Set<string>();
        }
        this._gen = body.gen;
      }
      this._terminals = terms;
      // Initialise active group + tab on first load.
      if (this._activeGroup === '' && terms.length > 0) {
        this._activeGroup = terms[0].group ?? '';
      }
      if (this._activeId === '' && terms.length > 0) {
        const inGroup = terms.filter((t) => (t.group ?? '') === this._activeGroup);
        this._activeId = (inGroup[0] ?? terms[0]).id;
      }
    } catch {
      // server not yet up — retry on next poll
    }
  }

  /** Distinct ordered group names derived from the terminal list, excluding fully closed groups. */
  private get _groups(): string[] {
    const seen = new Set<string>();
    const groups: string[] = [];
    for (const t of this._terminals) {
      if (this._closedIds.has(t.id))
			continue;

      const g = t.group ?? '';
      if (!seen.has(g)) { seen.add(g); groups.push(g); }
    }
    return groups;
  }

  /** True when there is more than one distinct (non-empty) group. */
  private get _showGroupBar(): boolean {
    const named = this._groups.filter((g) => g !== '');
    return named.length > 1;
  }

  private _selectGroup(group: string): void {
    this._activeGroup = group;
    // Auto-select first non-closed terminal in that group.
    const first = this._terminals.find((t) => (t.group ?? '') === group && !this._closedIds.has(t.id));
    if (first) this._activeId = first.id;
  }

  private _selectTab(id: string): void {
    this._activeId = id;
  }

  private _closeTab(id: string): void {
    // Tell the terminal component to kill its SSE before we remove it.
    const term = this.shadowRoot?.querySelector('vortex-terminal') as import('./components/vortex-terminal.js').VortexTerminal | null;
    if (term && term.terminal?.id === id) {
      term.closeStream();
    }
    // Kill the process on the server.
    fetch(`${API_BASE}/api/terminals/${encodeURIComponent(id)}`, { method: 'DELETE' });
    this._closedIds = new Set(this._closedIds).add(id);
    // If we just closed the active tab, switch to the next available one.
    if (this._activeId === id) {
      const remaining = this._terminals.filter(
        (t) => (t.group ?? '') === this._activeGroup && !this._closedIds.has(t.id)
      );
      if (remaining.length > 0) {
        this._activeId = remaining[0].id;
      } else {
        // Group is now empty — switch to the next available group.
        const availableGroups = this._groups;
        if (availableGroups.length > 0) {
          this._selectGroup(availableGroups[0]);
        } else {
          this._activeId = '';
          this._activeGroup = '';
        }
      }
    }
  }

  private async _rerunTab(id: string): Promise<void> {
    await fetch(`${API_BASE}/api/terminals/${encodeURIComponent(id)}/rerun`, { method: 'POST' });
    this._closedIds = new Set(this._closedIds);
    this._fetchTerminals();
  }

  private _clearTerminal(): void {
    const term = this.shadowRoot?.querySelector('vortex-terminal') as import('./components/vortex-terminal.js').VortexTerminal | null;
    term?.clearOutput();
  }

  private _rerunActiveTerminal(): void {
    if (!this._activeId) return;
    void this._rerunTab(this._activeId);
  }

  render() {
    const visibleTerminals = this._terminals.filter(
      (t) => (t.group ?? '') === this._activeGroup && !this._closedIds.has(t.id)
    );
    const active = this._terminals.find((t) => t.id === this._activeId);

    return html`
      <div class="header">
        <div class="tab-bar">
          <button class="active" type="button">${this._instanceName}</button>
        </div>
        ${this._showGroupBar
          ? html`
            <div class="group-bar">
              ${this._groups.filter((g) => g !== '').map(
                (g) => html`
                  <button
                    class=${g === this._activeGroup ? 'active' : ''}
                    @click=${() => this._selectGroup(g)}
                  >${g}</button>
                `
              )}
            </div>
          `
          : ''}
        <div class="tab-bar">
          ${visibleTerminals.map(
            (t) => html`
              <button
                class=${t.id === this._activeId ? 'active' : ''}
                @click=${() => this._selectTab(t.id)}
              >
                <span class="status-dot ${t.status}"></span>
                ${t.label}
                <span class="close-btn" @click=${(e: Event) => { e.stopPropagation(); this._closeTab(t.id); }} title="Close tab">&times;</span>
              </button>
            `
          )}
        </div>
      </div>
      <div class="panel">
        ${active
          ? html`
            <div class="panel-toolbar">
              <button class="toolbar-btn" @click=${() => this._rerunActiveTerminal()} title="Rerun this job and downstream dependent jobs">&#x21bb;</button>
              <button class="toolbar-btn" @click=${() => this._clearTerminal()} title="Clear terminal">&#x2715;</button>
            </div>
            <vortex-terminal .terminal=${active}></vortex-terminal>
          `
          : html`<div class="empty">No terminals.</div>`}
      </div>
    `;
  }
}

