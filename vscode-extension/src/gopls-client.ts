import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';
import * as cp from 'child_process';
import { log } from './log';
import type { AssembledSource } from './assembler';

/**
 * gopls LSP client for Go intellisense in .vortex files.
 *
 * Unlike the in-memory TS approach, gopls requires real files on disk
 * because it invokes the Go toolchain (go list, etc.). We maintain a
 * temporary project directory with go.mod + main.go and communicate
 * with gopls via JSON-RPC over stdio.
 */

// LSP message types (subset we need)
interface LspPosition { line: number; character: number; }
interface LspRange { start: LspPosition; end: LspPosition; }
interface LspLocation { uri: string; range: LspRange; }
interface LspTextDocumentIdentifier { uri: string; }
interface LspVersionedTextDocumentIdentifier { uri: string; version: number; }
interface LspCompletionItem {
  label: string;
  kind?: number;
  detail?: string;
  documentation?: string | { kind: string; value: string };
  insertText?: string;
  textEdit?: { range: LspRange; newText: string };
  sortText?: string;
}
interface LspCompletionList {
  isIncomplete: boolean;
  items: LspCompletionItem[];
}
interface LspHover {
  contents: string | { kind: string; value: string } | Array<string | { language: string; value: string }>;
  range?: LspRange;
}
interface LspSignatureHelp {
  signatures: Array<{
    label: string;
    documentation?: string | { kind: string; value: string };
    parameters?: Array<{ label: string | [number, number]; documentation?: string | { kind: string; value: string } }>;
  }>;
  activeSignature?: number;
  activeParameter?: number;
}

// LSP completion item kinds
const LSP_COMPLETION_KIND_MAP: Record<number, vscode.CompletionItemKind> = {
  1: vscode.CompletionItemKind.Text,
  2: vscode.CompletionItemKind.Method,
  3: vscode.CompletionItemKind.Function,
  4: vscode.CompletionItemKind.Constructor,
  5: vscode.CompletionItemKind.Field,
  6: vscode.CompletionItemKind.Variable,
  7: vscode.CompletionItemKind.Class,
  8: vscode.CompletionItemKind.Interface,
  9: vscode.CompletionItemKind.Module,
  10: vscode.CompletionItemKind.Property,
  11: vscode.CompletionItemKind.Unit,
  12: vscode.CompletionItemKind.Value,
  13: vscode.CompletionItemKind.Enum,
  14: vscode.CompletionItemKind.Keyword,
  15: vscode.CompletionItemKind.Snippet,
  16: vscode.CompletionItemKind.Color,
  17: vscode.CompletionItemKind.File,
  18: vscode.CompletionItemKind.Reference,
  19: vscode.CompletionItemKind.Folder,
  20: vscode.CompletionItemKind.EnumMember,
  21: vscode.CompletionItemKind.Constant,
  22: vscode.CompletionItemKind.Struct,
  23: vscode.CompletionItemKind.Event,
  24: vscode.CompletionItemKind.Operator,
  25: vscode.CompletionItemKind.TypeParameter,
};

export class GoplsClient {
  private process: cp.ChildProcess | null = null;
  private projectDir: string;
  private mainFilePath: string;
  private mainFileUri: string;
  private goModPath: string;
  private initialized = false;
  private nextId = 1;
  private pendingRequests = new Map<number, { resolve: (v: any) => void; reject: (e: any) => void }>();
  private buffer = '';
  private contentLength = -1;
  private fileVersion = 0;
  private currentText = '';
  private startPromise: Promise<boolean> | null = null;

  constructor() {
    this.projectDir = path.join(os.tmpdir(), 'vortex-gopls-' + process.pid);
    this.mainFilePath = path.join(this.projectDir, 'main.go');
    this.mainFileUri = `file://${this.mainFilePath}`;
    this.goModPath = path.join(this.projectDir, 'go.mod');
  }

  /**
   * Start gopls and initialize the LSP connection.
   * Returns true if successful, false if gopls is not available.
   */
  async start(): Promise<boolean> {
    // Deduplicate concurrent start calls
    if (this.startPromise) return this.startPromise;
    if (this.initialized) return true;

    this.startPromise = this._doStart();
    try {
      return await this.startPromise;
    } finally {
      this.startPromise = null;
    }
  }

