import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';
import { log } from './log';
import type { AssembledSource } from './assembler';

/**
 * TypeScript language service client for JS/TS intellisense in .vortex files.
 *
 * Unlike gopls and Roslyn (external LSP servers over stdio), this uses
 * TypeScript's in-process LanguageService API. We load TypeScript from
 * VS Code's built-in extension to avoid bundling it.
 */

type TS = typeof import('typescript');

// --- Helpers ---

/** Convert line+column to byte offset in text. */
function lineColToOffset(text: string, line: number, col: number): number {
  let currentLine = 0;
  let offset = 0;
  while (currentLine < line && offset < text.length) {
    if (text[offset] === '\n') currentLine++;
    offset++;
  }
  return offset + col;
}

/** Convert byte offset to line+column. */
function offsetToLineCol(text: string, offset: number): { line: number; col: number } {
  let line = 0;
  let lineStart = 0;
  for (let i = 0; i < offset && i < text.length; i++) {
    if (text[i] === '\n') {
      line++;
      lineStart = i + 1;
    }
  }
  return { line, col: offset - lineStart };
}

/** Convert TypeScript display parts to a plain string. */
function displayPartsToString(parts: ReadonlyArray<{ text: string }> | undefined): string {
  if (!parts) return '';
  return parts.map(p => p.text).join('');
}

/** Map TypeScript's ScriptElementKind to VS Code CompletionItemKind. */
function mapCompletionKind(kind: string): vscode.CompletionItemKind {
  switch (kind) {
    case 'keyword': return vscode.CompletionItemKind.Keyword;
    case 'script': case 'module': case 'external module name': return vscode.CompletionItemKind.Module;
    case 'class': case 'local class': return vscode.CompletionItemKind.Class;
    case 'interface': return vscode.CompletionItemKind.Interface;
    case 'type': case 'primitive type': return vscode.CompletionItemKind.TypeParameter;
    case 'enum': return vscode.CompletionItemKind.Enum;
    case 'enum member': return vscode.CompletionItemKind.EnumMember;
    case 'var': case 'local var': case 'let': return vscode.CompletionItemKind.Variable;
    case 'const': return vscode.CompletionItemKind.Constant;
    case 'function': case 'local function': return vscode.CompletionItemKind.Function;
    case 'method': case 'construct': return vscode.CompletionItemKind.Method;
    case 'getter': case 'setter': case 'property': return vscode.CompletionItemKind.Property;
    case 'constructor': return vscode.CompletionItemKind.Constructor;
    case 'parameter': return vscode.CompletionItemKind.Variable;
    case 'type parameter': return vscode.CompletionItemKind.TypeParameter;
    case 'alias': return vscode.CompletionItemKind.Reference;
    case 'directory': return vscode.CompletionItemKind.Folder;
    case 'string': return vscode.CompletionItemKind.Value;
    default: return vscode.CompletionItemKind.Text;
  }
}

/**
 * Load TypeScript from VS Code's built-in extensions.
 */
function loadTypeScript(): TS | null {
  const tsExt = vscode.extensions.getExtension('vscode.typescript-language-features');
  const candidates: string[] = [];

  if (tsExt) {
    candidates.push(path.join(tsExt.extensionPath, 'node_modules', 'typescript'));
    candidates.push(path.join(path.dirname(tsExt.extensionPath), 'node_modules', 'typescript'));
  }

  for (const tsPath of candidates) {
    try {
      const ts = require(tsPath);
      log(`[js] Loaded TypeScript ${ts.version} from ${tsPath}`);
      return ts;
    } catch { /* try next */ }
  }

  log(`[js] Failed to load TypeScript from any known path: ${candidates.join(', ')}`);
  return null;
}

export class TypeScriptClient {
  private ts: TS | null = null;
  private service: import('typescript').LanguageService | null = null;
  private initialized = false;
  private fileContents = new Map<string, string>();
  private fileVersions = new Map<string, number>();
  private bundledTypeRoots: string[];

