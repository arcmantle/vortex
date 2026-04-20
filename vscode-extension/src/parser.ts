import { parseDocument, isMap, isScalar, isSeq, type Document, type YAMLMap, type Scalar } from 'yaml';

/** An embedded code region within a .vortex file. */
export interface EmbeddedRegion {
  /** VS Code language ID for this region */
  languageId: string;
  /** 0-based start line in the document */
  startLine: number;
  /** 0-based end line (inclusive) in the document */
  endLine: number;
  /** Column offset for all lines (block scalar indent) */
  indent: number;
  /** The embedded code text */
  text: string;
  /** Context: 'command' | 'function' */
  kind: 'command' | 'function';
}

/** Maps vortex shell names to VS Code language IDs */
const SHELL_TO_LANGUAGE: Record<string, string> = {
  node: 'javascript',
  bun: 'javascript',
  deno: 'javascript',
  csharp: 'csharp',
  go: 'go',
};

/** Maps vortex runtime block names to VS Code language IDs */
const RUNTIME_BLOCKS: Record<string, string> = {
  node: 'javascript',
  bun: 'javascript',
  deno: 'javascript',
  csharp: 'csharp',
  go: 'go',
};

/**
 * Parse a .vortex document and extract all embedded code regions.
 * Uses the yaml library's AST to get exact source positions.
 */
export function extractEmbeddedRegions(source: string): EmbeddedRegion[] {
  const regions: EmbeddedRegion[] = [];
  const lineOffsets = computeLineOffsets(source);

  let doc: Document;
  try {
    doc = parseDocument(source, { keepSourceTokens: true });
  } catch {
    return regions;
  }

  const root = doc.contents;
  if (!isMap(root)) {
    return regions;
  }

  // 1) Extract from runtime blocks (top-level node/bun/deno/csharp/go)
  for (const [blockName, languageId] of Object.entries(RUNTIME_BLOCKS)) {
    const runtimeNode = root.get(blockName, true);
    if (!isMap(runtimeNode)) continue;

    // functions: { name: "code..." }
    const functionsNode = runtimeNode.get('functions', true);
    if (isMap(functionsNode)) {
      for (const pair of functionsNode.items) {
        const val = pair.value;
        if (isScalar(val) && typeof val.value === 'string' && val.range) {
          const region = scalarToRegion(val, source, lineOffsets, languageId, 'function');
          if (region) regions.push(region);
        }
      }
    }
  }

  // 2) Extract from jobs
  const jobsNode = root.get('jobs', true);
  if (!isSeq(jobsNode)) return regions;

  for (const jobNode of jobsNode.items) {
    if (!isMap(jobNode)) continue;

    // Determine language from shell field
    const shellScalar = jobNode.get('shell', true);
    const shellValue = isScalar(shellScalar) ? String(shellScalar.value) : '';
    const languageId = SHELL_TO_LANGUAGE[shellValue.toLowerCase().trim()];
    if (!languageId) continue;

    // command: |
    const commandNode = jobNode.get('command', true);
    if (isScalar(commandNode) && typeof commandNode.value === 'string' && commandNode.range) {
      const region = scalarToRegion(commandNode, source, lineOffsets, languageId, 'command');
      if (region) regions.push(region);
    }
  }

  return regions;
}

/**
 * Convert a YAML scalar node (block or flow) to an EmbeddedRegion.
 * The yaml library gives us range [start, valueEnd, nodeEnd] offsets.
 */
function scalarToRegion(
  node: Scalar,
  source: string,
  lineOffsets: number[],
  languageId: string,
  kind: 'command' | 'function'
): EmbeddedRegion | null {
  const text = node.value as string;
  if (!text.trim()) return null;

  const range = node.range;
  if (!range) return null;

  // For block scalars (| or >), the content starts after the indicator line.
  // The range[0] points to the block indicator, range[1] to value end.
  // We need to find where the actual content begins.
  const nodeStart = range[0];
  const nodeValueEnd = range[1];

  // Find the content in the source — search for the first line of text after the indicator
  const lines = text.split('\n');
  if (lines.length === 0) return null;

  // Find the first non-empty line to locate it in source
  const firstNonEmpty = lines.find(l => l.trim().length > 0) || lines[0];
  const contentStart = source.indexOf(firstNonEmpty, nodeStart);
  if (contentStart < 0) return null;

  const contentEnd = nodeValueEnd;
  const startLine = offsetToLine(contentStart, lineOffsets);
  let endLine = offsetToLine(contentEnd - 1, lineOffsets);

  // Trim trailing empty lines
  while (endLine > startLine) {
    const lineStart = lineOffsets[endLine];
    const lineEnd = endLine + 1 < lineOffsets.length ? lineOffsets[endLine + 1] : source.length;
    const lineText = source.substring(lineStart, lineEnd);
    if (lineText.trim().length > 0) break;
    endLine--;
  }

  // Calculate indent — the column where the block content starts
  const indent = contentStart - lineOffsets[startLine];

  return {
    languageId,
    startLine,
    endLine,
    indent,
    text,
    kind,
  };
}

function computeLineOffsets(source: string): number[] {
  const offsets: number[] = [0];
  for (let i = 0; i < source.length; i++) {
    if (source[i] === '\n') {
      offsets.push(i + 1);
    }
  }
  return offsets;
}

function offsetToLine(offset: number, lineOffsets: number[]): number {
  let low = 0;
  let high = lineOffsets.length - 1;
  while (low < high) {
    const mid = (low + high + 1) >> 1;
    if (lineOffsets[mid] <= offset) {
      low = mid;
    } else {
      high = mid - 1;
    }
  }
  return low;
}