  private async _doStart(): Promise<boolean> {
    // Check if gopls exists
    const goplsPath = this.findGopls();
    if (!goplsPath) {
      log('[go] gopls not found in PATH');
      return false;
    }

    // Create project directory with go.mod
    fs.mkdirSync(this.projectDir, { recursive: true });
    fs.writeFileSync(this.goModPath, 'module vortex/runtime\n\ngo 1.21\n');
    fs.writeFileSync(this.mainFilePath, 'package main\n');

    log(`[go] Starting gopls from ${goplsPath}, project at ${this.projectDir}`);

    try {
      this.process = cp.spawn(goplsPath, ['serve', '-rpc.trace'], {
        stdio: ['pipe', 'pipe', 'pipe'],
        cwd: this.projectDir,
        env: { ...process.env, GOFLAGS: '-mod=mod' },
      });

      this.process.stdout!.on('data', (data: Buffer) => this.onData(data));
      this.process.stderr!.on('data', (data: Buffer) => {
        // gopls stderr is diagnostic info, log it
        const text = data.toString().trim();
        if (text) log(`[go] gopls stderr: ${text}`);
      });
      this.process.on('exit', (code) => {
        log(`[go] gopls exited with code ${code}`);
        this.initialized = false;
        this.rejectAllPending(new Error(`gopls exited with code ${code}`));
      });
      this.process.on('error', (err) => {
        log(`[go] gopls error: ${err.message}`);
        this.initialized = false;
      });

      // LSP initialize
      const initResult = await this.sendRequest('initialize', {
        processId: process.pid,
        rootUri: `file://${this.projectDir}`,
        capabilities: {
          textDocument: {
            completion: {
              completionItem: {
                snippetSupport: false,
                documentationFormat: ['markdown', 'plaintext'],
              },
            },
            hover: { contentFormat: ['markdown', 'plaintext'] },
            signatureHelp: { signatureInformation: { documentationFormat: ['markdown', 'plaintext'] } },
          },
        },
      });

      // LSP initialized notification
      this.sendNotification('initialized', {});
      log(`[go] gopls initialized, capabilities: ${JSON.stringify(initResult?.capabilities?.completionProvider ? 'completion' : 'none')}`);

      // Open the main file
      this.sendNotification('textDocument/didOpen', {
        textDocument: {
          uri: this.mainFileUri,
          languageId: 'go',
          version: this.fileVersion,
          text: 'package main\n',
        },
      });

      this.initialized = true;
      return true;
    } catch (err: any) {
      log(`[go] Failed to start gopls: ${err.message}`);
      this.kill();
      return false;
    }
  }

  /**
   * Update the assembled Go source. Writes to disk and notifies gopls.
   */
  async updateSource(assembled: AssembledSource, goImports?: Array<{ path: string; version: string }>): Promise<void> {
    if (!this.initialized) return;

    const text = assembled.text;
    if (text === this.currentText) return;

    this.currentText = text;
    this.fileVersion++;

    // Write the Go file
    fs.writeFileSync(this.mainFilePath, text);

    // Update go.mod if imports changed
    if (goImports && goImports.length > 0) {
      const nonStdImports = goImports.filter(i => i.version !== 'std');
      let goMod = 'module vortex/runtime\n\ngo 1.21\n';
      if (nonStdImports.length > 0) {
        goMod += '\nrequire (\n';
        for (const imp of nonStdImports) {
          goMod += `\t${imp.path} ${imp.version}\n`;
        }
        goMod += ')\n';
      }
      fs.writeFileSync(this.goModPath, goMod);
    }

    // Notify gopls of the change
    this.sendNotification('textDocument/didChange', {
      textDocument: {
        uri: this.mainFileUri,
        version: this.fileVersion,
      } as LspVersionedTextDocumentIdentifier,
      contentChanges: [{ text }],
    });
  }

  /**
   * Get completions at a position in the assembled Go source.
   */
  async getCompletions(line: number, character: number): Promise<LspCompletionItem[]> {
    if (!this.initialized) return [];

    try {
      const result = await this.sendRequest('textDocument/completion', {
        textDocument: { uri: this.mainFileUri },
        position: { line, character },
      }, 5000);

      if (!result) return [];
      if (Array.isArray(result)) return result;
      if (result.items) return result.items;
      return [];
    } catch (err: any) {
      log(`[go] completion error: ${err.message}`);
      return [];
    }
  }