  constructor() {
    this.bundledTypeRoots = [path.join(__dirname, 'types', '@types')];
  }

  /**
   * Initialize the TypeScript language service.
   * Returns true if successful. Safe to call repeatedly.
   */
  start(): boolean {
    if (this.initialized) return this.service !== null;
    this.initialized = true;

    this.ts = loadTypeScript();
    if (!this.ts) return false;

    this.initLanguageService();
    log('[js] TypeScript language service initialized');
    log(`[js] typeRoots: ${this.bundledTypeRoots.join(', ')}`);
    return true;
  }

  get isReady(): boolean {
    return this.service !== null;
  }

  /**
   * Update the virtual file content for the assembled JS source.
   */
  updateSource(fileName: string, text: string): void {
    this.fileContents.set(fileName, text);
    this.fileVersions.set(fileName, (this.fileVersions.get(fileName) ?? 0) + 1);
  }

  /**
   * Remove a virtual file (when the .vortex document is closed).
   */
  removeFile(fileName: string): void {
    this.fileContents.delete(fileName);
    this.fileVersions.delete(fileName);
  }

  getHover(fileName: string, assembled: AssembledSource, line: number, character: number): vscode.Hover | null {
    if (!this.service || !this.ts) return null;

    const offset = lineColToOffset(assembled.text, line, character);
    const quickInfo = this.service.getQuickInfoAtPosition(fileName, offset);
    if (!quickInfo) return null;

    const parts: vscode.MarkdownString[] = [];
    const display = displayPartsToString(quickInfo.displayParts);
    if (display) {
      parts.push(new vscode.MarkdownString().appendCodeblock(display, 'typescript'));
    }
    const docs = displayPartsToString(quickInfo.documentation);
    if (docs) {
      parts.push(new vscode.MarkdownString(docs));
    }

    return parts.length > 0 ? new vscode.Hover(parts) : null;
  }

  getCompletions(
    fileName: string,
    assembled: AssembledSource,
    line: number,
    character: number,
    wordRange: vscode.Range,
  ): vscode.CompletionItem[] | null {
    if (!this.service) return null;

    const offset = lineColToOffset(assembled.text, line, character);
    const assembledLines = assembled.text.split('\n');
    const assembledLine = assembledLines[line] ?? '';
    log(`[js] completions: assembled(${line},${character}) offset=${offset} line="${assembledLine}"`);

    const completions = this.service.getCompletionsAtPosition(fileName, offset, {});
    if (!completions) {
      log(`[js] completions: TS returned null`);
      return null;
    }

    log(`[js] completions: ${completions.entries.length} items`);

    const results: vscode.CompletionItem[] = [];
    for (const entry of completions.entries) {
      const item = new vscode.CompletionItem(entry.name, mapCompletionKind(entry.kind));
      item.sortText = entry.sortText;
      item.range = wordRange;
      if (entry.isRecommended) item.preselect = true;
      results.push(item);
    }
    return results;
  }

  getDefinition(
    fileName: string,
    assembled: AssembledSource,
    line: number,
    character: number,
  ): { internal: Array<{ startLine: number; startCol: number; endLine: number; endCol: number }>; external: vscode.Location[] } | null {
    if (!this.service || !this.ts) return null;

    const offset = lineColToOffset(assembled.text, line, character);
    const definitions = this.service.getDefinitionAtPosition(fileName, offset);
    if (!definitions || definitions.length === 0) return null;

    const internal: Array<{ startLine: number; startCol: number; endLine: number; endCol: number }> = [];
    const external: vscode.Location[] = [];

    for (const def of definitions) {
      if (def.fileName === fileName) {
        const start = offsetToLineCol(assembled.text, def.textSpan.start);
        const end = offsetToLineCol(assembled.text, def.textSpan.start + def.textSpan.length);
        internal.push({ startLine: start.line, startCol: start.col, endLine: end.line, endCol: end.col });
      } else {
        try {
          const program = this.service.getProgram();
          const sourceFile = program?.getSourceFile(def.fileName);
          if (sourceFile) {
            const start = this.ts.getLineAndCharacterOfPosition(sourceFile, def.textSpan.start);
            const end = this.ts.getLineAndCharacterOfPosition(sourceFile, def.textSpan.start + def.textSpan.length);
            external.push(new vscode.Location(
              vscode.Uri.file(def.fileName),
              new vscode.Range(start.line, start.character, end.line, end.character)
            ));
          }
        } catch { /* file might not be readable */ }
      }
    }

    return (internal.length > 0 || external.length > 0) ? { internal, external } : null;
  }

