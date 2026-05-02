import { LitElement, html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { shellIcon } from '../lib/shell-icons.js';
import type { ShellProfile } from '../types.js';

const API_BASE = '';

@customElement('vx-profile-list')
export class VxProfileList extends LitElement {
  static styles = css`
    :host { display: block; }

    .profile-list {
      display: grid;
      grid-template-columns: auto 1fr auto;
      gap: 1px 0;
      background: var(--vx-border-default);
      border-radius: 6px;
      overflow: hidden;
    }

    .profile-item {
      display: grid;
      grid-template-columns: subgrid;
      grid-column: 1 / -1;
      align-items: center;
      gap: 10px;
      padding: 8px 12px;
      background: var(--vx-surface-2);
      cursor: grab;
    }
    .profile-item:active { cursor: grabbing; }
    .profile-item.dragging { opacity: 0.5; }
    .profile-item.drag-over { background: var(--vx-surface-3); }

    .profile-color {
      width: 10px;
      height: 10px;
      border-radius: 50%;
      flex-shrink: 0;
    }

    .profile-info {
      display: flex;
      align-items: center;
      gap: 6px;
      min-width: 0;
    }

    .shell-icon {
      width: 14px;
      height: 14px;
      flex-shrink: 0;
    }

    .profile-name {
      font-size: 13px;
      color: var(--vx-text-primary);
      font-weight: 500;
      white-space: nowrap;
    }

    .default-badge {
      font-size: 10px;
      color: var(--vx-text-muted);
      background: var(--vx-border-default);
      padding: 1px 5px;
      border-radius: 3px;
      margin-left: 2px;
    }

    .profile-command {
      font-size: 11px;
      color: var(--vx-text-muted);
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .profile-actions {
      display: flex;
      gap: 4px;
      align-items: center;
      justify-self: end;
    }

    .profile-actions button {
      padding: 3px 7px;
      background: transparent;
      border: 1px solid transparent;
      border-radius: 3px;
      color: var(--vx-text-muted);
      font-size: 11px;
      cursor: pointer;
    }
    .profile-actions button:hover {
      background: var(--vx-border-default);
      color: var(--vx-text-secondary);
      border-color: var(--vx-border-strong);
    }
    .profile-actions button.default-btn {
      color: var(--vx-text-muted);
      font-size: 10px;
    }
    .profile-actions button.default-btn:hover {
      color: var(--vx-text-secondary);
    }
    .profile-actions button.delete-btn:hover {
      color: var(--vx-error);
      border-color: var(--vx-error);
    }

    .add-section {
      margin-top: 8px;
      display: flex;
      gap: 8px;
    }
    .add-section button {
      padding: 5px 10px;
      background: var(--vx-surface-3);
      border: 1px solid var(--vx-border-strong);
      border-radius: 4px;
      color: var(--vx-text-secondary);
      font-size: 12px;
      cursor: pointer;
    }
    .add-section button:hover { background: var(--vx-surface-5); }

    .shell-header-actions {
      display: flex;
      gap: 8px;
      margin-bottom: 8px;
    }
    .shell-header-actions button {
      padding: 4px 10px;
      border: 1px solid var(--vx-border-strong);
      border-radius: 4px;
      background: var(--vx-surface-3);
      color: var(--vx-text-secondary);
      font-size: 12px;
      cursor: pointer;
    }
    .shell-header-actions button:hover { background: var(--vx-surface-5); }

    /* Edit form */
    .edit-overlay {
      position: absolute;
      inset: 0;
      background: rgba(0,0,0,0.6);
      display: grid;
      place-items: center;
      z-index: 10;
    }

    .edit-form {
      background: var(--vx-surface-2);
      border: 1px solid var(--vx-border-strong);
      border-radius: 8px;
      padding: 20px;
      min-width: 360px;
      max-width: 90%;
      display: flex;
      flex-direction: column;
      gap: 12px;
    }

    .edit-form h3 {
      margin: 0;
      font-size: 14px;
      color: var(--vx-text-primary);
    }

    .form-field {
      display: flex;
      flex-direction: column;
      gap: 4px;
    }
    .form-field label {
      font-size: 11px;
      color: var(--vx-text-muted);
      text-transform: uppercase;
      letter-spacing: 0.05em;
    }
    .form-field input {
      padding: 6px 10px;
      background: var(--vx-surface-0);
      border: 1px solid var(--vx-border-strong);
      border-radius: 4px;
      color: var(--vx-text-primary);
      font-size: 13px;
      font-family: inherit;
    }
    .form-field input:focus {
      outline: none;
      border-color: var(--vx-accent);
    }
    .form-field input[type="color"] {
      width: 40px;
      height: 30px;
      padding: 2px;
      cursor: pointer;
    }

    .form-actions {
      display: flex;
      justify-content: flex-end;
      gap: 8px;
      margin-top: 8px;
    }
    .form-actions button {
      padding: 6px 14px;
      border-radius: 4px;
      border: 1px solid var(--vx-border-strong);
      background: var(--vx-surface-3);
      color: var(--vx-text-secondary);
      font-size: 12px;
      cursor: pointer;
    }
    .form-actions button:hover { background: var(--vx-surface-5); }
    .form-actions button.primary {
      background: var(--vx-accent);
      border-color: var(--vx-accent);
      color: var(--vx-text-inverse);
    }
    .form-actions button.primary:hover { background: var(--vx-accent-hover); }
  `;

  @property({ type: String }) token = '';

  @state() private _profiles: ShellProfile[] = [];
  @state() private _editing: ShellProfile | null = null;
  @state() private _isNew = false;
  @state() private _dragIndex = -1;
  @state() private _dragOverIndex = -1;

  connectedCallback(): void {
    super.connectedCallback();
    this._fetchProfiles();
  }

  private _authHeaders(): HeadersInit {
    if (!this.token) return {};
    return { 'Authorization': `Bearer ${this.token}` };
  }

  private async _fetchProfiles(): Promise<void> {
    try {
      const res = await fetch(`${API_BASE}/api/settings/shells`, { headers: this._authHeaders() });
      if (!res.ok) return;
      this._profiles = (await res.json()) as ShellProfile[];
    } catch { /* ignore */ }
  }

  private async _saveProfiles(): Promise<void> {
    try {
      await fetch(`${API_BASE}/api/settings/shells`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json', ...this._authHeaders() },
        body: JSON.stringify(this._profiles),
      });
      this.dispatchEvent(new CustomEvent('profiles-changed', { detail: this._profiles, bubbles: true, composed: true }));
    } catch { /* ignore */ }
  }

  private async _redetect(): Promise<void> {
    try {
      const res = await fetch(`${API_BASE}/api/settings/shells/detect`, {
        method: 'POST',
        headers: this._authHeaders(),
      });
      if (!res.ok) return;
      this._profiles = (await res.json()) as ShellProfile[];
      await this._saveProfiles();
    } catch { /* ignore */ }
  }

  private _setDefault(id: string): void {
    this._profiles = this._profiles.map((p) => ({ ...p, default: p.id === id }));
    void this._saveProfiles();
  }

  private _deleteProfile(id: string): void {
    const wasDefault = this._profiles.find((p) => p.id === id)?.default;
    this._profiles = this._profiles.filter((p) => p.id !== id);
    if (wasDefault && this._profiles.length > 0) {
      this._profiles[0].default = true;
    }
    void this._saveProfiles();
  }

  private _editProfile(profile: ShellProfile): void {
    this._editing = { ...profile };
    this._isNew = false;
  }

  private _addProfile(): void {
    this._editing = { id: `custom-${Date.now()}`, name: '', command: '', color: '#888888' };
    this._isNew = true;
  }

  private _saveEdit(): void {
    if (!this._editing || !this._editing.name || !this._editing.command) return;
    if (this._isNew) {
      this._profiles = [...this._profiles, this._editing];
    } else {
      this._profiles = this._profiles.map((p) => p.id === this._editing!.id ? this._editing! : p);
    }
    this._editing = null;
    void this._saveProfiles();
  }

  private _cancelEdit(): void {
    this._editing = null;
  }

  private _onDragStart(index: number): void { this._dragIndex = index; }
  private _onDragOver(e: DragEvent, index: number): void { e.preventDefault(); this._dragOverIndex = index; }
  private _onDragEnd(): void {
    if (this._dragIndex >= 0 && this._dragOverIndex >= 0 && this._dragIndex !== this._dragOverIndex) {
      const profiles = [...this._profiles];
      const [moved] = profiles.splice(this._dragIndex, 1);
      profiles.splice(this._dragOverIndex, 0, moved);
      this._profiles = profiles;
      void this._saveProfiles();
    }
    this._dragIndex = -1;
    this._dragOverIndex = -1;
  }

  render() {
    return html`
      <div class="shell-header-actions">
        <button @click=${() => this._redetect()}>Re-detect shells</button>
      </div>
      <div class="profile-list">
        ${this._profiles.map((p, i) => html`
          <div
            class="profile-item ${this._dragIndex === i ? 'dragging' : ''} ${this._dragOverIndex === i ? 'drag-over' : ''}"
            draggable="true"
            @dragstart=${() => this._onDragStart(i)}
            @dragover=${(e: DragEvent) => this._onDragOver(e, i)}
            @dragend=${() => this._onDragEnd()}
          >
            <div class="profile-info">
              <span class="profile-color" style="background: ${p.color || '#888'}"></span>
              ${shellIcon(p.icon)}
              <span class="profile-name">${p.name}</span>
              ${p.default ? html`<span class="default-badge">default</span>` : ''}
            </div>
            <span class="profile-command">${p.command} ${(p.args ?? []).join(' ')}</span>
            <div class="profile-actions">
              ${!p.default ? html`<button class="default-btn" @click=${() => this._setDefault(p.id)} title="Set as default shell">Set default</button>` : ''}
              <button @click=${() => this._editProfile(p)} title="Edit">Edit</button>
              <button class="delete-btn" @click=${() => this._deleteProfile(p.id)} title="Delete">×</button>
            </div>
          </div>
        `)}
      </div>
      <div class="add-section">
        <button @click=${() => this._addProfile()}>+ Add Profile</button>
      </div>

      ${this._editing ? this._renderEditOverlay() : ''}
    `;
  }

  private _renderEditOverlay() {
    return html`
      <div class="edit-overlay" @click=${(e: Event) => { if (e.target === e.currentTarget) this._cancelEdit(); }}>
        <div class="edit-form">
          <h3>${this._isNew ? 'New Profile' : 'Edit Profile'}</h3>
          <div class="form-field">
            <label>Name</label>
            <input type="text" .value=${this._editing!.name} @input=${(e: InputEvent) => { this._editing = { ...this._editing!, name: (e.target as HTMLInputElement).value }; }} placeholder="e.g. Fish" />
          </div>
          <div class="form-field">
            <label>Command</label>
            <input type="text" .value=${this._editing!.command} @input=${(e: InputEvent) => { this._editing = { ...this._editing!, command: (e.target as HTMLInputElement).value }; }} placeholder="e.g. /usr/bin/fish" />
          </div>
          <div class="form-field">
            <label>Arguments</label>
            <input type="text" .value=${(this._editing!.args ?? []).join(' ')} @input=${(e: InputEvent) => { this._editing = { ...this._editing!, args: (e.target as HTMLInputElement).value.split(/\s+/).filter(Boolean) }; }} placeholder="e.g. -l --login" />
          </div>
          <div class="form-field">
            <label>Color</label>
            <input type="color" .value=${this._editing!.color || '#888888'} @input=${(e: InputEvent) => { this._editing = { ...this._editing!, color: (e.target as HTMLInputElement).value }; }} />
          </div>
          <div class="form-field">
            <label>Icon (built-in name or path)</label>
            <input type="text" .value=${this._editing!.icon || ''} @input=${(e: InputEvent) => { this._editing = { ...this._editing!, icon: (e.target as HTMLInputElement).value }; }} placeholder="e.g. zsh, bash, fish, terminal" />
          </div>
          <div class="form-field">
            <label>Font Family (optional, overrides global)</label>
            <input type="text" .value=${this._editing!.fontFamily || ''} @input=${(e: InputEvent) => { this._editing = { ...this._editing!, fontFamily: (e.target as HTMLInputElement).value }; }} placeholder="e.g. JetBrains Mono, monospace" />
          </div>
          <div class="form-field">
            <label>Font Size (optional)</label>
            <input type="number" min="8" max="36" .value=${String(this._editing!.fontSize || '')} @input=${(e: InputEvent) => { const v = parseInt((e.target as HTMLInputElement).value, 10); this._editing = { ...this._editing!, fontSize: v > 0 ? v : undefined }; }} placeholder="13" />
          </div>
          <div class="form-actions">
            <button @click=${() => this._cancelEdit()}>Cancel</button>
            <button class="primary" @click=${() => this._saveEdit()}>Save</button>
          </div>
        </div>
      </div>
    `;
  }
}
