import { html, LitElement, type TemplateResult } from 'lit';
import { unsafeHTML } from 'lit/directives/unsafe-html.js';

import { AnsiRenderer, type ParsedLine } from './ansi-renderer';
import { forgeTerminalStyles } from './forge-terminal-styles';

export class ForgeTerminal extends LitElement {

	static readonly LINE_HEIGHT = 20.8;
	static readonly OVERSCAN = 10;
	static readonly BOTTOM_SCROLL_BUFFER_LINES = 2;

	protected lines:             ParsedLine[] = [];
	protected ansiRenderer = new AnsiRenderer();
	rawBuffer = '';
	protected scrollLocked = false;
	protected rafPending = false;
	protected lastRenderedRange: { start: number; end: number; total: number; } | null = null;
	protected lineHeight = ForgeTerminal.LINE_HEIGHT;

	protected boundOnScroll: (() => void) | null = null;
	protected totalHeight = 0;
	protected offsetY = 0;
	protected visibleStart = 0;
	protected visibleLines:  ParsedLine[] = [];

	protected override render(): TemplateResult {
		return html`
		<div>
			<div
				class="runner-terminal-spacer"
				style=${ 'position:relative;overflow:hidden;height:' + this.totalHeight + 'px' }
			>
				<div
					class="runner-terminal-viewport"
					style=${ 'position:absolute;top:0;left:0;right:0;transform:translateY(' + this.offsetY + 'px)' }
				>
					${ unsafeHTML(this.visibleLinesHtml()) }
				</div>
			</div>
		</div>
		`;
	}

	protected visibleLinesHtml(): string {
		let out = '';
		for (const line of this.visibleLines)
			out += '<div class="runner-terminal-line">' + this.lineHtml(line) + '</div>';

		return out;
	}

	protected lineHtml(line: ParsedLine): string {
		if (!line.html)
			return '&nbsp;';

		return line.html;
	}

	override connectedCallback(): void {
		super.connectedCallback();

		this.boundOnScroll = this.handleScroll.bind(this);
		this.addEventListener('scroll', this.boundOnScroll);
		this.renderVirtual();
	}

	override disconnectedCallback(): void {
		if (this.boundOnScroll)
			this.removeEventListener('scroll', this.boundOnScroll);

		super.disconnectedCallback();
	}

	clear(): void {
		this.lines = [];
		this.ansiRenderer.reset();
		this.rawBuffer = '';
		this.scrollLocked = false;
		this.rafPending = false;
		this.lastRenderedRange = null;
		this.totalHeight = 0;
		this.offsetY = 0;
		this.visibleStart = 0;
		this.visibleLines = [];
		this.requestUpdate();
	}

	appendChunk(text: string): void {
		this.rawBuffer += text;
		const parsed = this.ansiRenderer.consume(text);

		if (parsed.hadPreviousTrailing)
			this.lines.pop();

		this.lines.push(...parsed.lines);
		if (parsed.trailingLine)
			this.lines.push(parsed.trailingLine);

		this.scheduleRender();
	}

	appendError(text: string): void {
		this.rawBuffer = text;
		const parsed = this.ansiRenderer.consume(text);
		if (parsed.hadPreviousTrailing)
			this.lines.pop();

		this.lines.push(...parsed.lines);
		if (parsed.trailingLine)
			this.lines.push(parsed.trailingLine);

		this.scheduleRender();
	}

	trimExitMarker(): void {
		while (this.lines.length > 0) {
			const last = this.lines[this.lines.length - 1];
			if (!last)
				break;

			if (last.html === '' || last.html === '&nbsp;' || last.html.includes('exit:'))
				this.lines.pop();
			else
				break;
		}

		this.scheduleRender();
	}

	protected scheduleRender(): void {
		if (this.rafPending)
			return;

		this.rafPending = true;
		requestAnimationFrame(() => {
			this.rafPending = false;
			this.renderVirtual();
		});
	}

	protected renderVirtual(): void {
		const lineHeight = this.lineHeight;
		const totalLines = this.lines.length;
		const scrollBuffer = Math.ceil(lineHeight * ForgeTerminal.BOTTOM_SCROLL_BUFFER_LINES);
		const totalHeight = Math.ceil(totalLines * lineHeight) + scrollBuffer;
		const viewportHeight = this.clientHeight;
		this.totalHeight = totalHeight;

		// column-reverse: scrollTop=0 is the bottom (latest content).
		// Lock means keep scrollTop=0 so newest lines are always visible.
		if (this.scrollLocked)
			this.scrollTop = 0;

		const scrollTop = this.scrollTop;

		// Which part of the spacer (top-down coords) is currently visible?
		// visibleBottom = distance from spacer-top to the bottom edge of the viewport.
		// visibleTop    = distance from spacer-top to the top edge of the viewport.
		const visibleBottom = totalHeight - scrollTop;
		const visibleTop = visibleBottom - viewportHeight;

		const firstVisible = Math.max(0, Math.floor(visibleTop / lineHeight));
		const lastVisible = Math.ceil(visibleBottom / lineHeight);
		const start = Math.max(0, firstVisible - ForgeTerminal.OVERSCAN);
		const end = Math.min(totalLines, lastVisible + ForgeTerminal.OVERSCAN);

		if (this.lastRenderedRange
			&& this.lastRenderedRange.start === start
			&& this.lastRenderedRange.end === end
			&& this.lastRenderedRange.total === totalLines)
			return;

		this.lastRenderedRange = { start, end, total: totalLines };
		this.offsetY = Math.round(start * lineHeight);
		this.visibleStart = start;
		this.visibleLines = this.lines.slice(start, end);
		this.requestUpdate();
	}

	scrollToBottom(): void {
		this.scrollLocked = true;
		this.lastRenderedRange = null;
		this.scrollTop = 0;
		this.scheduleRender();
	}

	protected handleScroll(): void {
		// column-reverse: atBottom means scrollTop is near 0.
		const atBottom = this.scrollTop <= this.lineHeight * 2;
		this.scrollLocked = atBottom;
		if (!this.rafPending)
			this.scheduleRender();
	}

	static override styles = [forgeTerminalStyles];

}

customElements.define('forge-terminal', ForgeTerminal);
