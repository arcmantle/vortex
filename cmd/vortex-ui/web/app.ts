import { LitElement, html, css } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { cache } from 'lit/directives/cache.js';
import type { TerminalInfo, ShellInfo, ShellProfile } from './types.js';
import './components/vortex-terminal.js';
import './components/vortex-settings.js';
import './components/vortex-config-preview.js';
import './components/vx-dropdown.js';
import './components/vx-tab-bar.js';
import './components/vx-group-bar.js';
import { applyTheme, dark } from './themes/index.js';
import { ThemeManager } from './themes/manager.js';

const API_BASE = '';

/** Internal constant for the shell group name. */
const SHELL_GROUP = '\x00shell';

// ---------------------------------------------------------------------------
// vortex-app — root application shell with group + terminal tab bars
// ---------------------------------------------------------------------------

@customElement('vortex-app')
export class VortexApp extends LitElement {
  static styles = css`
    :host {
      display: flex;
      flex-direction: column;
      height: 100vh;
      font-family: system-ui, sans-serif;
      background: var(--vx-surface-0);
      color: var(--vx-text-primary);
      position: relative;
    }

    :host::before {
      content: '';
      position: absolute;
      inset: 0;
      background-image: var(--vx-bg-image, none);
      background-size: cover;
      background-position: center;
      opacity: var(--vx-bg-opacity, 0);
      filter: blur(var(--vx-bg-blur, 0px));
      pointer-events: none;
      z-index: 0;
    }

    :host > * {
      position: relative;
      z-index: 1;
    }

    ::-webkit-scrollbar { width: 8px; height: 8px; }
    ::-webkit-scrollbar-track { background: transparent; }
    ::-webkit-scrollbar-thumb { background: var(--vx-scrollbar-thumb); border-radius: 4px; }
    ::-webkit-scrollbar-thumb:hover { background: var(--vx-scrollbar-thumb-hover); }
    ::-webkit-scrollbar-corner { background: transparent; }

    .disconnect-banner {
      display: flex;
      align-items: center;
      gap: 8px;
      padding: 8px 14px;
      background: var(--vx-error);
      color: var(--vx-text-inverse);
      font-size: 13px;
      font-weight: 500;
    }
    .disconnect-banner svg {
      width: 14px;
      height: 14px;
      flex-shrink: 0;
    }
    .disconnect-banner code {
      background: rgba(255,255,255,0.1);
      padding: 1px 5px;
      border-radius: 3px;
      font-size: 12px;
    }

    .header {
      display: flex;
      flex-direction: column;
      background: var(--vx-surface-1);
      border-bottom: 1px solid var(--vx-border-default);
      z-index: 2;
    }

    /* Process tab bar */
    .tab-bar {
      display: flex;
      align-items: stretch;
      flex-wrap: wrap;
      overflow: visible;
      min-height: 33px;
      background: var(--vx-surface-1);
      border-bottom: 1px solid var(--vx-border-default);
    }

    .panel {
      position: relative;
      display: flex;
      flex-direction: column;
      flex: 1;
      min-height: 0;
      overflow: hidden;
    }

    .panel vortex-terminal {
      flex: 1;
      min-height: 0;
    }

    .empty {
      display: grid;
      place-items: center;
      color: var(--vx-border-strong);
    }

    vortex-config-preview {
      position: absolute;
      inset: 0;
      z-index: 20;
    }

  `;

  @state() private _terminals: TerminalInfo[] = [];
  @state() private _shells: ShellInfo[] = [];
  @state() private _profiles: ShellProfile[] = [];
  @state() private _activeId = '';
  @state() private _activeGroup = '';
  @state() private _instanceName = 'Vortex';
  @state() private _jobFontFamily = '';
  private _previousGroup = '';
  private _previousId = '';
  @state() private _jobFontSize = 0;
  @state() private _configContent = '';
  @state() private _configPath = '';
  @state() private _disconnected = false;
  private _gen = -1;
  private _groupInitialized = false;
  private _fetchSeq = 0;
  private _reportedReady = false;
  private _pollInterval?: ReturnType<typeof setInterval>;
  private _token = '';
  private _failedPolls = 0;
  private _themeManager!: ThemeManager;

