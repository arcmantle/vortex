import { parseDocument, isMap, isScalar, isSeq, type Scalar } from 'yaml';
import { SourceMap, type LineMapping } from './source-map';

export interface AssembledSource {
  /** The full assembled source text. */
  text: string;
  /** Bidirectional source map between vortex and assembled lines. */
  sourceMap: SourceMap;
  /** The language of the assembled source. */
  languageId: 'javascript' | 'go' | 'csharp';
}

const JS_RUNTIMES = ['node', 'bun', 'deno'];
const GO_RUNTIMES = ['go'];
const CSHARP_RUNTIMES = ['csharp'];

/**
 * Assemble a complete JavaScript source from a .vortex file's embedded code.
 * Pure function — no side effects, no VS Code dependencies.
 */
export function assembleJsSource(vortexSource: string): AssembledSource | null {
  let doc;
  try {
    doc = parseDocument(vortexSource, { keepSourceTokens: true });
  } catch {
    return null;
  }

  const root = doc.contents;
  if (!isMap(root)) return null;

  // Find the first JS runtime block
  let runtimeNode = null;
  for (const rt of JS_RUNTIMES) {
    const n = root.get(rt, true);
    if (isMap(n)) {
      runtimeNode = n;
      break;
    }
  }
  if (!runtimeNode) return null;

  const lineOffsets = computeLineOffsets(vortexSource);
  const builder = new AssemblyBuilder(vortexSource, lineOffsets);

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
        const entryLine = item.range ? offsetToLine(item.range[0], lineOffsets) : -1;
        builder.addLine(importLine, entryLine, 0, 'import');
      }
    }
  }

  // 2) Emit vars as const declarations
  const varsNode = runtimeNode.get('vars', true);
  if (isMap(varsNode)) {
    builder.addSynthetic('');
    for (const pair of varsNode.items) {
      const keyNode = isScalar(pair.key) ? pair.key : null;
      const key = keyNode ? String(keyNode.value) : '';
      const val = isScalar(pair.value) ? pair.value.value : '';
      if (!key) continue;
      const varLine = keyNode?.range ? offsetToLine(keyNode.range[0], lineOffsets) : -1;
      const valStr = typeof val === 'string' ? `"${val}"` : String(val);
      builder.addLine(`const ${key} = ${valStr};`, varLine, 0, 'synthetic');
    }
  }

  // 3) Emit functions
  const functionsNode = runtimeNode.get('functions', true);
  if (isMap(functionsNode)) {
    builder.addSynthetic('');
    for (const pair of functionsNode.items) {
      const val = pair.value;
      if (!isScalar(val) || typeof val.value !== 'string') continue;
      builder.addBlockScalar(val as Scalar, 'function');
    }
  }

  // 4) Emit job commands
  const jobsNode = root.get('jobs', true);
  if (isSeq(jobsNode)) {
    for (const jobNode of jobsNode.items) {
      if (!isMap(jobNode)) continue;
      const shellScalar = jobNode.get('shell', true);
      const shell = isScalar(shellScalar) ? String(shellScalar.value).toLowerCase().trim() : '';
      if (!JS_RUNTIMES.includes(shell)) continue;

      const commandNode = jobNode.get('command', true);
      if (!isScalar(commandNode) || typeof commandNode.value !== 'string') continue;
      builder.addSynthetic('');
      builder.addBlockScalar(commandNode as Scalar, 'command');
    }
  }

  return builder.buildJs();
}

/**
 * Assemble a complete Go source file from a .vortex file's embedded Go code.
 * Pure function — no side effects, no VS Code dependencies.
 *
 * Produces a valid `package main` file with imports, vars, functions,
 * and job commands wrapped in `func main()`.
 */
