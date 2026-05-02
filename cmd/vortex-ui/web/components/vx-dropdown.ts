import { LitElement, html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';

/**
 * A dropdown popup that anchors to a trigger element and auto-flips
 * horizontally to stay within the viewport. Uses position:fixed to
 * escape any overflow-clipping ancestors.
 *
 * Usage:
 *   <vx-dropdown .open=${bool} @close=${handler}>
 *     <slot></slot>
 *   </vx-dropdown>
 *
 * Place this element as a sibling or child near the trigger button.
 * It will anchor itself to its offsetParent (the nearest positioned ancestor).
 */
@customElement('vx-dropdown')
export class VxDropdown extends LitElement {
  static styles = css`
    :host {
      display: contents;
    }
    .backdrop {
      position: fixed;
      inset: 0;
      z-index: 999;
    }
    .popup {
      position: fixed;
      z-index: 1000;
      min-width: 180px;
      background: var(--vx-surface-2);
      border: 1px solid var(--vx-border-strong);
      border-radius: 6px;
      box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
      padding: 4px 0;
      opacity: 0;
      pointer-events: none;
      transition: opacity 0.1s;
    }
    .popup.visible {
      opacity: 1;
      pointer-events: auto;
    }
  `;

  @property({ type: Boolean }) open = false;

  @state() private _top = 0;
  @state() private _left = 0;
  @state() private _visible = false;

  private _resizeObserver?: ResizeObserver;
  private _frameId = 0;

  override connectedCallback() {
    super.connectedCallback();
    this._resizeObserver = new ResizeObserver(() => this._reposition());
    window.addEventListener('resize', this._onResize);
    window.addEventListener('scroll', this._onResize, true);
  }

  override disconnectedCallback() {
    super.disconnectedCallback();
    this._resizeObserver?.disconnect();
    window.removeEventListener('resize', this._onResize);
    window.removeEventListener('scroll', this._onResize, true);
    cancelAnimationFrame(this._frameId);
  }

  private _onResize = () => {
    if (this.open) this._reposition();
  };

  override willUpdate(changed: Map<string, unknown>) {
    if (changed.has('open') && !this.open) {
      this._visible = false;
    }
  }

  override updated(changed: Map<string, unknown>) {
    if (changed.has('open') && this.open) {
      this.updateComplete.then(() => this._reposition());
    }
  }

  private _reposition() {
    cancelAnimationFrame(this._frameId);
    this._frameId = requestAnimationFrame(() => {
      // Anchor to the host's parent element (the trigger container)
      const anchor = this.parentElement;
      if (!anchor) return;
      const anchorRect = anchor.getBoundingClientRect();
      const popup = this.shadowRoot?.querySelector('.popup') as HTMLElement | null;
      if (!popup) return;

      // Measure popup size (it's rendered but may be at 0,0 with opacity 0)
      const popupWidth = popup.offsetWidth;
      const popupHeight = popup.offsetHeight;

      // Position below the anchor
      let top = anchorRect.bottom + 4;
      let left = anchorRect.left;

      // Flip horizontally if overflowing right
      if (left + popupWidth > window.innerWidth - 8) {
        left = anchorRect.right - popupWidth;
      }
      // Clamp left to not go off-screen left
      if (left < 8) left = 8;

      // Flip vertically if overflowing bottom
      if (top + popupHeight > window.innerHeight - 8) {
        top = anchorRect.top - popupHeight - 4;
      }

      this._top = top;
      this._left = left;
      this._visible = true;

      // Observe the anchor for layout shifts
      this._resizeObserver?.disconnect();
      this._resizeObserver?.observe(anchor);
    });
  }

  private _onBackdropClick() {
    this.dispatchEvent(new Event('close'));
  }

  render() {
    if (!this.open) return html``;
    return html`
      <div class="backdrop" @click=${this._onBackdropClick}></div>
      <div
        class="popup ${this._visible ? 'visible' : ''}"
        style="top: ${this._top}px; left: ${this._left}px;"
      >
        <slot></slot>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'vx-dropdown': VxDropdown;
  }
}
