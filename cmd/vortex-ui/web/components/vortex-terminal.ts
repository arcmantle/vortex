import { LitElement, html, css, unsafeCSS, PropertyValues } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebLinksAddon } from '@xterm/addon-web-links';
import xtermCss from '@xterm/xterm/css/xterm.css?inline';
import type { TerminalInfo } from '../types.js';

// Always use relative URLs so Vite's proxy (in dev) or the embedded server (in prod)
// handles routing. Never hardcode the Go server address from the browser.
const API_BASE = '';

type NativeBrowserBridge = {
	vortexOpenExternal?: (url: string) => Promise<unknown>;
};

type FileLinkMatch = {
	text: string;
	path: string;
	startIndex: number;
	endIndex: number;
};

const FILE_PATH_PATTERN = /(?:^|[\s("'])((?:~\/|\.{1,2}[\\/]|\/|[A-Za-z]:[\\/])?(?:[^\s"'`()\[\]{}<>:]+[\\/])+[^\s"'`()\[\]{}<>:]+(?:\:\d+(?::\d+)?)?)/g;
const ASSIGNED_FILE_PATH_PATTERN = /(?:^|\s)[A-Za-z_][\w-]*=(?:"((?:~\/|\.{1,2}[\\/]|\/|[A-Za-z]:[\\/])[^"\r\n]+)"|'((?:~\/|\.{1,2}[\\/]|\/|[A-Za-z]:[\\/])[^'\r\n]+)'|((?:~\/|\.{1,2}[\\/]|\/|[A-Za-z]:[\\/])[^\r\n]+?))(?=$|\s+[A-Za-z_][\w-]*=)/g;

// ---------------------------------------------------------------------------
// vortex-terminal — xterm.js pane connected to one process via SSE
//
// Shadow DOM notes:
//   - xterm injects its stylesheet into document.head, which doesn't reach
//     inside a shadow root. We import the CSS with ?inline and adopt it via
//     unsafeCSS so it lives in this component's shadow root.
//   - FitAddon resizes the terminal to fill the container. We trigger it via
//     a ResizeObserver so it responds to layout changes automatically.
// ---------------------------------------------------------------------------

@customElement('vortex-terminal')
export class VortexTerminal extends LitElement {
	static styles = [
		unsafeCSS(xtermCss),
		css`
			:host {
				display: grid;
				grid-template-rows: 1fr;
				min-height: 0;
				overflow: hidden;
				background: var(--vx-surface-0);
				position: relative;
			}
			.term-wrap {
				min-height: 0;
				overflow: hidden;
			}
			::-webkit-scrollbar { width: 8px; height: 8px; }
			::-webkit-scrollbar-track { background: transparent; }
			::-webkit-scrollbar-thumb { background: var(--vx-scrollbar-thumb); border-radius: 4px; }
			::-webkit-scrollbar-thumb:hover { background: var(--vx-scrollbar-thumb-hover); }
			::-webkit-scrollbar-corner { background: transparent; }

			.panel-toolbar {
				position: absolute;
				top: 8px;
				right: 12px;
				z-index: 10;
				display: inline-grid;
				grid-auto-flow: column;
				gap: 8px;
				padding: 6px;
				border: 1px solid var(--vx-border-strong);
				border-radius: 999px;
				background: var(--vx-surface-1);
				box-shadow: 0 10px 24px rgba(0, 0, 0, 0.25);
			}
			.toolbar-btn {
				width: 28px;
				height: 28px;
				border-radius: 50%;
				border: 1px solid var(--vx-border-strong);
				background: var(--vx-surface-3);
				color: var(--vx-text-secondary);
				cursor: pointer;
				display: grid;
				place-items: center;
				opacity: 0.6;
				transition: opacity 0.15s, background 0.15s;
			}
			.toolbar-btn:hover {
				opacity: 1;
				background: var(--vx-surface-5);
			}
			.toolbar-btn svg {
				width: 14px;
				height: 14px;
				fill: currentColor;
			}
		`,
	];

	@property({ type: Object }) terminal!: TerminalInfo;
	@property({ type: String }) token = '';
	@property({ type: String }) fontFamily = '';
	@property({ type: Number }) fontSize = 0;
	@property({ type: Boolean }) showToolbar = false;

	private _term?: Terminal;
	private _fitAddon?: FitAddon;
	private _sse?: EventSource;
	private _sseAbort?: AbortController;
	private _ro?: ResizeObserver;
	private _inputDisposable?: { dispose(): void; };
	private _contextMenuHandler?: (e: MouseEvent) => void;
	private _themeHandler?: () => void;
	private _lastReportedSize = '';
	private _sseErrorShown = false;
	private _sseInitial = true;
	private _replayWindow = false;
	private _replayTimer?: ReturnType<typeof setTimeout>;

	protected firstUpdated(): void {
		const wrap = this.shadowRoot!.querySelector('.term-wrap') as HTMLElement;

		this._fitAddon = new FitAddon();
		this._term = new Terminal({
			theme: this._buildXtermTheme(),
			convertEol: false,
			scrollback: 10_000,
			fontFamily: this.fontFamily || 'Consolas, "Courier New", monospace',
			fontSize: this.fontSize || 13,
			rightClickSelectsWord: true,
		});
		this._term.loadAddon(new WebLinksAddon((_event, uri) => {
			void openExternalLink(uri);
		}));
		this._term.registerLinkProvider({
			provideLinks: (bufferLineNumber, callback) => {
				const text = this._term?.buffer.active.getLine(bufferLineNumber - 1)?.translateToString(true) ?? '';
				const matches = findFilePathMatches(text);
				callback(matches.map((match) => ({
					range: {
						start: { x: match.startIndex + 1, y: bufferLineNumber },
						end: { x: match.endIndex + 1, y: bufferLineNumber },
					},
					text: match.text,
					activate: () => {
						void revealTerminalPath(match.path, this._authHeaders());
					},
				})));
			},
		});
		this._term.loadAddon(this._fitAddon);
		this._term.open(wrap);
		this._inputDisposable = this._term.onData((data) => {
			void this._sendInput(data);
		});
		// Copy selection to clipboard on Cmd/Ctrl+C (when text is selected)
		// and on right-click via the context menu.
		this._term.attachCustomKeyEventHandler((e) => {
			if (e.type === 'keydown' && e.key === 'c' && (e.metaKey || e.ctrlKey) && this._term?.hasSelection()) {
				void navigator.clipboard.writeText(this._term.getSelection());
				return false; // prevent xterm from sending ^C
			}
			return true;
		});
		this._contextMenuHandler = (e: MouseEvent) => {
			const sel = this._term?.getSelection();
			if (sel) {
				e.preventDefault();
				void navigator.clipboard.writeText(sel);
			}
		};
		wrap.addEventListener('contextmenu', this._contextMenuHandler);
		this._fitAddon.fit();
		void this._reportSize();

		// Re-fit whenever the host element is resized.
		this._ro = new ResizeObserver(() => {
			this._fitAddon?.fit();
			void this._reportSize();
		});
		this._ro.observe(this);

		// Listen for theme changes from ThemeManager
		this._themeHandler = () => this.reapplyTheme();
		this.getRootNode().addEventListener('vx-theme-changed', this._themeHandler);

		this._connectSSE();
	}

	protected updated(changed: PropertyValues): void {
		if (changed.has('terminal')) {
			const prev = changed.get('terminal') as TerminalInfo | undefined;
			// Skip when prev is undefined — that's the initial property assignment
			// which firstUpdated() already handles by connecting SSE.
			if (prev && prev.id !== this.terminal?.id) {
				this._term?.reset();
				this._lastReportedSize = '';
				this._sseErrorShown = false;
				this._disconnectSSE();
				this._connectSSE();
				void this._reportSize();
			}
		}
		if (changed.has('fontFamily') || changed.has('fontSize')) {
			if (this._term) {
				if (this.fontFamily) this._term.options.fontFamily = this.fontFamily;
				if (this.fontSize) this._term.options.fontSize = this.fontSize;
				this._fitAddon?.fit();
			}
		}
	}

	connectedCallback(): void {
		super.connectedCallback();
		// Re-attach DOM-dependent observers on reattach (e.g. cache directive).
		// SSE + input stay alive while detached so no reset/replay is needed.
		if (this._term) {
			if (!this._ro) {
				this._ro = new ResizeObserver(() => {
					this._fitAddon?.fit();
					void this._reportSize();
				});
				this._ro.observe(this);
			}
			if (!this._themeHandler) {
				this._themeHandler = () => this.reapplyTheme();
				this.getRootNode().addEventListener('vx-theme-changed', this._themeHandler);
			}
			if (!this._sse && this.terminal) {
				this._connectSSE();
			}
			requestAnimationFrame(() => this._fitAddon?.fit());
		}
	}

	disconnectedCallback(): void {
		super.disconnectedCallback();
		// Only detach DOM-dependent observers. SSE + input remain alive so the
		// xterm buffer stays current for flicker-free reattach.
		this._ro?.disconnect();
		this._ro = undefined;
		if (this._themeHandler) {
			this.getRootNode().removeEventListener('vx-theme-changed', this._themeHandler);
			this._themeHandler = undefined;
		}
	}

	private _connectSSE(): void {
		if (!this.terminal) return;
		const params = new URLSearchParams({ id: this.terminal.id });
		if (this.token) params.set('token', this.token);
		const url = `${API_BASE}/events?${params.toString()}`;
		this._sseInitial = true;
		this._replayWindow = true;
		clearTimeout(this._replayTimer);
		// AbortController ensures all listeners are removed atomically on disconnect,
		// preventing stale callbacks from firing after close().
		this._sseAbort = new AbortController();
		const { signal } = this._sseAbort;
		this._sse = new EventSource(url);
		this._sse.addEventListener('open', () => {
			// On auto-reconnect (network blip) the server replays the full buffer,
			// so we must reset to avoid duplicates. On the *initial* connect the
			// terminal is already in the correct state (empty or pre-reset by
			// the updated() handler), so skip the reset to avoid a visible flash.
			if (!this._sseInitial) {
				this._term?.reset();
				this._replayWindow = true;
			}
			this._sseInitial = false;
			this._sseErrorShown = false;
			// Suppress CSI responses (CPR, DA) generated by xterm.js during replay.
			// Use a debounce: as long as messages keep arriving, we're still replaying.
			// Once messages stop for 200ms, the replay is done and live responses pass.
			clearTimeout(this._replayTimer);
			this._replayTimer = setTimeout(() => { this._replayWindow = false; }, 200);
		}, { signal });
		this._sse.addEventListener('message', (e) => {
			// Reset the replay debounce on each message — large replays can exceed
			// a fixed timeout, so we keep the window open while data flows.
			if (this._replayWindow) {
				clearTimeout(this._replayTimer);
				this._replayTimer = setTimeout(() => { this._replayWindow = false; }, 200);
			}
			try {
				const decoded = decodeBase64((JSON.parse(e.data) as { data: string }).data);
				this._batchWrite(decoded);
			} catch { /* ignore corrupt frame */ }
		}, { signal });
		// The server keeps the SSE stream open across process restarts.
		// "exit" means the process exited, but the stream stays alive —
		// new output will arrive when the job restarts.
		this._sse.addEventListener('exit', () => {
			// Server already appended [process exited] to the buffer.
		}, { signal });
		this._sse.addEventListener('skipped', () => {
			this._term?.write('\r\n\x1b[33m[job was skipped]\x1b[0m\r\n');
		}, { signal });
		this._sse.addEventListener('failure', () => {
			this._term?.write('\r\n\x1b[31m[job failed to start]\x1b[0m\r\n');
		}, { signal });
		this._sse.addEventListener('error', () => {
			if (!this._sseErrorShown) {
				this._sseErrorShown = true;
				this._term?.write('\r\n\x1b[31m[connection lost]\x1b[0m\r\n');
			}
		}, { signal });
	}

	private _disconnectSSE(): void {
		this._sseAbort?.abort();
		this._sseAbort = undefined;
		this._sse?.close();
		this._sse = undefined;
	}

	/** Write terminal output. xterm.js handles internal batching. */
	private _batchWrite(data: Uint8Array): void {
		if (data.length === 0) return;
		this._term?.write(data);
	}

	private _authHeaders(): HeadersInit {
		const headers: HeadersInit = {};
		if (this.token) headers['Authorization'] = `Bearer ${this.token}`;
		return headers;
	}

	private _buildXtermTheme(): Record<string, string> {
		const s = getComputedStyle(this);
		const v = (name: string, fallback: string) => s.getPropertyValue(name).trim() || fallback;
		return {
			background: v('--vx-surface-0', '#1e1e1e'),
			foreground: v('--vx-text-primary', '#d4d4d4'),
			cursor: v('--vx-text-primary', '#d4d4d4'),
			cursorAccent: v('--vx-surface-0', '#1e1e1e'),
			selectionBackground: v('--vx-accent-muted', '#094771'),
			scrollbarSliderBackground: v('--vx-scrollbar-thumb', '#42424280'),
			scrollbarSliderHoverBackground: v('--vx-scrollbar-thumb-hover', '#555555b3'),
			scrollbarSliderActiveBackground: v('--vx-scrollbar-thumb-active', '#666666cc'),
			black: v('--vx-ansi-black', '#1e1e1e'),
			red: v('--vx-ansi-red', '#f14c4c'),
			green: v('--vx-ansi-green', '#23d18b'),
			yellow: v('--vx-ansi-yellow', '#f5f543'),
			blue: v('--vx-ansi-blue', '#3b8eea'),
			magenta: v('--vx-ansi-magenta', '#d670d6'),
			cyan: v('--vx-ansi-cyan', '#29b8db'),
			white: v('--vx-ansi-white', '#cccccc'),
			brightBlack: v('--vx-ansi-bright-black', '#666666'),
			brightRed: v('--vx-ansi-bright-red', '#f14c4c'),
			brightGreen: v('--vx-ansi-bright-green', '#23d18b'),
			brightYellow: v('--vx-ansi-bright-yellow', '#f5f543'),
			brightBlue: v('--vx-ansi-bright-blue', '#3b8eea'),
			brightMagenta: v('--vx-ansi-bright-magenta', '#d670d6'),
			brightCyan: v('--vx-ansi-bright-cyan', '#29b8db'),
			brightWhite: v('--vx-ansi-bright-white', '#ffffff'),
		};
	}

	/** Re-apply terminal theme from current CSS variables. Call when theme changes. */
	reapplyTheme(): void {
		if (this._term) {
			this._term.options.theme = this._buildXtermTheme();
		}
	}

	/** Clear the terminal display and the server-side buffer. */
	async clearOutput(): Promise<void> {
		this._term?.clear();
		this._term?.reset();
		if (this.terminal) {
			try {
				const res = await fetch(`${API_BASE}/api/terminals/${encodeURIComponent(this.terminal.id)}/buffer`, { method: 'DELETE', headers: this._authHeaders() });
				if (!res.ok) {
					this._term?.write('\r\n\x1b[2m[clear buffer failed]\x1b[0m\r\n');
				}
			} catch {
				this._term?.write('\r\n\x1b[2m[clear buffer failed]\x1b[0m\r\n');
			}
		}
	}

	/** Close the SSE stream (for tab close). */
	closeStream(): void {
		this._disconnectSSE();
	}

	/** Write a dim status message into the terminal (e.g. for action failures). */
	writeStatus(message: string): void {
		this._term?.write(`\r\n\x1b[2m[${message}]\x1b[0m\r\n`);
	}

	private async _reportSize(): Promise<void> {
		if (!this.terminal || !this._term) return;

		const cols = this._term.cols;
		const rows = this._term.rows;
		if (!cols || !rows) return;

		const key = `${this.terminal.id}:${cols}x${rows}`;
		if (key === this._lastReportedSize) return;

		try {
			await fetch(`${API_BASE}/api/terminals/${encodeURIComponent(this.terminal.id)}/size`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json', ...this._authHeaders() },
				body: JSON.stringify({ cols, rows }),
			});
			this._lastReportedSize = key;
		} catch {
			// The terminal may not be available yet; the next resize or reconnect will retry.
		}
	}

	private async _sendInput(data: string): Promise<void> {
		if (!this.terminal || data.length === 0) return;
		// Filter out terminal query responses generated by xterm.js.
		// OSC/DCS replies are always noise (unsolicited). CSI responses (CPR, DA)
		// are only noise during the replay window — during live interaction the
		// shell actively expects them (e.g. zsh reverse-i-search uses \x1b[6n).
		if (data.charCodeAt(0) === 0x1b) {
			const c = data.charCodeAt(1);
			// OSC (\x1b]) or DCS (\x1bP) — always suppress
			if (c === 0x5d || c === 0x50) return;
			// CSI responses: DA (\x1b[?...c) or CPR (\x1b[...R) — suppress during replay only
			if (this._replayWindow && c === 0x5b && /^\x1b\[[\?0-9;]*[Rcn]$/.test(data)) return;
		}

		try {
			await fetch(`${API_BASE}/api/terminals/${encodeURIComponent(this.terminal.id)}/input`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json', ...this._authHeaders() },
				body: JSON.stringify({ data: encodeBase64(data) }),
			});
		} catch {
			// Ignore transient failures; a reconnect or rerun will restore the terminal stream.
		}
	}

	private _fireToolbarEvent(type: string): void {
		this.dispatchEvent(new CustomEvent(type, { detail: { id: this.terminal.id }, bubbles: true, composed: true }));
	}

	render() {
		return html`
			${this.showToolbar ? html`
				<div class="panel-toolbar">
					${this.terminal.status === 'running' || this.terminal.status === 'pending'
						? html`<button class="toolbar-btn" @click=${() => this._fireToolbarEvent('terminal-stop')} title="Stop process"><svg viewBox="0 0 16 16"><rect x="3" y="3" width="10" height="10" rx="1"/></svg></button>`
						: html`<button class="toolbar-btn" @click=${() => this._fireToolbarEvent('terminal-rerun')} title="Start process"><svg viewBox="0 0 16 16"><path d="M4.5 2.5v11l9-5.5z"/></svg></button>`}
					<button class="toolbar-btn" @click=${() => this._fireToolbarEvent('terminal-rerun')} title="Rerun this job and downstream dependent jobs"><svg viewBox="0 0 16 16"><path d="M13.5 2v4h-4" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><path d="M13.15 5.97A5.5 5.5 0 1 1 7.5 2.5c1.58 0 3.02.67 4.03 1.74L13.5 6" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg></button>
					<button class="toolbar-btn" @click=${() => void this.clearOutput()} title="Clear terminal"><svg viewBox="0 0 16 16"><path d="M2 2l12 12M14 2L2 14" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg></button>
				</div>
			` : ''}
			<div class="term-wrap"></div>
		`;
	}
}

function decodeBase64(data: string): Uint8Array {
	const binary = atob(data);
	const out = new Uint8Array(binary.length);
	for (let i = 0; i < binary.length; i++) {
		out[i] = binary.charCodeAt(i);
	}
	return out;
}

function encodeBase64(data: string): string {
	const bytes = new TextEncoder().encode(data);
	let binary = '';
	for (const value of bytes) {
		binary += String.fromCharCode(value);
	}
	return btoa(binary);
}

async function openExternalLink(url: string): Promise<void> {
	const nativeBridge = (globalThis as typeof globalThis & NativeBrowserBridge).vortexOpenExternal;
	if (typeof nativeBridge === 'function') {
		await nativeBridge(url);
		return;
	}

	window.open(url, '_blank', 'noopener,noreferrer');
}

async function revealTerminalPath(path: string, headers: HeadersInit = {}): Promise<void> {
	try {
		await fetch(`${API_BASE}/api/open-path`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json', ...headers },
			body: JSON.stringify({ path }),
		});
	} catch {
		// network error — ignored
	}
}

function findFilePathMatches(line: string): FileLinkMatch[] {
	const matches: FileLinkMatch[] = [];
	const assignedRegex = new RegExp(ASSIGNED_FILE_PATH_PATTERN);
	for (;;) {
		const result = assignedRegex.exec(line);
		if (!result || result.index < 0) {
			break;
		}
		const text = (result[1] ?? result[2] ?? result[3] ?? '').trimEnd();
		if (!text || text.includes('://')) {
			continue;
		}
		const startIndex = result.index + result[0].lastIndexOf(text);
		pushFilePathMatch(matches, {
			text,
			path: text,
			startIndex,
			endIndex: startIndex + text.length,
		});
	}

	const regex = new RegExp(FILE_PATH_PATTERN);
	while (true) {
		const result = regex.exec(line);
		if (!result || result.index < 0) {
			break;
		}
		const text = result[1];
		if (!text || text.includes('://')) {
			continue;
		}
		const startIndex = result.index + result[0].lastIndexOf(text);
		pushFilePathMatch(matches, {
			text,
			path: text,
			startIndex,
			endIndex: startIndex + text.length,
		});
	}

	matches.sort((left, right) => left.startIndex - right.startIndex);
	return matches;
}

function pushFilePathMatch(matches: FileLinkMatch[], candidate: FileLinkMatch): void {
	for (let index = 0; index < matches.length; index++) {
		const existing = matches[index];
		if (candidate.startIndex < existing.endIndex && candidate.endIndex > existing.startIndex) {
			const candidateLength = candidate.endIndex - candidate.startIndex;
			const existingLength = existing.endIndex - existing.startIndex;
			if (candidateLength > existingLength) {
				matches[index] = candidate;
			}
			return;
		}
	}
	matches.push(candidate);
}