export function assembleGoSource(vortexSource: string): AssembledSource | null {
  let doc;
  try {
    doc = parseDocument(vortexSource, { keepSourceTokens: true });
  } catch {
    return null;
  }

  const root = doc.contents;
  if (!isMap(root)) return null;

  const goNode = root.get('go', true);
  if (!isMap(goNode)) return null;

  const lineOffsets = computeLineOffsets(vortexSource);
  const builder = new AssemblyBuilder(vortexSource, lineOffsets);

  // 1) Package declaration
  builder.addSynthetic('package main');
  builder.addSynthetic('');

  // 2) Collect imports and emit import block
  const imports: string[] = [];
  const importsNode = goNode.get('imports', true);
  if (isSeq(importsNode)) {
    for (const item of importsNode.items) {
      if (!isMap(item)) continue;
      const pathScalar = item.get('path', true);
      const importPath = isScalar(pathScalar) ? String(pathScalar.value) : '';
      if (importPath) imports.push(importPath);
    }
  }
  if (imports.length > 0) {
    builder.addSynthetic('import (');
    for (const imp of imports) {
      builder.addSynthetic(`\t"${imp}"`);
    }
    builder.addSynthetic(')');
    builder.addSynthetic('');
  }

  // 3) Emit vars as package-level var declarations
  const varsNode = goNode.get('vars', true);
  if (isMap(varsNode)) {
    for (const pair of varsNode.items) {
      const keyNode = isScalar(pair.key) ? pair.key : null;
      const key = keyNode ? String(keyNode.value) : '';
      const val = isScalar(pair.value) ? pair.value.value : '';
      if (!key) continue;
      const varLine = keyNode?.range ? offsetToLine(keyNode.range[0], lineOffsets) : -1;
      const valStr = typeof val === 'string' ? `"${val}"` : String(val);
      builder.addLine(`var ${key} = ${valStr}`, varLine, 0, 'synthetic');
    }
    builder.addSynthetic('');
  }

  // 4) Emit functions
  const functionsNode = goNode.get('functions', true);
  if (isMap(functionsNode)) {
    for (const pair of functionsNode.items) {
      const val = pair.value;
      if (!isScalar(val) || typeof val.value !== 'string') continue;
      builder.addBlockScalar(val as Scalar, 'function');
      builder.addSynthetic('');
    }
  }

  // 5) Emit job commands inside func main()
  const jobsNode = root.get('jobs', true);
  let hasMainBody = false;
  if (isSeq(jobsNode)) {
    for (const jobNode of jobsNode.items) {
      if (!isMap(jobNode)) continue;
      const shellScalar = jobNode.get('shell', true);
      const shell = isScalar(shellScalar) ? String(shellScalar.value).toLowerCase().trim() : '';
      if (!GO_RUNTIMES.includes(shell)) continue;

      const commandNode = jobNode.get('command', true);
      if (!isScalar(commandNode) || typeof commandNode.value !== 'string') continue;

      if (!hasMainBody) {
        builder.addSynthetic('func main() {');
        hasMainBody = true;
      }
      builder.addBlockScalar(commandNode as Scalar, 'command');
    }
  }
  if (hasMainBody) {
    builder.addSynthetic('}');
  }

  return builder.buildGo();
}

/**
 * Assemble a complete C# source file from a .vortex file's embedded C# code.
 * Pure function — no side effects, no VS Code dependencies.
 *
 * Produces a valid single-file C# program:
 *   - using directives
 *   - top-level statements (job commands)
 *   - static class Vortex { vars + functions }
 *
 * This matches the runtime's two-file output (Shared.cs + Program.cs) but
 * combined into one file, which C# supports natively: top-level statements
 * can reference types declared later in the same file.
 */
export function assembleCSharpSource(vortexSource: string): AssembledSource | null {
  let doc;
  try {
    doc = parseDocument(vortexSource, { keepSourceTokens: true });
  } catch {
    return null;
  }

  const root = doc.contents;
  if (!isMap(root)) return null;

  const csNode = root.get('csharp', true);
  if (!isMap(csNode)) return null;

  const lineOffsets = computeLineOffsets(vortexSource);
  const builder = new AssemblyBuilder(vortexSource, lineOffsets);

  // Determine if we need the Vortex class
  const varsNode = csNode.get('vars', true);
  const functionsNode = csNode.get('functions', true);
  const hasSharedClass = (isMap(varsNode) && varsNode.items.length > 0) ||
                         (isMap(functionsNode) && functionsNode.items.length > 0);

  // 1) Emit using directives
  const usingsNode = csNode.get('usings', true);
  if (isSeq(usingsNode)) {
    for (const item of usingsNode.items) {
      if (isScalar(item) && typeof item.value === 'string') {
        builder.addSynthetic(`using ${item.value};`);
      }
    }
  }
  if (hasSharedClass) {
    builder.addSynthetic('using static Vortex;');
  }
  builder.addSynthetic('');

  // 2) Emit job commands as top-level statements
  const jobsNode = root.get('jobs', true);
  let hasCommands = false;
  if (isSeq(jobsNode)) {
    for (const jobNode of jobsNode.items) {
      if (!isMap(jobNode)) continue;
      const shellScalar = jobNode.get('shell', true);
      const shell = isScalar(shellScalar) ? String(shellScalar.value).toLowerCase().trim() : '';
      if (!CSHARP_RUNTIMES.includes(shell)) continue;

      const commandNode = jobNode.get('command', true);
      if (!isScalar(commandNode) || typeof commandNode.value !== 'string') continue;
      hasCommands = true;
      builder.addBlockScalar(commandNode as Scalar, 'command');
    }
  }

  // 3) Emit static class Vortex with vars and functions
  if (hasSharedClass) {
    if (hasCommands) builder.addSynthetic('');
    builder.addSynthetic('static class Vortex');
    builder.addSynthetic('{');

    // Vars as public static readonly fields
    if (isMap(varsNode)) {
      for (const pair of varsNode.items) {
        const keyNode = isScalar(pair.key) ? pair.key : null;
        const key = keyNode ? String(keyNode.value) : '';
        const val = isScalar(pair.value) ? pair.value.value : '';
        if (!key) continue;
        const varLine = keyNode?.range ? offsetToLine(keyNode.range[0], lineOffsets) : -1;
        const { typeName, literal } = csharpLiteral(val);
        builder.addLine(`public static readonly ${typeName} ${key} = ${literal};`, varLine, 0, 'synthetic');
      }
    }

    // Functions
    if (isMap(functionsNode)) {
      if (isMap(varsNode) && varsNode.items.length > 0) {
        builder.addSynthetic('');
      }
      for (const pair of functionsNode.items) {
        const val = pair.value;
        if (!isScalar(val) || typeof val.value !== 'string') continue;
        builder.addBlockScalar(val as Scalar, 'function');
        builder.addSynthetic('');
      }
    }

    builder.addSynthetic('}');
  }

  return builder.buildCSharp();
}

