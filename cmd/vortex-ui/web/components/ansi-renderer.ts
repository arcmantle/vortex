export interface ParsedLine {
	html: string;
}

interface AnsiState {
	bold:      boolean;
	dim:       boolean;
	italic:    boolean;
	underline: boolean;
	fg:        string | null;
	bg:        string | null;
}

interface AnsiParseResult {
	lines:        ParsedLine[];
	trailingHtml: string;
	trailingRaw:  string;
}

interface AnsiConsumeResult {
	lines:               ParsedLine[];
	trailingLine:        ParsedLine | null;
	hadPreviousTrailing: boolean;
}

const ANSI_COLORS: Record<string, string> = {
	'30': '#555',
	'31': '#f85149',
	'32': '#56d364',
	'33': '#d29922',
	'34': '#58a6ff',
	'35': '#bc8cff',
	'36': '#39d2c0',
	'37': '#cccccc',
	'90': '#6e7681',
	'91': '#ff7b72',
	'92': '#7ee787',
	'93': '#e3b341',
	'94': '#79c0ff',
	'95': '#d2a8ff',
	'96': '#56d4dd',
	'97': '#ffffff',
};

const ANSI_BG_COLORS: Record<string, string> = {
	'40':  '#555',
	'41':  '#f85149',
	'42':  '#56d364',
	'43':  '#d29922',
	'44':  '#58a6ff',
	'45':  '#bc8cff',
	'46':  '#39d2c0',
	'47':  '#cccccc',
	'100': '#6e7681',
	'101': '#ff7b72',
	'102': '#7ee787',
	'103': '#e3b341',
	'104': '#79c0ff',
	'105': '#d2a8ff',
	'106': '#56d4dd',
	'107': '#ffffff',
};

export class AnsiRenderer {

	protected state: AnsiState = { bold: false, dim: false, italic: false, underline: false, fg: null, bg: null };
	protected trailingChunk = '';

	reset(): void {
		this.state = { bold: false, dim: false, italic: false, underline: false, fg: null, bg: null };
		this.trailingChunk = '';
	}

	consume(text: string): AnsiConsumeResult {
		const chunk = (this.trailingChunk || '') + text;
		const parsed = this.parseChunk(chunk);
		const hadPreviousTrailing = this.trailingChunk !== '';

		this.trailingChunk = parsed.trailingRaw;

		return {
			lines:        parsed.lines,
			trailingLine: parsed.trailingHtml ? { html: parsed.trailingHtml } : null,
			hadPreviousTrailing,
		};
	}

	protected parseChunk(text: string): AnsiParseResult {
		const lines: ParsedLine[] = [];
		let result = '';
		let rawCurrent = '';
		let spanOpen = false;
		let { bold, dim, italic, underline, fg, bg } = this.state;

		const buildSpan = (): void => {
			if (spanOpen)
				result += '</span>';

			const styles: string[] = [];
			if (fg)
				styles.push('color:' + fg);

			if (bg)
				styles.push('background:' + bg);

			if (bold)
				styles.push('font-weight:bold');

			if (dim)
				styles.push('opacity:0.6');

			if (italic)
				styles.push('font-style:italic');

			if (underline)
				styles.push('text-decoration:underline');

			if (styles.length > 0) {
				result += '<span style="' + styles.join(';') + '">';
				spanOpen = true;
			}
			else {
				spanOpen = false;
			}
		};

		let index = 0;
		while (index < text.length) {
			if (text[index] === '\n') {
				if (spanOpen)
					result += '</span>';

				lines.push({ html: result });
				result = '';
				rawCurrent = '';
				spanOpen = false;
				if (fg || bg || bold || dim || italic || underline)
					buildSpan();

				index++;
				continue;
			}

			if (text[index] === '\r') {
				index++;
				continue;
			}

			if (text[index] === '\x1b' && text[index + 1] === '[') {
				const end = text.indexOf('m', index + 2);
				if (end === -1) {
					rawCurrent += text.slice(index);
					break;
				}

				rawCurrent += text.slice(index, end + 1);

				const seq = text.substring(index + 2, end);
				const codes = seq.split(';');
				for (const code of codes) {
					const value = code.trim();
					if (value === '0' || value === '') {
						bold = false;
						dim = false;
						italic = false;
						underline = false;
						fg = null;
						bg = null;
					}
					else if (value === '1') {
						bold = true;
					}
					else if (value === '2') {
						dim = true;
					}
					else if (value === '3') {
						italic = true;
					}
					else if (value === '4') {
						underline = true;
					}
					else if (value === '22') {
						bold = false;
						dim = false;
					}
					else if (value === '23') {
						italic = false;
					}
					else if (value === '24') {
						underline = false;
					}
					else if (value === '39') {
						fg = null;
					}
					else if (value === '49') {
						bg = null;
					}
					else if (ANSI_COLORS[value]) {
						fg = ANSI_COLORS[value];
					}
					else if (ANSI_BG_COLORS[value]) {
						bg = ANSI_BG_COLORS[value];
					}
				}

				buildSpan();
				index = end + 1;
				continue;
			}

			if (text[index] === '<')
				result += '&lt;';
			else if (text[index] === '>')
				result += '&gt;';
			else if (text[index] === '&')
				result += '&amp;';
			else
				result += text[index];

			rawCurrent += text[index];

			index++;
		}

		if (spanOpen)
			result += '</span>';

		this.state.bold = bold;
		this.state.dim = dim;
		this.state.italic = italic;
		this.state.underline = underline;
		this.state.fg = fg;
		this.state.bg = bg;

		return { lines, trailingHtml: result, trailingRaw: rawCurrent };
	}

}