  connectedCallback(): void {
    super.connectedCallback();
    this._themeManager = new ThemeManager(this);
    this._themeManager.setTheme('dark');
    const params = new URLSearchParams(window.location.search);
    this._token = params.get('token') || '';
    // Restore navigation state from URL.
    const urlGroup = params.get('group');
    const urlTab = params.get('tab');
    if (urlGroup) {
      this._activeGroup = urlGroup === 'shell' ? SHELL_GROUP : urlGroup;
      this._groupInitialized = true;
      if (this._activeGroup === SHELL_GROUP) this._fetchProfiles();
      if (this._activeGroup === '@editor') void this._loadConfigContent();
    }
    if (urlTab) {
      this._activeId = urlTab;
    } else if (this._activeGroup === '@settings') {
      this._activeId = 'appearance';
    } else if (this._activeGroup === '@editor') {
      this._activeId = 'config';
    }
    this._fetchTerminals();
    this._fetchGeneralSettings();
    this._pollInterval = setInterval(() => this._fetchTerminals(), 3000);
    window.addEventListener('popstate', this._onPopState);
  }

  disconnectedCallback(): void {
    super.disconnectedCallback();
    this._themeManager.destroy();
    window.removeEventListener('popstate', this._onPopState);
    if (this._pollInterval !== undefined) {
      clearInterval(this._pollInterval);
      this._pollInterval = undefined;
    }
  }

  private _onPopState = (): void => {
    const params = new URLSearchParams(window.location.search);
    const urlGroup = params.get('group');
    const urlTab = params.get('tab');
    this._activeGroup = urlGroup === 'shell' ? SHELL_GROUP : (urlGroup || this._previousGroup || this._groups[0] || '');
    this._activeId = urlTab || '';
    if (this._activeGroup === SHELL_GROUP) this._fetchProfiles();
    if (this._activeGroup === '@editor') void this._loadConfigContent();
  };

  protected firstUpdated(): void {
    if (this._reportedReady) return;
    this._reportedReady = true;
    requestAnimationFrame(() => {
      (window as typeof window & { vortexAppReady?: () => void }).vortexAppReady?.();
    });
  }

  private async _fetchGeneralSettings(): Promise<void> {
    try {
      await this._themeManager.loadCustomThemes(API_BASE, this._authHeaders());
      const res = await fetch('/api/settings', { headers: this._authHeaders() });
      if (!res.ok) return;
      const data = await res.json() as { fontFamily: string; fontSize: number; theme: string; backgroundImage: string };
      this._jobFontFamily = data.fontFamily || '';
      this._jobFontSize = data.fontSize || 0;
      if (data.theme) {
        this._themeManager.setTheme(data.theme);
      }
      this._applyBackgroundImage(data.backgroundImage || '');
    } catch {
      // ignore
    }
  }