/**
 * Map a YAML value to a C# type name and literal.
 * Matches the runtime's csharpLiteral() in csharp_runtime.go.
 */
function csharpLiteral(value: any): { typeName: string; literal: string } {
  if (typeof value === 'string') {
    return { typeName: 'string', literal: JSON.stringify(value) };
  }
  if (typeof value === 'boolean') {
    return { typeName: 'bool', literal: value ? 'true' : 'false' };
  }
  if (typeof value === 'number') {
    if (Number.isInteger(value)) {
      return { typeName: 'int', literal: String(value) };
    }
    return { typeName: 'double', literal: String(value) };
  }
  return { typeName: 'string', literal: JSON.stringify(String(value)) };
}

/**
 * Helper class that builds the assembled source line by line,
 * constructing the SourceMap as it goes.
 */
class AssemblyBuilder {
  private lines: string[] = [];
  private mappings: LineMapping[] = [];

  constructor(
    private vortexSource: string,
    private lineOffsets: number[],
  ) {}

  addLine(text: string, vortexLine: number, colOffset: number, kind: LineMapping['kind']): void {
    const assembledLine = this.lines.length;
    this.lines.push(text);
    this.mappings.push({ assembledLine, vortexLine, colOffset, kind });
  }

  addSynthetic(text: string): void {
    this.addLine(text, -1, 0, 'synthetic');
  }

  /**
   * Add a YAML block scalar (function body or command).
   *
   * The yaml library parses block scalars and strips the YAML indentation,
   * giving us the raw content. But in the .vortex file, the content lines
   * have leading whitespace (the block scalar indent). We need to track
   * this indent as colOffset so we can map columns correctly.
   */
  addBlockScalar(scalar: Scalar, kind: 'function' | 'command'): void {
    const text = scalar.value as string;
    if (!text.trim()) return;

    const scalarRange = scalar.range;
    if (!scalarRange) return;

    const contentLines = text.split('\n');
    const nodeStart = scalarRange[0];

    // Find the first non-empty line to locate it in the vortex source
    const firstNonEmpty = contentLines.find(l => l.trim().length > 0) || contentLines[0];
    const contentStart = this.vortexSource.indexOf(firstNonEmpty, nodeStart);
    if (contentStart < 0) return;

    const startVortexLine = offsetToLine(contentStart, this.lineOffsets);
    const colOffset = contentStart - this.lineOffsets[startVortexLine];

    for (let i = 0; i < contentLines.length; i++) {
      const vortexLine = startVortexLine + i;
      this.addLine(contentLines[i], vortexLine, colOffset, kind);
    }
  }

  buildJs(): AssembledSource | null {
    if (this.lines.length === 0) return null;
    return {
      text: this.lines.join('\n'),
      sourceMap: new SourceMap(this.mappings),
      languageId: 'javascript',
    };
  }

  buildGo(): AssembledSource | null {
    if (this.lines.length === 0) return null;
    return {
      text: this.lines.join('\n'),
      sourceMap: new SourceMap(this.mappings),
      languageId: 'go',
    };
  }

  buildCSharp(): AssembledSource | null {
    if (this.lines.length === 0) return null;
    return {
      text: this.lines.join('\n'),
      sourceMap: new SourceMap(this.mappings),
      languageId: 'csharp',
    };
  }
}

// --- Utilities (exported for tests) ---

export function computeLineOffsets(source: string): number[] {
  const offsets: number[] = [0];
  for (let i = 0; i < source.length; i++) {
    if (source[i] === '\n') offsets.push(i + 1);
  }
  return offsets;
}

export function offsetToLine(offset: number, lineOffsets: number[]): number {
  let low = 0, high = lineOffsets.length - 1;
  while (low < high) {
    const mid = (low + high + 1) >> 1;
    if (lineOffsets[mid] <= offset) low = mid;
    else high = mid - 1;
  }
  return low;
}
