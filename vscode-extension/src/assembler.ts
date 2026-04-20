import { parseDocument, isMap, isScalar, isSeq } from 'yaml';

/**
 * Maps each line in the assembled source back to its origin in the .vortex file.
 * `vortexLine` is the 0-based line in the .vortex document.
 * `col` is the indent offset to add when mapping columns.
 * `kind` describes what this line represents.
 */
export interface SourceMapEntry {
  vortexLine: number;
  col: number;
  kind: 'import' | 'function' | 'command' | 'synthetic';
}

export interface AssembledSource {
  /** The full assembled JavaScript/TypeScript source text. */
  text: string;
  /** Line-by-line source map: assembledLine → vortex origin. */
  sourceMap: SourceMapEntry[];
  /** The language (for now, always 'javascript'). */
  languageId: string;
}

/**
 * Assemble a complete JavaScript source from a .vortex file's embedded code.
 * This produces a file that a TS language server can analyze, providing
 * intellisense, hover, and go-to-definition for embedded regions.
 */
export function assembleSource(vortexSource: string): AssembledSource | null {
  let doc;
  try {
    doc = parseDocument(vortexSource, { keepSourceTokens: true });
  } catch {
    return null;
  }

  const root = doc.contents;
  if (!isMap(root)) return null;

  // Find the first JS runtime block (node/bun/deno)
  const jsRuntimes = ['node', 'bun', 'deno'];
  let runtimeNode = null;
  for (const rt of jsRuntimes) {
    const n = root.get(rt, true);
    if (isMap(n)) {
      runtimeNode = n;
      break;
    }
  }

  if (!runtimeNode) return null;

  const lines: string[] = [];
  const sourceMap: SourceMapEntry[] = [];
  const vortexLines = vortexSource.split('\n');

  // Helper: find the 0-based line number of a character offset
  const lineOffsets = computeLineOffsets(vortexSource);
  function offsetToLine(offset: number): number {
    let low = 0, high = lineOffsets.length - 1;
    while (low < high) {
      const mid = (low + high + 1) >> 1;
      if (lineOffsets[mid] <= offset) low = mid;
      else high = mid - 1;
    }
    return low;
  }

  // 1) Emit import statements
  const importsNode = runtimeNode.get('imports', true);
  if (isSeq(importsNode)) {
    for (const item of importsNode.items) {
      if (!isMap(item)) continue;

      const fromScalar = item.get('from', true);
      const from = isScalar(fromScalar) ? String(fromScalar.value) : '';
      if (!from) continue;

      const defaultScalar = item.get('default', true);
      const namesNode = item.get('names', true);

      let importLine = '';
      if (isScalar(defaultScalar)) {
        importLine = `import ${defaultScalar.value} from '${from}';`;
      } else if (isSeq(namesNode)) {
        const names = namesNode.items
          .filter(n => isScalar(n))
          .map(n => String((n as any).value));
        importLine = `import { ${names.join(', ')} } from '${from}';`;
      }

      if (importLine) {
        // Find the vortex line for this import entry
        const entryLine = item.range ? offsetToLine(item.range[0]) : -1;
        lines.push(importLine);
        sourceMap.push({ vortexLine: entryLine, col: 0, kind: 'import' });
      }
    }
  }

  // 2) Emit vars as const declarations
  const varsNode = runtimeNode.get('vars', true);
  if (isMap(varsNode)) {
    lines.push('');
    sourceMap.push({ vortexLine: -1, col: 0, kind: 'synthetic' });

    for (const pair of varsNode.items) {
      const keyNode = isScalar(pair.key) ? pair.key : null;
      const key = keyNode ? String(keyNode.value) : '';
      const val = isScalar(pair.value) ? pair.value.value : '';
      if (!key) continue;

      const varLine = keyNode?.range ? offsetToLine(keyNode.range[0]) : -1;
      const valStr = typeof val === 'string' ? `"${val}"` : String(val);
      lines.push(`const ${key} = ${valStr};`);
      sourceMap.push({ vortexLine: varLine, col: 0, kind: 'synthetic' });
    }
  }

  // 3) Emit functions
  const functionsNode = runtimeNode.get('functions', true);
  if (isMap(functionsNode)) {
    lines.push('');
    sourceMap.push({ vortexLine: -1, col: 0, kind: 'synthetic' });

    for (const pair of functionsNode.items) {
      const val = pair.value;
      if (!isScalar(val) || typeof val.value !== 'string') continue;

      const fnText = val.value as string;
      const fnLines = fnText.split('\n');

      // Find where this function starts in the vortex file
      let startLine = -1;
      let indent = 0;
      if (val.range) {
        const nodeStart = val.range[0];
        // Find first non-empty line of content
        const firstNonEmpty = fnLines.find(l => l.trim().length > 0) || fnLines[0];
        const contentStart = vortexSource.indexOf(firstNonEmpty, nodeStart);
        if (contentStart >= 0) {
          startLine = offsetToLine(contentStart);
          indent = contentStart - lineOffsets[startLine];
        }
      }

      for (let i = 0; i < fnLines.length; i++) {
        lines.push(fnLines[i]);
        sourceMap.push({
          vortexLine: startLine >= 0 ? startLine + i : -1,
          col: indent,
          kind: 'function',
        });
      }
    }
  }

  // 4) Emit job commands as top-level statements (wrapped in async IIFE for await support)
  const jobsNode = root.get('jobs', true);
  if (isSeq(jobsNode)) {
    for (const jobNode of jobsNode.items) {
      if (!isMap(jobNode)) continue;

      const shellScalar = jobNode.get('shell', true);
      const shell = isScalar(shellScalar) ? String(shellScalar.value).toLowerCase().trim() : '';
      if (!jsRuntimes.includes(shell)) continue;

      const commandNode = jobNode.get('command', true);
      if (!isScalar(commandNode) || typeof commandNode.value !== 'string') continue;

      const cmdText = commandNode.value as string;
      const cmdLines = cmdText.split('\n');

      // Find position in vortex file
      let startLine = -1;
      let indent = 0;
      if (commandNode.range) {
        const nodeStart = commandNode.range[0];
        const firstNonEmpty = cmdLines.find(l => l.trim().length > 0) || cmdLines[0];
        const contentStart = vortexSource.indexOf(firstNonEmpty, nodeStart);
        if (contentStart >= 0) {
          startLine = offsetToLine(contentStart);
          indent = contentStart - lineOffsets[startLine];
        }
      }

      // Separator
      lines.push('');
      sourceMap.push({ vortexLine: -1, col: 0, kind: 'synthetic' });

      // Emit command lines directly (they're top-level statements)
      for (let i = 0; i < cmdLines.length; i++) {
        lines.push(cmdLines[i]);
        sourceMap.push({
          vortexLine: startLine >= 0 ? startLine + i : -1,
          col: indent,
          kind: 'command',
        });
      }
    }
  }

  if (lines.length === 0) return null;

  return {
    text: lines.join('\n'),
    sourceMap,
    languageId: 'javascript',
  };
}

function computeLineOffsets(source: string): number[] {
  const offsets: number[] = [0];
  for (let i = 0; i < source.length; i++) {
    if (source[i] === '\n') offsets.push(i + 1);
  }
  return offsets;
}
