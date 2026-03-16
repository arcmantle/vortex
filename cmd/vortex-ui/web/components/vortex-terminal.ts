import { LitElement, html, css, unsafeCSS, PropertyValues } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
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
	text: string;
}

// Always use relative URLs so Vite's proxy (in dev) or the embedded server (in prod)
// handles routing. Never hardcode the Go server address from the browser.
const API_BASE = '';

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

	private _term?: Terminal;
	private _fitAddon?: FitAddon;
	private _sse?: EventSource;
	private _ro?: ResizeObserver;

	protected firstUpdated(): void {
		const wrap = this.shadowRoot!.querySelector('.term-wrap') as HTMLElement;

		this._fitAddon = new FitAddon();
		this._term = new Terminal({
			theme: { background: '#1e1e1e', foreground: '#d4d4d4' },
			convertEol: true,
			scrollback: 10_000, // matches maxBufferedLines in internal/process/process.go
			fontFamily: 'Consolas, "Courier New", monospace',
			fontSize: 13,
		});
		this._term.loadAddon(this._fitAddon);
		this._term.open(wrap);
		this._fitAddon.fit();

		// Re-fit whenever the host element is resized.
		this._ro = new ResizeObserver(() => this._fitAddon?.fit());
		this._ro.observe(this);

		this._connectSSE();
	}

	protected updated(changed: PropertyValues): void {
		if (changed.has('terminal')) {
			const prev = changed.get('terminal') as TerminalInfo | undefined;
			if (prev?.id !== this.terminal?.id) {
				this._term?.clear();
				this._disconnectSSE();
				this._connectSSE();
			}
		}
	}

	disconnectedCallback(): void {
		super.disconnectedCallback();
		this._ro?.disconnect();
		this._disconnectSSE();
		this._term?.dispose();
	}

	private _connectSSE(): void {
		if (!this.terminal) return;
		const url = `${API_BASE}/events?id=${encodeURIComponent(this.terminal.id)}`;
		this._sse = new EventSource(url);
		this._sse.onmessage = (e) => {
			const line = JSON.parse(e.data) as LineDTO;
			this._term?.writeln(line.text);
		};
		// The server keeps the SSE stream open across process restarts.
		// "exit" means the process exited, but the stream stays alive —
		// new output will arrive when the job restarts.
		this._sse.addEventListener('exit', () => {
			// Server already appended [process exited] to the buffer.
		});
		this._sse.addEventListener('skipped', () => {
			this._term?.writeln('\x1b[33m[job was skipped]\x1b[0m');
		});
		this._sse.addEventListener('failure', () => {
			this._term?.writeln('\x1b[31m[job failed to start]\x1b[0m');
		});
		this._sse.onerror = () => {
			this._term?.writeln('\r\n\x1b[31m[connection lost]\x1b[0m');
		};
	}

	private _disconnectSSE(): void {
		this._sse?.close();
		this._sse = undefined;
	}

	/** Clear the terminal display and the server-side buffer. */
	async clearOutput(): Promise<void> {
		this._term?.clear();
		this._term?.reset();
		if (this.terminal) {
			await fetch(`${API_BASE}/api/terminals/${encodeURIComponent(this.terminal.id)}/buffer`, { method: 'DELETE' });
		}
	}

	/** Close the SSE stream (for tab close). */
	closeStream(): void {
		this._disconnectSSE();
	}

	render() {
		return html`<div class="term-wrap"></div>`;
	}
}