  private async _fetchTerminals(): Promise<void> {
    const seq = ++this._fetchSeq;
    try {
      const res = await fetch(`${API_BASE}/api/terminals`, { headers: this._authHeaders() });
      if (!res.ok) return;
      if (seq !== this._fetchSeq) return; // stale response — a newer fetch is in flight
      this._failedPolls = 0;
      if (this._disconnected) this._disconnected = false;
      const body = (await res.json()) as {
        instance?: { name?: string };
        gen: number;
        terminals: TerminalInfo[];
        shells?: ShellInfo[];
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
      this._shells = body.shells ?? [];
      // Initialise active group + tab on first load.
      if (!this._groupInitialized) {
        this._activeGroup = terms.length > 0 ? (terms[0].group ?? '') : SHELL_GROUP;
        this._groupInitialized = true;
        if (this._activeGroup === SHELL_GROUP) this._fetchProfiles();
      }
      // Reset active group if the current group no longer exists.
      // Skip reset when viewing a system group (e.g. @settings, @editor).
      const allGroups = this._groups;
      if (!this._activeGroup.startsWith('@') && terms.length > 0 && !allGroups.includes(this._activeGroup)) {
        this._activeGroup = allGroups[0] ?? '';
      }
      // Reset active tab if the current selection no longer exists.
      if (!this._activeGroup.startsWith('@')) {
        const allVisibleIds = this._activeGroup === SHELL_GROUP
          ? this._shells.map((s) => s.id)
          : terms.filter((t) => (t.group ?? '') === this._activeGroup).map((t) => t.id);
        if (this._activeId !== '' && !allVisibleIds.includes(this._activeId)) {
          this._activeId = '';
        }
        if (this._activeId === '' && allVisibleIds.length > 0) {
          this._activeId = allVisibleIds[0];
        }
      }
    } catch {
      this._failedPolls++;
      if (this._failedPolls >= 2) this._disconnected = true;
    }
  }

  /** Distinct ordered group names derived from the terminal list, plus the shell group. */
  private get _groups(): string[] {
    const seen = new Set<string>();
    const groups: string[] = [];
    for (const t of this._terminals) {
      const g = t.group ?? '';
      if (!seen.has(g)) { seen.add(g); groups.push(g); }
    }
    // Shell group is always last.
    groups.push(SHELL_GROUP);
    return groups;
  }

  /** True when there is more than one distinct group (always true now due to shell group). */
  private get _showGroupBar(): boolean {
    return this._groups.length > 1;
  }

  private _selectGroup(group: string): void {
    this._activeGroup = group;
    if (group === SHELL_GROUP) {
      this._activeId = this._shells[0]?.id ?? '';
      this._fetchProfiles();
    } else {
      const first = this._terminals.find((t) => (t.group ?? '') === group);
      this._activeId = first?.id ?? '';
    }
    this._syncURL();
  }

  private async _fetchProfiles(): Promise<void> {
    try {
      const res = await fetch(`${API_BASE}/api/settings/shells`, { headers: this._authHeaders() });
      if (!res.ok) return;
      this._profiles = (await res.json()) as ShellProfile[];
    } catch {
      // ignore
    }
  }

  private _selectTab(id: string): void {
    this._activeId = id;
    this._syncURL();
  }

  /** Update the URL to reflect current navigation state. */
  private _syncURL(push = false): void {
    const params = new URLSearchParams();
    if (this._token) params.set('token', this._token);
    const group = this._activeGroup === SHELL_GROUP ? 'shell' : this._activeGroup;
    if (group) params.set('group', group);
    if (this._activeId) params.set('tab', this._activeId);
    const qs = params.toString();
    const url = qs ? `?${qs}` : window.location.pathname;
    if (push) history.pushState(null, '', url);
    else history.replaceState(null, '', url);
  }

  private _stopTerminal(id: string): void {
    void fetch(`${API_BASE}/api/terminals/${encodeURIComponent(id)}`, { method: 'DELETE', headers: this._authHeaders() })
      .then((res) => { if (!res.ok) this._activeTerminalEl()?.writeStatus('stop failed'); })
      .catch(() => { this._activeTerminalEl()?.writeStatus('stop failed'); });
  }

  private async _rerunTab(id: string): Promise<void> {
    try {
      const res = await fetch(`${API_BASE}/api/terminals/${encodeURIComponent(id)}/rerun`, { method: 'POST', headers: this._authHeaders() });
      if (!res.ok) {
        this._activeTerminalEl()?.writeStatus('rerun failed');
        return;
      }
      this._fetchTerminals();
    } catch {
      this._activeTerminalEl()?.writeStatus('rerun failed');
    }
  }

  private async _openConfigFile(): Promise<void> {
    if (this._activeGroup === '@editor') {
      this._closeSystemGroup();
      return;
    }
    this._navigateToSystemGroup('@editor', 'config');
    await this._loadConfigContent();
  }

  private async _loadConfigContent(): Promise<void> {
    try {
      const res = await fetch(`${API_BASE}/api/config-file`, { headers: this._authHeaders() });
      if (!res.ok) {
        this._closeSystemGroup();
        return;
      }
      const body = (await res.json()) as { path: string; content: string };
      this._configPath = body.path || '';
      this._configContent = body.content || '';
    } catch {
      this._closeSystemGroup();
    }
  }

  private async _openConfigInEditor(): Promise<void> {
    if (!this._configPath) return;
    try {
      await fetch(`${API_BASE}/api/open-path`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', ...this._authHeaders() },
        body: JSON.stringify({ path: this._configPath }),
      });
    } catch {
      // ignore
    }
  }

  private _rerunActiveTerminal(): void {
    if (!this._activeId) return;
    void this._rerunTab(this._activeId);
  }

  private _activeTerminalEl(): import('./components/vortex-terminal.js').VortexTerminal | null {
    return this.shadowRoot?.querySelector('vortex-terminal') as import('./components/vortex-terminal.js').VortexTerminal | null;
  }