  getSignatureHelp(
    fileName: string,
    assembled: AssembledSource,
    line: number,
    character: number,
  ): vscode.SignatureHelp | null {
    if (!this.service) return null;

    const offset = lineColToOffset(assembled.text, line, character);
    const sigHelp = this.service.getSignatureHelpItems(fileName, offset, {});
    if (!sigHelp) return null;

    const result = new vscode.SignatureHelp();
    result.activeSignature = sigHelp.selectedItemIndex;
    result.activeParameter = sigHelp.argumentIndex;

    for (const item of sigHelp.items) {
      const label = [
        ...item.prefixDisplayParts,
        ...item.parameters.flatMap((p, i) =>
          i > 0 ? [{ text: ', ' }, ...p.displayParts] : p.displayParts
        ),
        ...item.suffixDisplayParts,
      ].map(p => p.text).join('');

      const sig = new vscode.SignatureInformation(label);
      sig.documentation = new vscode.MarkdownString(displayPartsToString(item.documentation));
      for (const param of item.parameters) {
        const paramLabel = displayPartsToString(param.displayParts);
        const paramDoc = displayPartsToString(param.documentation);
        sig.parameters.push(new vscode.ParameterInformation(paramLabel, paramDoc || undefined));
      }
      result.signatures.push(sig);
    }

    return result;
  }

  dispose(): void {
    if (this.service) {
      this.service.dispose();
      this.service = null;
    }
  }

  private initLanguageService(): void {
    const ts = this.ts!;
    const self = this;

    const host: import('typescript').LanguageServiceHost = {
      getScriptFileNames: () => [...self.fileContents.keys()],
      getScriptVersion: (fileName) => String(self.fileVersions.get(fileName) ?? 0),
      getScriptSnapshot: (fileName) => {
        const content = self.fileContents.get(fileName);
        if (content !== undefined) {
          return ts.ScriptSnapshot.fromString(content);
        }
        try {
          return ts.ScriptSnapshot.fromString(fs.readFileSync(fileName, 'utf-8'));
        } catch {
          return undefined;
        }
      },
      getCurrentDirectory: () =>
        vscode.workspace.workspaceFolders?.[0]?.uri.fsPath ?? process.cwd(),
      getCompilationSettings: () => ({
        target: ts.ScriptTarget.ES2022,
        module: ts.ModuleKind.ES2022,
        moduleResolution: ts.ModuleResolutionKind.Bundler,
        allowJs: true,
        checkJs: true,
        strict: false,
        noEmit: true,
        typeRoots: self.bundledTypeRoots,
        types: ['node'],
      }),
      getDefaultLibFileName: (opts) => ts.getDefaultLibFilePath(opts),
      fileExists: (fileName) =>
        self.fileContents.has(fileName) || ts.sys.fileExists(fileName),
      readFile: (fileName, encoding) => {
        const content = self.fileContents.get(fileName);
        if (content !== undefined) return content;
        return ts.sys.readFile(fileName, encoding);
      },
      readDirectory: ts.sys.readDirectory,
      directoryExists: ts.sys.directoryExists,
      getDirectories: ts.sys.getDirectories,
    };

    this.service = ts.createLanguageService(host, ts.createDocumentRegistry());
  }
}
