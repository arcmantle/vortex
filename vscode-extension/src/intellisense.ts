import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';
import { assembleSource, type AssembledSource, type SourceMapEntry } from './assembler';

/**
 * Manages the virtual JS document and provides intellisense for embedded
 * JavaScript in .vortex files by proxying to VS Code's TypeScript service.
 */
export class VortexIntellisenseProvider implements
  vscode.HoverProvider,
  vscode.CompletionItemProvider,
  vscode.DefinitionProvider,
  vscode.SignatureHelpProvider,
  vscode.DocumentHighlightProvider,
  vscode.Disposable
{
  private tempDir: string;
  private tempFiles = new Map<string, { filePath: string; assembled: AssembledSource }>();
  private disposables: vscode.Disposable[] = [];

  constructor() {
    this.tempDir = path.join(os.tmpdir(), 'vortex-intellisense');
    fs.mkdirSync(this.tempDir, { recursive: true });

    // Write a jsconfig.json so TS service picks up the files
    const jsconfig = {
      compilerOptions: {
        target: 'ES2022',
        module: 'ES2022',
        moduleResolution: 'node',
        allowJs: true,
        checkJs: false,
        strict: false,
        noEmit: true,
      },
      include: ['*.js'],
    };
    fs.writeFileSync(
      path.join(this.tempDir, 'jsconfig.json'),
      JSON.stringify(jsconfig, null, 2)
    );
  }

  /**
   * Ensure the assembled source file is up-to-date for the given .vortex document.
   * Returns the assembled source info, or null if assembly failed.
   */
  private ensureAssembled(document: vscode.TextDocument): { filePath: string; assembled: AssembledSource } | null {
    const key = document.uri.toString();
    const existing = this.tempFiles.get(key);

    const assembled = assembleSource(document.getText());
    if (!assembled) return null;

    // Use a stable filename derived from the vortex file
    const baseName = path.basename(document.uri.fsPath, '.vortex');
    const safeName = baseName.replace(/[^a-zA-Z0-9_-]/g, '_');
    const filePath = path.join(this.tempDir, `${safeName}.js`);

    // Only rewrite if content changed
    if (!existing || existing.assembled.text !== assembled.text) {
      fs.writeFileSync(filePath, assembled.text);
      this.tempFiles.set(key, { filePath, assembled });
    }

    return this.tempFiles.get(key)!;
  }

  /**
   * Map a position in the .vortex file to the corresponding position
   * in the assembled JS source. Returns null if position isn't in an embedded region.
   */
  private mapToAssembled(
    document: vscode.TextDocument,
    position: vscode.Position,
    assembled: AssembledSource
  ): vscode.Position | null {
    const vortexLine = position.line;
    const vortexCol = position.character;

    // Find the assembled line that maps from this vortex line
    for (let i = 0; i < assembled.sourceMap.length; i++) {
      const entry = assembled.sourceMap[i];
      if (entry.vortexLine === vortexLine && entry.kind !== 'synthetic') {
        // The column in the assembled source = vortex col - indent offset
        const assembledCol = Math.max(0, vortexCol - entry.col);
        return new vscode.Position(i, assembledCol);
      }
    }

    return null;
  }

  /**
   * Map a position in the assembled JS source back to the .vortex file.
   */
  private mapFromAssembled(
    assembledLine: number,
    assembledCol: number,
    assembled: AssembledSource
  ): vscode.Position | null {
    if (assembledLine < 0 || assembledLine >= assembled.sourceMap.length) return null;
    const entry = assembled.sourceMap[assembledLine];
    if (entry.vortexLine < 0) return null;

    const vortexCol = assembledCol + entry.col;
    return new vscode.Position(entry.vortexLine, vortexCol);
  }

  /**
   * Map a Range from the assembled source back to the .vortex file.
   */
  private mapRangeFromAssembled(
    range: vscode.Range,
    assembled: AssembledSource
  ): vscode.Range | null {
    const start = this.mapFromAssembled(range.start.line, range.start.character, assembled);
    const end = this.mapFromAssembled(range.end.line, range.end.character, assembled);
    if (!start || !end) return null;
    return new vscode.Range(start, end);
  }

  // --- Hover ---

  async provideHover(
    document: vscode.TextDocument,
    position: vscode.Position,
    _token: vscode.CancellationToken
  ): Promise<vscode.Hover | null> {
    const info = this.ensureAssembled(document);
    if (!info) return null;

    const mappedPos = this.mapToAssembled(document, position, info.assembled);
    if (!mappedPos) return null;

    const uri = vscode.Uri.file(info.filePath);
    const hovers = await vscode.commands.executeCommand<vscode.Hover[]>(
      'vscode.executeHoverProvider',
      uri,
      mappedPos
    );

    if (!hovers || hovers.length === 0) return null;

    // Return the hover content at the original position
    const hover = hovers[0];
    return new vscode.Hover(hover.contents, undefined);
  }

  // --- Completions ---

  async provideCompletionItems(
    document: vscode.TextDocument,
    position: vscode.Position,
    _token: vscode.CancellationToken,
    _context: vscode.CompletionContext
  ): Promise<vscode.CompletionItem[] | null> {
    const info = this.ensureAssembled(document);
    if (!info) return null;

    const mappedPos = this.mapToAssembled(document, position, info.assembled);
    if (!mappedPos) return null;

    const uri = vscode.Uri.file(info.filePath);
    const completions = await vscode.commands.executeCommand<vscode.CompletionList>(
      'vscode.executeCompletionItemProvider',
      uri,
      mappedPos
    );

    if (!completions || completions.items.length === 0) return null;

    return completions.items;
  }

  // --- Definition ---

  async provideDefinition(
    document: vscode.TextDocument,
    position: vscode.Position,
    _token: vscode.CancellationToken
  ): Promise<vscode.Definition | null> {
    const info = this.ensureAssembled(document);
    if (!info) return null;

    const mappedPos = this.mapToAssembled(document, position, info.assembled);
    if (!mappedPos) return null;

    const uri = vscode.Uri.file(info.filePath);
    const definitions = await vscode.commands.executeCommand<(vscode.Location | vscode.LocationLink)[]>(
      'vscode.executeDefinitionProvider',
      uri,
      mappedPos
    );

    if (!definitions || definitions.length === 0) return null;

    // Map definitions back: if they point into our temp file, remap to .vortex
    const results: vscode.Location[] = [];
    for (const def of definitions) {
      const loc = 'targetUri' in def
        ? new vscode.Location(def.targetUri, def.targetRange)
        : def;

      if (loc.uri.fsPath === info.filePath) {
        // Remap to the vortex file
        const mappedRange = this.mapRangeFromAssembled(loc.range, info.assembled);
        if (mappedRange) {
          results.push(new vscode.Location(document.uri, mappedRange));
        }
      } else {
        // External definition (e.g., node_modules) — keep as-is
        results.push(loc);
      }
    }

    return results.length > 0 ? results : null;
  }

  // --- Signature Help ---

  async provideSignatureHelp(
    document: vscode.TextDocument,
    position: vscode.Position,
    _token: vscode.CancellationToken,
    _context: vscode.SignatureHelpContext
  ): Promise<vscode.SignatureHelp | null> {
    const info = this.ensureAssembled(document);
    if (!info) return null;

    const mappedPos = this.mapToAssembled(document, position, info.assembled);
    if (!mappedPos) return null;

    const uri = vscode.Uri.file(info.filePath);
    const help = await vscode.commands.executeCommand<vscode.SignatureHelp>(
      'vscode.executeSignatureHelpProvider',
      uri,
      mappedPos
    );

    return help || null;
  }

  // --- Document Highlights (suppress YAML extension's blue block highlight) ---

  provideDocumentHighlights(
    document: vscode.TextDocument,
    position: vscode.Position,
    _token: vscode.CancellationToken
  ): vscode.DocumentHighlight[] | null {
    const info = this.ensureAssembled(document);
    if (!info) return null;

    // If the cursor is inside an embedded code region, return empty array
    // to suppress other providers from highlighting the whole block scalar
    const mappedPos = this.mapToAssembled(document, position, info.assembled);
    if (mappedPos) {
      return [];
    }
    return null;
  }

  // --- Cleanup ---

  dispose(): void {
    for (const d of this.disposables) d.dispose();

    // Clean up temp files
    for (const [, { filePath }] of this.tempFiles) {
      try { fs.unlinkSync(filePath); } catch { /* ignore */ }
    }
    try {
      fs.unlinkSync(path.join(this.tempDir, 'jsconfig.json'));
      fs.rmdirSync(this.tempDir);
    } catch { /* ignore */ }
  }

  /**
   * Remove cached data for a closed document.
   */
  removeDocument(uri: vscode.Uri): void {
    const key = uri.toString();
    const info = this.tempFiles.get(key);
    if (info) {
      try { fs.unlinkSync(info.filePath); } catch { /* ignore */ }
      this.tempFiles.delete(key);
    }
  }
}