  private async _createShell(profileId?: string): Promise<void> {
    try {
      const body = profileId ? JSON.stringify({ profile_id: profileId }) : undefined;
      const headers: HeadersInit = { ...this._authHeaders() };
      if (body) (headers as Record<string, string>)['Content-Type'] = 'application/json';
      const res = await fetch(`${API_BASE}/api/shells`, { method: 'POST', headers, body });
      if (!res.ok) return;
      const shell = (await res.json()) as ShellInfo;
      this._shells = [...this._shells, shell];
      this._activeId = shell.id;
      this._syncURL();
    } catch {
      // ignore
    }
  }

  private async _closeShell(id: string): Promise<void> {
    try {
      await fetch(`${API_BASE}/api/shells/${encodeURIComponent(id)}`, { method: 'DELETE', headers: this._authHeaders() });
      this._shells = this._shells.filter((s) => s.id !== id);
      if (this._activeId === id) {
        this._activeId = this._shells[0]?.id ?? '';
      }
      this._syncURL();
    } catch {
      // ignore
    }
  }

  /** Whether the currently active tab is a shell. */
  private get _isShellActive(): boolean {
    return this._activeGroup === SHELL_GROUP;
  }

  private _toggleSettings(): void {
    if (this._activeGroup === '@settings') {
      this._closeSystemGroup();
    } else {
      this._navigateToSystemGroup('@settings', 'appearance');
    }
  }

  private _navigateToSystemGroup(group: string, tab: string): void {
    if (!this._activeGroup.startsWith('@')) {
      this._previousGroup = this._activeGroup;
      this._previousId = this._activeId;
    }
    this._activeGroup = group;
    this._activeId = tab;
    this._syncURL(true);
  }

  private _closeSystemGroup(): void {
    this._activeGroup = this._previousGroup || this._groups[0] || '';
    this._activeId = this._previousId || '';
    this._previousGroup = '';
    this._previousId = '';
    // Re-validate active tab in the restored group
    if (this._activeGroup === SHELL_GROUP) {
      if (!this._shells.find(s => s.id === this._activeId)) {
        this._activeId = this._shells[0]?.id ?? '';
      }
    } else if (this._activeGroup) {
      const inGroup = this._terminals.filter(t => (t.group ?? '') === this._activeGroup);
      if (!inGroup.find(t => t.id === this._activeId)) {
        this._activeId = inGroup[0]?.id ?? '';
      }
    }
    this._syncURL();
  }

  private _onGeneralSettingsChanged(e: CustomEvent): void {
    const { fontFamily, fontSize, theme, backgroundImage } = e.detail as { fontFamily: string; fontSize: number; theme?: string; backgroundImage?: string };
    this._jobFontFamily = fontFamily;
    this._jobFontSize = fontSize;
    if (theme) {
      this._themeManager.setTheme(theme);
    }
    this._applyBackgroundImage(backgroundImage ?? '');
  }

  private _applyBackgroundImage(url: string): void {
    if (url) {
      this.style.setProperty('--vx-bg-image', `url("${url}")`);
      this.style.setProperty('--vx-bg-opacity', '0.15');
      this.style.setProperty('--vx-bg-blur', '12px');
      document.documentElement.style.setProperty('--vx-bg-image', `url("${url}")`);
    } else {
      this.style.setProperty('--vx-bg-image', 'none');
      this.style.setProperty('--vx-bg-opacity', '0');
      document.documentElement.style.setProperty('--vx-bg-image', 'none');
    }
  }

  private _shellFontFamily(shell?: ShellInfo): string {
    if (!shell?.profile_id) return '';
    const profile = this._profiles.find((p) => p.id === shell.profile_id);
    return profile?.fontFamily || '';
  }

  private _shellFontSize(shell?: ShellInfo): number {
    if (!shell?.profile_id) return 0;
    const profile = this._profiles.find((p) => p.id === shell.profile_id);
    return profile?.fontSize || 0;
  }

  private _onProfilesChanged(e: CustomEvent): void {
    this._profiles = e.detail as ShellProfile[];
  }

  private _authHeaders(): HeadersInit {
    if (!this._token) return {};
    return { 'Authorization': `Bearer ${this._token}` };
  }

