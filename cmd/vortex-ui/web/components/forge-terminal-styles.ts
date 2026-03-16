import { css } from 'lit';

export const forgeTerminalStyles = css`
	:host {
		font-family: var(--font-mono);
		font-size: 13px;
		line-height: 1.6;
		color: #cccccc;
		display: block;
		height: 400px;
		overflow: auto;
		tab-size: 4;
		position: relative;
		scrollbar-width: auto;
		scrollbar-color: #555 #1a1a1a;

		display: flex;
		flex-flow: nowrap column-reverse;
	}

	:host::-webkit-scrollbar {
		width: 12px;
	}
	:host::-webkit-scrollbar-track {
		background: #1a1a1a;
	}
	:host::-webkit-scrollbar-thumb {
		background: #555;
		border-radius: 6px;
		border: 2px solid #1a1a1a;
	}
	:host::-webkit-scrollbar-thumb:hover {
		background: #888;
	}

	.runner-terminal-spacer {
	}
	.runner-terminal-viewport {
		padding: 0 16px;
	}

	.runner-terminal-line {
		white-space: pre-wrap;
		overflow-wrap: anywhere;
		word-break: break-word;
	}
`;