  /**
   * Get hover info at a position.
   */
  async getHover(line: number, character: number): Promise<LspHover | null> {
    if (!this.initialized) return null;

    try {
      return await this.sendRequest('textDocument/hover', {
        textDocument: { uri: this.mainFileUri },
        position: { line, character },
      }, 5000);
    } catch (err: any) {
      log(`[go] hover error: ${err.message}`);
      return null;
    }
  }

  /**
   * Get definitions at a position.
   */
  async getDefinition(line: number, character: number): Promise<LspLocation[]> {
    if (!this.initialized) return [];

    try {
      const result = await this.sendRequest('textDocument/definition', {
        textDocument: { uri: this.mainFileUri },
        position: { line, character },
      }, 5000);

      if (!result) return [];
      if (Array.isArray(result)) return result;
      return [result];
    } catch (err: any) {
      log(`[go] definition error: ${err.message}`);
      return [];
    }
  }

  /**
   * Get signature help at a position.
   */
  async getSignatureHelp(line: number, character: number): Promise<LspSignatureHelp | null> {
    if (!this.initialized) return null;

    try {
      return await this.sendRequest('textDocument/signatureHelp', {
        textDocument: { uri: this.mainFileUri },
        position: { line, character },
      }, 5000);
    } catch (err: any) {
      log(`[go] signatureHelp error: ${err.message}`);
      return null;
    }
  }

  /**
   * Check if gopls is running and ready.
   */
  get isReady(): boolean {
    return this.initialized;
  }

  /**
   * Stop gopls and clean up temp files.
   */
  dispose(): void {
    this.kill();
    // Clean up temp directory
    try {
      const files = fs.readdirSync(this.projectDir);
      for (const f of files) {
        try { fs.unlinkSync(path.join(this.projectDir, f)); } catch { /* ignore */ }
      }
      fs.rmdirSync(this.projectDir);
    } catch { /* ignore */ }
  }

  // --- JSON-RPC transport ---

  private sendRequest(method: string, params: any, timeoutMs = 10000): Promise<any> {
    return new Promise((resolve, reject) => {
      if (!this.process?.stdin?.writable) {
        reject(new Error('gopls not running'));
        return;
      }

      const id = this.nextId++;
      const message = JSON.stringify({ jsonrpc: '2.0', id, method, params });
      const header = `Content-Length: ${Buffer.byteLength(message)}\r\n\r\n`;

      this.pendingRequests.set(id, { resolve, reject });
      this.process.stdin.write(header + message);

      // Timeout
      setTimeout(() => {
        if (this.pendingRequests.has(id)) {
          this.pendingRequests.delete(id);
          reject(new Error(`gopls request timed out: ${method}`));
        }
      }, timeoutMs);
    });
  }

  private sendNotification(method: string, params: any): void {
    if (!this.process?.stdin?.writable) return;
    const message = JSON.stringify({ jsonrpc: '2.0', method, params });
    const header = `Content-Length: ${Buffer.byteLength(message)}\r\n\r\n`;
    this.process.stdin.write(header + message);
  }

  private onData(data: Buffer): void {
    this.buffer += data.toString();

    while (true) {
      if (this.contentLength < 0) {
        const headerEnd = this.buffer.indexOf('\r\n\r\n');
        if (headerEnd < 0) return;

        const header = this.buffer.substring(0, headerEnd);
        const match = header.match(/Content-Length:\s*(\d+)/i);
        if (!match) {
          this.buffer = this.buffer.substring(headerEnd + 4);
          continue;
        }

        this.contentLength = parseInt(match[1], 10);
        this.buffer = this.buffer.substring(headerEnd + 4);
      }

      if (this.buffer.length < this.contentLength) return;

      const body = this.buffer.substring(0, this.contentLength);
      this.buffer = this.buffer.substring(this.contentLength);
      this.contentLength = -1;

      try {
        const msg = JSON.parse(body);
        if (msg.id !== undefined && msg.id !== null) {
          const pending = this.pendingRequests.get(msg.id);
          if (pending) {
            this.pendingRequests.delete(msg.id);
            if (msg.error) {
              pending.reject(new Error(`gopls error: ${msg.error.message}`));
            } else {
              pending.resolve(msg.result);
            }
          }
        }
        // Ignore notifications from server (diagnostics, etc.) for now
      } catch { /* ignore parse errors */ }
    }
  }

