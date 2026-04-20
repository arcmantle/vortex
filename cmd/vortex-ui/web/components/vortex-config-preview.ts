import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { EditorView, basicSetup } from 'codemirror';
import { EditorState } from '@codemirror/state';
import { yaml } from '@codemirror/lang-yaml';
import { oneDark } from '@codemirror/theme-one-dark';

@customElement('vortex-config-preview')
export class VortexConfigPreview extends LitElement {
  static styles = css`
    :host {
      display: grid;
      grid-template-rows: auto 1fr;
      background: #1e1e1e;
      overflow: hidden;
    }

    .header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 8px 12px;
      background: #252526;
      border-bottom: 1px solid #3c3c3c;
    }

    .path {
      font-size: 12px;
      color: #999;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      min-width: 0;
    }

    .actions {
      display: flex;
      gap: 8px;
      flex-shrink: 0;
      margin-left: 12px;
    }

    .actions button {
      padding: 4px 10px;
      border: 1px solid #555;
      border-radius: 4px;
      background: #333;
      color: #ccc;
      font-size: 12px;
      cursor: pointer;
    }
    .actions button:hover {
      background: #444;
    }

    .editor {
      overflow: auto;
      min-height: 0;
    }

    .editor .cm-editor {
      height: 100%;
    }

    .editor .cm-scroller {
      overflow: auto;
    }
  `;

  @property() path = '';
  @property() content = '';

  private _editorView?: EditorView;
  private _editorContainer?: HTMLDivElement;
  private _lastContent = '';

  protected firstUpdated(): void {
    this._editorContainer = this.shadowRoot!.querySelector('.editor') as HTMLDivElement;
    this._createEditor();
  }

  updated(changed: Map<string, unknown>): void {
    if (changed.has('content') && this._editorView && this.content !== this._lastContent) {
      this._lastContent = this.content;
      this._editorView.dispatch({
        changes: { from: 0, to: this._editorView.state.doc.length, insert: this.content },
      });
    }
  }

  disconnectedCallback(): void {
    super.disconnectedCallback();
    this._editorView?.destroy();
    this._editorView = undefined;
  }

  private _createEditor(): void {
    if (!this._editorContainer) return;
    this._lastContent = this.content;
    const state = EditorState.create({
      doc: this.content,
      extensions: [
        basicSetup,
        yaml(),
        oneDark,
        EditorState.readOnly.of(true),
        EditorView.theme({
          '&': { height: '100%', fontSize: '13px' },
          '.cm-scroller': { overflow: 'auto' },
          '.cm-gutters': { background: '#1e1e1e', border: 'none' },
        }),
      ],
    });
    this._editorView = new EditorView({ state, parent: this._editorContainer });
  }

  private _handleClose(): void {
    this.dispatchEvent(new CustomEvent('close'));
  }

  private _handleOpenInEditor(): void {
    this.dispatchEvent(new CustomEvent('open-in-editor'));
  }

  render() {
    return html`
      <div class="header">
        <span class="path">${this.path}</span>
        <div class="actions">
          <button @click=${this._handleOpenInEditor}>Open in Editor</button>
          <button @click=${this._handleClose}>Close</button>
        </div>
      </div>
      <div class="editor"></div>
    `;
  }
}