  render() {
    const isSystemGroup = this._activeGroup.startsWith('@');
    const isShellGroup = this._activeGroup === SHELL_GROUP;
    const visibleTerminals = isShellGroup || isSystemGroup
      ? []
      : this._terminals.filter((t) => (t.group ?? '') === this._activeGroup);
    const active = isShellGroup || isSystemGroup
      ? undefined
      : this._terminals.find((t) => t.id === this._activeId);
    const activeShell = isShellGroup
      ? this._shells.find((s) => s.id === this._activeId)
      : undefined;
    // Build a TerminalInfo-compatible object for the shell so vortex-terminal can render it.
    const activeShellInfo: TerminalInfo | undefined = activeShell
      ? { id: activeShell.id, label: activeShell.label, command: '', group: SHELL_GROUP, needs: [], status: 'running' }
      : undefined;

    return html`
      ${this._disconnected ? html`
        <div class="disconnect-banner">
          <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="8" cy="8" r="6.5"/><line x1="5" y1="5" x2="11" y2="11"/></svg>
          Host disconnected — run <code>vortex</code> to restart
        </div>
      ` : ''}
      <div class="header">
        <vx-group-bar
          .groups=${this._showGroupBar ? this._groups.filter((g: string) => g !== SHELL_GROUP) : []}
          .activeGroup=${this._activeGroup}
          .shellActive=${this._activeGroup === SHELL_GROUP && !isSystemGroup}
          .configActive=${this._activeGroup === '@editor'}
          .settingsActive=${this._activeGroup === '@settings'}
          .showConfigToggle=${this._terminals.length > 0}
          @group-select=${(e: CustomEvent) => this._selectGroup(e.detail as string)}
          @shell-select=${() => this._selectGroup(SHELL_GROUP)}
          @config-toggle=${() => this._openConfigFile()}
          @settings-toggle=${() => this._toggleSettings()}
        ></vx-group-bar>
      </div>
      <div class="panel">
        ${!isSystemGroup
          ? html`
            <div class="tab-bar">
              <vx-tab-bar
                .mode=${isShellGroup ? 'shell' : 'job'}
                .shells=${this._shells}
                .terminals=${visibleTerminals}
                .activeId=${this._activeId}
                .profiles=${this._profiles}
                @tab-select=${(e: CustomEvent) => this._selectTab(e.detail as string)}
                @tab-close=${(e: CustomEvent) => this._closeShell(e.detail as string)}
                @shell-create=${(e: CustomEvent) => this._createShell(e.detail as string | undefined)}
              ></vx-tab-bar>
            </div>
          `
          : ''}
        ${this._activeGroup === '@editor'
          ? html`
            <vortex-config-preview
              .path=${this._configPath}
              .content=${this._configContent}
              @close=${() => this._closeSystemGroup()}
              @open-in-editor=${() => this._openConfigInEditor()}
            ></vortex-config-preview>
          `
          : ''}
        ${cache(this._activeGroup === '@settings'
          ? html`
            <vortex-settings
              .token=${this._token}
              .tab=${this._activeId || 'appearance'}
              @close=${() => this._closeSystemGroup()}
              @tab-change=${(e: CustomEvent) => { this._activeId = e.detail as string; this._syncURL(); }}
              @settings-changed=${(e: CustomEvent) => this._onGeneralSettingsChanged(e)}
              @profiles-changed=${(e: CustomEvent) => this._onProfilesChanged(e)}
            ></vortex-settings>
          `
          : active
          ? html`
            <vortex-terminal .terminal=${active} .token=${this._token} .fontFamily=${this._jobFontFamily} .fontSize=${this._jobFontSize} .showToolbar=${true}
              @terminal-stop=${(e: CustomEvent) => this._stopTerminal(e.detail.id)}
              @terminal-rerun=${() => this._rerunActiveTerminal()}
            ></vortex-terminal>
          `
          : activeShellInfo
          ? html`
            <vortex-terminal .terminal=${activeShellInfo} .token=${this._token} .fontFamily=${this._shellFontFamily(activeShell)} .fontSize=${this._shellFontSize(activeShell)}></vortex-terminal>
          `
          : html`<div class="empty">${isShellGroup ? 'Click + to open a shell.' : 'No terminals.'}</div>`)}
      </div>
    `;
  }
}