  private kill(): void {
    if (this.process) {
      try {
        this.sendNotification('shutdown', null);
        this.sendNotification('exit', null);
      } catch { /* ignore */ }

      setTimeout(() => {
        try { this.process?.kill(); } catch { /* ignore */ }
      }, 500);

      this.process = null;
    }
    this.initialized = false;
    this.rejectAllPending(new Error('gopls stopped'));
  }

  private rejectAllPending(err: Error): void {
    for (const [, p] of this.pendingRequests) {
      p.reject(err);
    }
    this.pendingRequests.clear();
  }

  private findGopls(): string | null {
    try {
      // Check if gopls is in PATH
      const result = cp.execFileSync('which', ['gopls'], {
        encoding: 'utf-8',
        timeout: 5000,
      }).trim();
      return result || null;
    } catch {
      // Try common locations
      const homeDir = os.homedir();
      const candidates = [
        path.join(homeDir, 'go', 'bin', 'gopls'),
        '/usr/local/go/bin/gopls',
        '/usr/local/bin/gopls',
      ];
      for (const c of candidates) {
        if (fs.existsSync(c)) return c;
      }
      return null;
    }
  }
}

/** Convert LSP CompletionItem to VS Code CompletionItem */
export function lspCompletionToVscode(
  item: LspCompletionItem,
  wordRange: vscode.Range,
): vscode.CompletionItem {
  const kind = item.kind ? (LSP_COMPLETION_KIND_MAP[item.kind] ?? vscode.CompletionItemKind.Text) : vscode.CompletionItemKind.Text;
  const result = new vscode.CompletionItem(item.label, kind);
  result.detail = item.detail;
  result.sortText = item.sortText;
  result.range = wordRange;

  if (item.documentation) {
    if (typeof item.documentation === 'string') {
      result.documentation = item.documentation;
    } else if (item.documentation.value) {
      result.documentation = new vscode.MarkdownString(item.documentation.value);
    }
  }

  if (item.insertText) {
    result.insertText = item.insertText;
  }

  return result;
}

/** Convert LSP Hover to VS Code Hover */
export function lspHoverToVscode(hover: LspHover): vscode.Hover {
  const parts: vscode.MarkdownString[] = [];

  if (typeof hover.contents === 'string') {
    parts.push(new vscode.MarkdownString(hover.contents));
  } else if (Array.isArray(hover.contents)) {
    for (const c of hover.contents) {
      if (typeof c === 'string') {
        parts.push(new vscode.MarkdownString(c));
      } else {
        parts.push(new vscode.MarkdownString().appendCodeblock(c.value, c.language));
      }
    }
  } else if (hover.contents && 'value' in hover.contents) {
    parts.push(new vscode.MarkdownString(hover.contents.value));
  }

  return new vscode.Hover(parts);
}

/** Convert LSP SignatureHelp to VS Code SignatureHelp */
export function lspSignatureHelpToVscode(sigHelp: LspSignatureHelp): vscode.SignatureHelp {
  const result = new vscode.SignatureHelp();
  result.activeSignature = sigHelp.activeSignature ?? 0;
  result.activeParameter = sigHelp.activeParameter ?? 0;

  for (const sig of sigHelp.signatures) {
    const info = new vscode.SignatureInformation(sig.label);
    if (sig.documentation) {
      if (typeof sig.documentation === 'string') {
        info.documentation = sig.documentation;
      } else {
        info.documentation = new vscode.MarkdownString(sig.documentation.value);
      }
    }
    if (sig.parameters) {
      for (const param of sig.parameters) {
        const paramLabel = typeof param.label === 'string' ? param.label : param.label;
        let paramDoc: string | vscode.MarkdownString | undefined;
        if (param.documentation) {
          if (typeof param.documentation === 'string') {
            paramDoc = param.documentation;
          } else {
            paramDoc = new vscode.MarkdownString(param.documentation.value);
          }
        }
        info.parameters.push(new vscode.ParameterInformation(paramLabel, paramDoc));
      }
    }
    result.signatures.push(info);
  }

  return result;
}
