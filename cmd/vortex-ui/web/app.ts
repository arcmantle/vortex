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
    .toolbar-btn svg {
      width: 14px;
      height: 14px;
      fill: currentColor;
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
  @state() private _instanceName = 'Vortex';
  private _gen = -1;
  private _groupInitialized = false;
  private _fetchSeq = 0;
  private _reportedReady = false;
  private _pollInterval?: ReturnType<typeof setInterval>;
  private _token = '';

  connectedCallback(): void {
    super.connectedCallback();
    const params = new URLSearchParams(window.location.search);
    this._token = params.get('token') || '';
    this._fetchTerminals();
    this._pollInterval = setInterval(() => this._fetchTerminals(), 3000);
  }

  disconnectedCallback(): void {
    super.disconnectedCallback();
    if (this._pollInterval !== undefined) {
      clearInterval(this._pollInterval);
      this._pollInterval = undefined;
    }
  }

  protected firstUpdated(): void {
    if (this._reportedReady) return;
    this._reportedReady = true;
    requestAnimationFrame(() => {
      (window as typeof window & { vortexAppReady?: () => void }).vortexAppReady?.();
    });
  }

  private async _fetchTerminals(): Promise<void> {
    const seq = ++this._fetchSeq;
    try {
      const res = await fetch(`${API_BASE}/api/terminals`, { headers: this._authHeaders() });
      if (!res.ok) return;
      if (seq !== this._fetchSeq) return; // stale response — a newer fetch is in flight
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
        this._gen = body.gen;
      }
      this._terminals = terms;
      // Initialise active group + tab on first load.
      if (!this._groupInitialized && terms.length > 0) {
        this._activeGroup = terms[0].group ?? '';
        this._groupInitialized = true;
      }
      // Reset active group if the current group no longer exists.
      if (terms.length > 0 && !terms.some((t) => (t.group ?? '') === this._activeGroup)) {
        this._activeGroup = terms[0].group ?? '';
      }
      // Reset active tab if the current selection no longer exists.
      if (this._activeId !== '' && !terms.some((t) => t.id === this._activeId)) {
        this._activeId = '';
      }
      if (this._activeId === '' && terms.length > 0) {
        const inGroup = terms.filter((t) => (t.group ?? '') === this._activeGroup);
        this._activeId = (inGroup[0] ?? terms[0]).id;
      }
    } catch {
      // server not yet up — retry on next poll
    }
  }

  /** Distinct ordered group names derived from the terminal list. */
  private get _groups(): string[] {
    const seen = new Set<string>();
    const groups: string[] = [];
    for (const t of this._terminals) {
      const g = t.group ?? '';
      if (!seen.has(g)) { seen.add(g); groups.push(g); }
    }
    return groups;
  }

  /** True when there is more than one distinct group. */
  private get _showGroupBar(): boolean {
    return this._groups.length > 1;
  }

  private _selectGroup(group: string): void {
    this._activeGroup = group;
    const first = this._terminals.find((t) => (t.group ?? '') === group);
    this._activeId = first?.id ?? '';
  }

  private _selectTab(id: string): void {
    this._activeId = id;
  }

  private _stopTerminal(id: string): void {
    void fetch(`${API_BASE}/api/terminals/${encodeURIComponent(id)}`, { method: 'DELETE', headers: this._authHeaders() }).catch(() => {});
  }

  private async _rerunTab(id: string): Promise<void> {
    try {
      const res = await fetch(`${API_BASE}/api/terminals/${encodeURIComponent(id)}/rerun`, { method: 'POST', headers: this._authHeaders() });
      if (!res.ok) return;
      this._fetchTerminals();
    } catch {
      // network error — ignored
    }
  }

  private _clearTerminal(): void {
    const term = this.shadowRoot?.querySelector('vortex-terminal') as import('./components/vortex-terminal.js').VortexTerminal | null;
    void term?.clearOutput();
  }

  private _rerunActiveTerminal(): void {
    if (!this._activeId) return;
    void this._rerunTab(this._activeId);
  }

  private _authHeaders(): HeadersInit {
    if (!this._token) return {};
    return { 'Authorization': `Bearer ${this._token}` };
  }

  render() {
    const visibleTerminals = this._terminals.filter(
      (t) => (t.group ?? '') === this._activeGroup
    );
    const active = this._terminals.find((t) => t.id === this._activeId);

    return html`
      <div class="header">
        ${this._showGroupBar
          ? html`
            <div class="group-bar">
              ${this._groups.map(
                (g) => html`
                  <button
                    class=${g === this._activeGroup ? 'active' : ''}
                    @click=${() => this._selectGroup(g)}
                  >${g || '(default)'}</button>
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
              </button>
            `
          )}
        </div>
      </div>
      <div class="panel">
        ${active
          ? html`
            <div class="panel-toolbar">
              ${active.status === 'running' || active.status === 'pending'
                ? html`<button class="toolbar-btn" @click=${() => this._stopTerminal(active.id)} title="Stop process"><svg viewBox="0 0 16 16"><rect x="3" y="3" width="10" height="10" rx="1"/></svg></button>`
                : html`<button class="toolbar-btn" @click=${() => this._rerunActiveTerminal()} title="Start process"><svg viewBox="0 0 16 16"><path d="M4.5 2.5v11l9-5.5z"/></svg></button>`}
              <button class="toolbar-btn" @click=${() => this._rerunActiveTerminal()} title="Rerun this job and downstream dependent jobs"><svg viewBox="0 0 16 16"><path d="M13.5 2v4h-4" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><path d="M13.15 5.97A5.5 5.5 0 1 1 7.5 2.5c1.58 0 3.02.67 4.03 1.74L13.5 6" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg></button>
              <button class="toolbar-btn" @click=${() => this._clearTerminal()} title="Clear terminal"><svg viewBox="0 0 16 16"><path d="M2 2l12 12M14 2L2 14" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg></button>
            </div>
            <vortex-terminal .terminal=${active} .token=${this._token}></vortex-terminal>
          `
          : html`<div class="empty">No terminals.</div>`}
      </div>
    `;
  }
}

