import { html, type TemplateResult } from 'lit';

/**
 * Returns an inline SVG icon for a shell profile icon name.
 * Falls back to a generic terminal icon if the name is unknown.
 */
export function shellIcon(name?: string): TemplateResult {
  switch (name) {
    case 'zsh':
    case 'bash':
    case 'sh':
      return html`<svg class="shell-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3"><path d="M4 12l4-4-4-4"/><line x1="9" y1="12" x2="13" y2="12"/></svg>`;
    case 'fish':
      return html`<svg class="shell-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3"><path d="M2 8c2-4 6-4 9-2s3 6 1 7-4-1-6-1-4 0-4-4z"/><circle cx="11" cy="7" r="0.8" fill="currentColor" stroke="none"/></svg>`;
    case 'powershell':
    case 'pwsh':
      return html`<svg class="shell-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3"><path d="M3 12l5-4-5-4"/><line x1="9" y1="12" x2="14" y2="12"/><rect x="1" y="2" width="14" height="12" rx="2"/></svg>`;
    case 'cmd':
      return html`<svg class="shell-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3"><rect x="1.5" y="2.5" width="13" height="11" rx="1.5"/><path d="M4 10h4"/><path d="M4 7l2.5 1.5L4 10"/></svg>`;
    case 'wsl':
      return html`<svg class="shell-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3"><path d="M3 13c1-3 3-5 5-5s4 2 5 5"/><circle cx="8" cy="5" r="3"/></svg>`;
    case 'terminal':
    default:
      return html`<svg class="shell-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3"><rect x="1.5" y="2.5" width="13" height="11" rx="1.5"/><path d="M4.5 6l2.5 2-2.5 2"/><line x1="8.5" y1="10" x2="11.5" y2="10"/></svg>`;
  }
}
