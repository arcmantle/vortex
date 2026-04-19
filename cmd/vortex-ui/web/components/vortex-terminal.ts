import { LitElement, html, css, unsafeCSS, PropertyValues } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebLinksAddon } from '@xterm/addon-web-links';
import xtermCss from '@xterm/xterm/css/xterm.css?inline';

export interface TerminalInfo {
	id: string;
	label: string;
	command: string;
	group: string;
	needs: string[];
	status: 'pending' | 'running' | 'success' | 'failure' | 'skipped';
}

export interface LineDTO {
	t: number;
	data: string;
}

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
				background: #1e1e1e;
			}
			.term-wrap {
				min-height: 0;
				overflow: hidden;
			}
		`,
	];

	@property({ type: Object }) terminal!: TerminalInfo;
	@property({ type: String }) token = '';

	private _term?: Terminal;
	private _fitAddon?: FitAddon;
	private _sse?: EventSource;
	private _ro?: ResizeObserver;
	private _inputDisposable?: { dispose(): void; };
	private _lastReportedSize = '';
	private _sseErrorShown = false;

	protected firstUpdated(): void {
		const wrap = this.shadowRoot!.querySelector('.term-wrap') as HTMLElement;

		this._fitAddon = new FitAddon();
		this._term = new Terminal({
			theme: { background: '#1e1e1e', foreground: '#d4d4d4' },
			convertEol: false,
			scrollback: 10_000,
			fontFamily: 'Consolas, "Courier New", monospace',
			fontSize: 13,
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
		this._fitAddon.fit();
		void this._reportSize();

		// Re-fit whenever the host element is resized.
		this._ro = new ResizeObserver(() => {
			this._fitAddon?.fit();
			void this._reportSize();
		});
		this._ro.observe(this);

		this._connectSSE();
	}

	protected updated(changed: PropertyValues): void {
		if (changed.has('terminal')) {
			const prev = changed.get('terminal') as TerminalInfo | undefined;
			if (prev?.id !== this.terminal?.id) {
				this._term?.reset();
				this._lastReportedSize = '';
				this._sseErrorShown = false;
				this._disconnectSSE();
				this._connectSSE();
				void this._reportSize();
			}
		}
	}

	disconnectedCallback(): void {
		super.disconnectedCallback();
		this._ro?.disconnect();
		this._disconnectSSE();
		this._inputDisposable?.dispose();
		this._inputDisposable = undefined;
		this._term?.dispose();
	}

	private _connectSSE(): void {
		if (!this.terminal) return;
		const params = new URLSearchParams({ id: this.terminal.id });
		if (this.token) params.set('token', this.token);
		const url = `${API_BASE}/events?${params.toString()}`;
		this._sse = new EventSource(url);
		this._sse.onopen = () => {
			// Reset the terminal on (re)connection so that the full server-side
			// replay does not duplicate already-displayed output.
			this._term?.reset();
			this._sseErrorShown = false;
		};
		this._sse.onmessage = (e) => {
			try {
				const chunk = JSON.parse(e.data) as LineDTO;
				this._term?.write(decodeBase64(chunk.data));
			} catch { /* ignore corrupt frame */ }
			this._sseErrorShown = false;
		};
		// The server keeps the SSE stream open across process restarts.
		// "exit" means the process exited, but the stream stays alive —
		// new output will arrive when the job restarts.
		this._sse.addEventListener('exit', () => {
			// Server already appended [process exited] to the buffer.
		});
		this._sse.addEventListener('skipped', () => {
			this._term?.write('\r\n\x1b[33m[job was skipped]\x1b[0m\r\n');
		});
		this._sse.addEventListener('failure', () => {
			this._term?.write('\r\n\x1b[31m[job failed to start]\x1b[0m\r\n');
		});
		this._sse.onerror = () => {
			if (!this._sseErrorShown) {
				this._sseErrorShown = true;
				this._term?.write('\r\n\x1b[31m[connection lost]\x1b[0m\r\n');
			}
		};
	}

	private _disconnectSSE(): void {
		this._sse?.close();
		this._sse = undefined;
	}

	private _authHeaders(): HeadersInit {
		const headers: HeadersInit = {};
		if (this.token) headers['Authorization'] = `Bearer ${this.token}`;
		return headers;
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

	render() {
		return html`<div class="term-wrap"></div>`;
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
