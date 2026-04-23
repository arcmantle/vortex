import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';
import { assembleJsSource, assembleGoSource, assembleCSharpSource, type AssembledSource } from './assembler';
import { TypeScriptClient } from './typescript-client';
import { GoplsClient, lspCompletionToVscode, lspHoverToVscode, lspSignatureHelpToVscode } from './gopls-client';
import { RoslynClient, roslynCompletionToVscode, roslynHoverToVscode, roslynSignatureHelpToVscode } from './roslyn-client';
import { log } from './log';

/**
 * Provides intellisense for embedded code in .vortex files.
 *
 * JS/TS: TypeScriptClient — in-process TypeScript language service (lazy-init).
 * Go:    GoplsClient — gopls subprocess via JSON-RPC over stdio (lazy-init).
 * C#:    RoslynClient — Roslyn subprocess via JSON-RPC over stdio (lazy-init).
 *
 * The language is detected from the assembled source, and each provider
 * routes to the correct backend.
 */
export class VortexIntellisenseProvider implements
  vscode.HoverProvider,
  vscode.CompletionItemProvider,
  vscode.DefinitionProvider,
  vscode.SignatureHelpProvider,
  vscode.DocumentHighlightProvider,
  vscode.Disposable
{
  // --- JS/TS backend (lazy) ---
  private tsClient: TypeScriptClient | null = null;

  // --- Go backend (lazy) ---
  private goplsClient: GoplsClient | null = null;
  private goplsStartPromise: Promise<boolean> | null = null;

  // --- C# backend (lazy) ---
  private roslynClient: RoslynClient | null = null;
  private roslynStartPromise: Promise<boolean> | null = null;

  // Cached assembled source per .vortex document URI.
  private assembledCache = new Map<string, { assembled: AssembledSource; fileName: string }>();

  // For "View Assembled Source" on-demand temp files.
  private tempDir = path.join(os.tmpdir(), 'vortex-intellisense');

  constructor() {}

  // --- Lazy initialization ---

  private ensureTypeScriptReady(): boolean {
    if (!this.tsClient) {
      this.tsClient = new TypeScriptClient();
    }
    return this.tsClient.start();
  }

  /**
   * Assemble the .vortex document's embedded code. Pure assembly + caching,
   * does NOT start any language servers.
   */
  private ensureAssembled(document: vscode.TextDocument): { assembled: AssembledSource; fileName: string } | null {
    const key = document.uri.toString();
    const text = document.getText();

    // Try C# first, then Go, then JS
    const assembled = assembleCSharpSource(text) ?? assembleGoSource(text) ?? assembleJsSource(text);
    if (!assembled) return null;

    const baseName = path.basename(document.uri.fsPath, '.vortex');
    const safeName = baseName.replace(/[^a-zA-Z0-9_-]/g, '_');
    const extMap: Record<string, string> = { go: '.go', csharp: '.cs', javascript: '.js' };
    const ext = extMap[assembled.languageId] || '.js';
    const fileName = path.join(path.dirname(document.uri.fsPath), `.${safeName}.vortex-assembled${ext}`);

    const existing = this.assembledCache.get(key);
    if (existing && existing.assembled.text === assembled.text) {
      // Ensure TS client has this file even on cache hit (e.g. after
      // switching from a C#/Go file — tsClient may have just been created)
      if (assembled.languageId === 'javascript' && this.tsClient) {
        this.tsClient.updateSource(existing.fileName, assembled.text);
      }
      return existing;
    }

    // Update cache
    this.assembledCache.set(key, { assembled, fileName });

    // For JS: update TS client's virtual file system
    if (assembled.languageId === 'javascript' && this.tsClient) {
      this.tsClient.updateSource(fileName, assembled.text);
    }

    return this.assembledCache.get(key)!;
  }

  /**
   * Ensure gopls is started and has the latest Go source.
   * Returns true if gopls is ready. Safe to call repeatedly.
   */
  private async ensureGoplsReady(document: vscode.TextDocument): Promise<boolean> {
    // Lazy-create the client
    if (!this.goplsClient) {
      this.goplsClient = new GoplsClient();
    }

    // Start gopls (deduplicated — won't restart if already running)
    if (!this.goplsStartPromise) {
      this.goplsStartPromise = this.goplsClient.start();
      this.goplsStartPromise.then(ok => {
        if (ok) {
          log('[go] gopls started');
        } else {
          log('[go] gopls not available — Go intellisense disabled');
        }
      });
    }

    const ok = await this.goplsStartPromise;
    if (!ok || !this.goplsClient.isReady) return false;

    // Update source
    const text = document.getText();
    const assembled = assembleGoSource(text);
    if (!assembled) return false;

    // Extract imports for go.mod
    const { parseDocument, isMap, isSeq, isScalar } = require('yaml');
    let goImports: Array<{ path: string; version: string }> = [];
    try {
      const doc = parseDocument(text, { keepSourceTokens: true });
      const root = doc.contents;
      if (isMap(root)) {
        const goNode = root.get('go', true);
        if (isMap(goNode)) {
          const importsNode = goNode.get('imports', true);
          if (isSeq(importsNode)) {
            for (const item of importsNode.items) {
              if (!isMap(item)) continue;
              const p = item.get('path', true);
              const v = item.get('version', true);
              if (isScalar(p) && isScalar(v)) {
                goImports.push({ path: String(p.value), version: String(v.value) });
              }
            }
          }
        }
      }
    } catch { /* ignore */ }

    await this.goplsClient.updateSource(assembled, goImports);
    return true;
  }

  /**
   * Ensure Roslyn is started and has the latest C# source.
   * Returns true if Roslyn is ready. Safe to call repeatedly.
   */
  private async ensureRoslynReady(document: vscode.TextDocument): Promise<boolean> {
    if (!this.roslynClient) {
      this.roslynClient = new RoslynClient();
    }

    if (!this.roslynStartPromise) {
      this.roslynStartPromise = this.roslynClient.start();
      this.roslynStartPromise.then(ok => {
        if (ok) {
          log('[csharp] Roslyn started');
        } else {
          log('[csharp] Roslyn not available — C# intellisense disabled');
        }
      });
    }

    const ok = await this.roslynStartPromise;
    if (!ok || !this.roslynClient.isReady) return false;

    // Update source
    const text = document.getText();
    const assembled = assembleCSharpSource(text);
    if (!assembled) return false;

    // Extract framework and packages from the vortex config
    const { parseDocument, isMap, isSeq, isScalar } = require('yaml');
    let framework = 'net8.0';
    let packages: Array<{ name: string; version: string }> = [];
    try {
      const doc = parseDocument(text, { keepSourceTokens: true });
      const root = doc.contents;
      if (isMap(root)) {
        const csNode = root.get('csharp', true);
        if (isMap(csNode)) {
          const fw = csNode.get('framework', true);
          if (isScalar(fw) && typeof fw.value === 'string') {
            framework = fw.value;
          }
          const pkgsNode = csNode.get('packages', true);
          if (isSeq(pkgsNode)) {
            for (const item of pkgsNode.items) {
              if (!isMap(item)) continue;
              const n = item.get('name', true);
              const v = item.get('version', true);
              if (isScalar(n) && isScalar(v)) {
                packages.push({ name: String(n.value), version: String(v.value) });
              }
            }
          }
        }
      }
    } catch { /* ignore */ }

    await this.roslynClient.updateSource(assembled, framework, packages);
    return true;
  }

  // --- Position mapping helpers ---

  private mapToAssembled(
    position: vscode.Position,
    assembled: AssembledSource
  ): vscode.Position | null {
    const result = assembled.sourceMap.toAssembled(position.line, position.character);
    if (!result) return null;
    return new vscode.Position(result.line, result.col);
  }

  private mapFromAssembled(
    assembledLine: number,
    assembledCol: number,
    assembled: AssembledSource
  ): vscode.Position | null {
    const result = assembled.sourceMap.toVortex(assembledLine, assembledCol);
    if (!result) return null;
    return new vscode.Position(result.line, result.col);
  }

  // --- Hover ---

  async provideHover(
    document: vscode.TextDocument,
    position: vscode.Position,
    _token: vscode.CancellationToken
  ): Promise<vscode.Hover | null> {
    const info = this.ensureAssembled(document);
    if (!info) return null;

    const mappedPos = this.mapToAssembled(position, info.assembled);
    if (!mappedPos) return null;

    if (info.assembled.languageId === 'go') {
      const ready = await this.ensureGoplsReady(document);
      if (!ready) return null;
      const hover = await this.goplsClient!.getHover(mappedPos.line, mappedPos.character);
      if (!hover) return null;
      return lspHoverToVscode(hover);
    }

    if (info.assembled.languageId === 'csharp') {
      const ready = await this.ensureRoslynReady(document);
      if (!ready) return null;
      const hover = await this.roslynClient!.getHover(mappedPos.line, mappedPos.character);
      if (!hover) return null;
      return roslynHoverToVscode(hover);
    }

    // JS path — lazy-init TS
    if (!this.ensureTypeScriptReady()) return null;

    return this.tsClient!.getHover(info.fileName, info.assembled, mappedPos.line, mappedPos.character);
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

    const mappedPos = this.mapToAssembled(position, info.assembled);
    if (!mappedPos) return null;

    const lang = info.assembled.languageId;
    const wordRange = this.getWordRangeAtCursor(document, position);

    if (lang === 'go') {
      const ready = await this.ensureGoplsReady(document);
      if (!ready) {
        log(`[go] completions: gopls not available`);
        return null;
      }
      log(`[go] completions: vortex(${position.line},${position.character}) -> assembled(${mappedPos.line},${mappedPos.character})`);
      const items = await this.goplsClient!.getCompletions(mappedPos.line, mappedPos.character);
      log(`[go] completions: ${items.length} items`);
      return items.map(item => lspCompletionToVscode(item, wordRange));
    }

    if (lang === 'csharp') {
      const ready = await this.ensureRoslynReady(document);
      if (!ready) {
        log(`[csharp] completions: Roslyn not available`);
        return null;
      }
      log(`[csharp] completions: vortex(${position.line},${position.character}) -> assembled(${mappedPos.line},${mappedPos.character})`);
      const items = await this.roslynClient!.getCompletions(mappedPos.line, mappedPos.character);
      log(`[csharp] completions: ${items.length} items`);
      return items.map(item => roslynCompletionToVscode(item, wordRange));
    }

    // JS path — lazy-init TS
    if (!this.ensureTypeScriptReady()) {
      log(`[js] completions: TypeScript not available`);
      return null;
    }

    log(`[js] completions: vortex(${position.line},${position.character}) -> assembled(${mappedPos.line},${mappedPos.character})`);
    return this.tsClient!.getCompletions(info.fileName, info.assembled, mappedPos.line, mappedPos.character, wordRange);
  }

  private getWordRangeAtCursor(document: vscode.TextDocument, position: vscode.Position): vscode.Range {
    const line = document.lineAt(position.line).text;
    let start = position.character;
    while (start > 0 && /[a-zA-Z0-9_$]/.test(line[start - 1])) {
      start--;
    }
    return new vscode.Range(position.line, start, position.line, position.character);
  }

  // --- Definition ---

  async provideDefinition(
    document: vscode.TextDocument,
    position: vscode.Position,
    _token: vscode.CancellationToken
  ): Promise<vscode.Definition | null> {
    const info = this.ensureAssembled(document);
    if (!info) return null;

    const mappedPos = this.mapToAssembled(position, info.assembled);
    if (!mappedPos) return null;

    if (info.assembled.languageId === 'go') {
      const ready = await this.ensureGoplsReady(document);
      if (!ready) return null;
      const defs = await this.goplsClient!.getDefinition(mappedPos.line, mappedPos.character);
      if (!defs || defs.length === 0) return null;

      const results: vscode.Location[] = [];
      for (const def of defs) {
        const defUri = def.uri.startsWith('file://') ? def.uri : `file://${def.uri}`;
        const defPath = vscode.Uri.parse(defUri).fsPath;

        // If definition is in our assembled file, remap to .vortex
        if (defPath === this.goplsClient!['mainFilePath']) {
          const startVortex = this.mapFromAssembled(
            def.range.start.line, def.range.start.character, info.assembled
          );
          const endVortex = this.mapFromAssembled(
            def.range.end.line, def.range.end.character, info.assembled
          );
          if (startVortex && endVortex) {
            results.push(new vscode.Location(document.uri, new vscode.Range(startVortex, endVortex)));
          }
        } else {
          // External definition (Go stdlib, etc.)
          results.push(new vscode.Location(
            vscode.Uri.file(defPath),
            new vscode.Range(
              def.range.start.line, def.range.start.character,
              def.range.end.line, def.range.end.character,
            )
          ));
        }
      }
      return results.length > 0 ? results : null;
    }

    if (info.assembled.languageId === 'csharp') {
      const ready = await this.ensureRoslynReady(document);
      if (!ready) return null;
      const defs = await this.roslynClient!.getDefinition(mappedPos.line, mappedPos.character);
      if (!defs || defs.length === 0) return null;

      const results: vscode.Location[] = [];
      for (const def of defs) {
        const defUri = def.uri.startsWith('file://') ? def.uri : `file://${def.uri}`;
        const defPath = vscode.Uri.parse(defUri).fsPath;

        if (defPath === this.roslynClient!['programFilePath']) {
          const startVortex = this.mapFromAssembled(
            def.range.start.line, def.range.start.character, info.assembled
          );
          const endVortex = this.mapFromAssembled(
            def.range.end.line, def.range.end.character, info.assembled
          );
          if (startVortex && endVortex) {
            results.push(new vscode.Location(document.uri, new vscode.Range(startVortex, endVortex)));
          }
        } else {
          results.push(new vscode.Location(
            vscode.Uri.file(defPath),
            new vscode.Range(
              def.range.start.line, def.range.start.character,
              def.range.end.line, def.range.end.character,
            )
          ));
        }
      }
      return results.length > 0 ? results : null;
    }

    // JS path — lazy-init TS
    if (!this.ensureTypeScriptReady()) return null;

    const defs = this.tsClient!.getDefinition(info.fileName, info.assembled, mappedPos.line, mappedPos.character);
    if (!defs) return null;

    const results: vscode.Location[] = [];
    for (const def of defs.internal) {
      const startVortex = this.mapFromAssembled(def.startLine, def.startCol, info.assembled);
      const endVortex = this.mapFromAssembled(def.endLine, def.endCol, info.assembled);
      if (startVortex && endVortex) {
        results.push(new vscode.Location(document.uri, new vscode.Range(startVortex, endVortex)));
      }
    }
    results.push(...defs.external);

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

    const mappedPos = this.mapToAssembled(position, info.assembled);
    if (!mappedPos) return null;

    if (info.assembled.languageId === 'go') {
      const ready = await this.ensureGoplsReady(document);
      if (!ready) return null;
      const sigHelp = await this.goplsClient!.getSignatureHelp(mappedPos.line, mappedPos.character);
      if (!sigHelp) return null;
      return lspSignatureHelpToVscode(sigHelp);
    }

    if (info.assembled.languageId === 'csharp') {
      const ready = await this.ensureRoslynReady(document);
      if (!ready) return null;
      const sigHelp = await this.roslynClient!.getSignatureHelp(mappedPos.line, mappedPos.character);
      if (!sigHelp) return null;
      return roslynSignatureHelpToVscode(sigHelp);
    }

    // JS path — lazy-init TS
    if (!this.ensureTypeScriptReady()) return null;

    return this.tsClient!.getSignatureHelp(info.fileName, info.assembled, mappedPos.line, mappedPos.character);
  }

  // --- Document Highlights (suppress blue block highlight) ---

  provideDocumentHighlights(
    _document: vscode.TextDocument,
    _position: vscode.Position,
    _token: vscode.CancellationToken
  ): vscode.DocumentHighlight[] {
    return [];
  }

  // --- Public methods ---

  /**
   * Called when a .vortex document changes. Updates the in-memory assembled source
   * so the TS language service sees the latest content immediately.
   */
  handleDocumentChange(document: vscode.TextDocument): void {
    this.ensureAssembled(document);
  }

  /**
   * Open the assembled source in a side editor for debugging/inspection.
   * This is the ONLY place we write to disk — and only when explicitly requested.
   */
  async viewAssembledSource(document: vscode.TextDocument): Promise<void> {
    const info = this.ensureAssembled(document);
    if (!info) {
      vscode.window.showWarningMessage('No assembled source available for this file.');
      return;
    }
    fs.mkdirSync(this.tempDir, { recursive: true });
    const baseName = path.basename(info.fileName);
    const viewPath = path.join(this.tempDir, baseName);
    fs.writeFileSync(viewPath, info.assembled.text);
    await vscode.window.showTextDocument(vscode.Uri.file(viewPath), {
      viewColumn: vscode.ViewColumn.Beside,
      preview: true,
      preserveFocus: true,
    });
  }

  /**
   * Remove cached data for a closed document.
   */
  removeDocument(uri: vscode.Uri): void {
    const key = uri.toString();
    const info = this.assembledCache.get(key);
    if (info) {
      if (this.tsClient) {
        this.tsClient.removeFile(info.fileName);
      }
      this.assembledCache.delete(key);
    }
  }

  dispose(): void {
    if (this.tsClient) {
      this.tsClient.dispose();
      this.tsClient = null;
    }
    if (this.goplsClient) {
      this.goplsClient.dispose();
      this.goplsClient = null;
    }
    if (this.roslynClient) {
      this.roslynClient.dispose();
      this.roslynClient = null;
    }
    // Clean up any temp files from "View Assembled Source"
    try {
      const files = fs.readdirSync(this.tempDir);
      for (const f of files) {
        try { fs.unlinkSync(path.join(this.tempDir, f)); } catch { /* ignore */ }
      }
      fs.rmdirSync(this.tempDir);
    } catch { /* ignore */ }
  }
}
