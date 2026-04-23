import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';
import * as cp from 'child_process';
import { log } from './log';
import type { AssembledSource } from './assembler';

/**
 * Roslyn LSP client for C# intellisense in .vortex files.
 *
 * Spawns the Roslyn language server (from the C# extension) with a
 * temporary .NET project directory containing .csproj + Program.cs.
 * Communicates via JSON-RPC over stdio, same as the gopls client.
 */

// LSP message types (subset we need — shared with gopls-client)
interface LspPosition { line: number; character: number; }
interface LspRange { start: LspPosition; end: LspPosition; }
interface LspLocation { uri: string; range: LspRange; }
interface LspCompletionItem {
  label: string;
  kind?: number;
  detail?: string;
  documentation?: string | { kind: string; value: string };
  insertText?: string;
  textEdit?: { range: LspRange; newText: string };
  sortText?: string;
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

// LSP completion item kinds → VS Code kinds
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

export class RoslynClient {
  private process: cp.ChildProcess | null = null;
  private projectDir: string;
  private programFilePath: string;
  private programFileUri: string;
  private csprojPath: string;
  private initialized = false;
  private nextId = 1;
  private pendingRequests = new Map<number, { resolve: (v: any) => void; reject: (e: any) => void }>();
  private buffer = '';
  private contentLength = -1;
  private fileVersion = 0;
  private currentText = '';
  private startPromise: Promise<boolean> | null = null;

  constructor() {
    this.projectDir = path.join(os.tmpdir(), 'vortex-roslyn-' + process.pid);
    this.programFilePath = path.join(this.projectDir, 'Program.cs');
    this.programFileUri = `file://${this.programFilePath}`;
    this.csprojPath = path.join(this.projectDir, 'project.csproj');
  }

  /**
   * Start Roslyn and initialize the LSP connection.
   * Returns true if successful, false if Roslyn is not available.
   */
  async start(): Promise<boolean> {
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
    const serverPath = this.findRoslynServer();
    if (!serverPath) {
      log('[csharp] Roslyn language server not found');
      return false;
    }

    // Create project directory with a minimal .csproj
    fs.mkdirSync(this.projectDir, { recursive: true });
    this.writeCsproj('net8.0', []);
    fs.writeFileSync(this.programFilePath, '// placeholder\n');

    log(`[csharp] Starting Roslyn from ${serverPath}, project at ${this.projectDir}`);

    try {
      this.process = cp.spawn(serverPath, [
        '--stdio',
        '--logLevel', 'Warning',
        '--telemetryLevel', 'off',
      ], {
        stdio: ['pipe', 'pipe', 'pipe'],
        cwd: this.projectDir,
      });

      this.process.stdout!.on('data', (data: Buffer) => this.onData(data));
      this.process.stderr!.on('data', (data: Buffer) => {
        const text = data.toString().trim();
        if (text) log(`[csharp] roslyn stderr: ${text}`);
      });
      this.process.on('exit', (code) => {
        log(`[csharp] Roslyn exited with code ${code}`);
        this.initialized = false;
        this.rejectAllPending(new Error(`Roslyn exited with code ${code}`));
      });
      this.process.on('error', (err) => {
        log(`[csharp] Roslyn error: ${err.message}`);
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
            signatureHelp: {
              signatureInformation: {
                documentationFormat: ['markdown', 'plaintext'],
              },
            },
          },
        },
      }, 30000); // Roslyn can be slow on first start

      this.sendNotification('initialized', {});
      log(`[csharp] Roslyn initialized`);

      // Open the program file
      this.sendNotification('textDocument/didOpen', {
        textDocument: {
          uri: this.programFileUri,
          languageId: 'csharp',
          version: this.fileVersion,
          text: '// placeholder\n',
        },
      });

      // Give Roslyn time to load the project — requests sent immediately
      // after init get "The task was cancelled" errors.
      await new Promise(resolve => setTimeout(resolve, 2000));

      this.initialized = true;
      return true;
    } catch (err: any) {
      log(`[csharp] Failed to start Roslyn: ${err.message}`);
      this.kill();
      return false;
    }
  }

  /**
   * Update the assembled C# source. Writes to disk and notifies Roslyn.
   */
  async updateSource(
    assembled: AssembledSource,
    framework?: string,
    packages?: Array<{ name: string; version: string }>,
  ): Promise<void> {
    if (!this.initialized) return;

    const text = assembled.text;
    if (text === this.currentText) return;

    const previousText = this.currentText;
    this.currentText = text;

    // Update .csproj if framework/packages changed
    this.writeCsproj(framework || 'net8.0', packages || []);

    // Write the C# file
    fs.writeFileSync(this.programFilePath, text);

    // Send didChange with an explicit range covering the full document.
    // Roslyn crashes on rangeless full-content replacement (NullReferenceException
    // in DidChangeHandler), but handles ranged replacement fine.
    const prevLines = previousText.split('\n');
    const lastLine = Math.max(0, prevLines.length - 1);
    const lastChar = prevLines[lastLine]?.length ?? 0;

    this.fileVersion++;
    this.sendNotification('textDocument/didChange', {
      textDocument: { uri: this.programFileUri, version: this.fileVersion },
      contentChanges: [{
        range: {
          start: { line: 0, character: 0 },
          end: { line: lastLine, character: lastChar },
        },
        text,
      }],
    });
  }

  async getCompletions(line: number, character: number): Promise<LspCompletionItem[]> {
    if (!this.initialized) return [];
    try {
      const result = await this.sendRequest('textDocument/completion', {
        textDocument: { uri: this.programFileUri },
        position: { line, character },
      }, 10000);
      if (!result) return [];
      if (Array.isArray(result)) return result;
      if (result.items) return result.items;
      return [];
    } catch (err: any) {
      log(`[csharp] completion error: ${err.message}`);
      return [];
    }
  }

  async getHover(line: number, character: number): Promise<LspHover | null> {
    if (!this.initialized) return null;
    try {
      return await this.sendRequest('textDocument/hover', {
        textDocument: { uri: this.programFileUri },
        position: { line, character },
      }, 10000);
    } catch (err: any) {
      log(`[csharp] hover error: ${err.message}`);
      return null;
    }
  }

  async getDefinition(line: number, character: number): Promise<LspLocation[]> {
    if (!this.initialized) return [];
    try {
      const result = await this.sendRequest('textDocument/definition', {
        textDocument: { uri: this.programFileUri },
        position: { line, character },
      }, 10000);
      if (!result) return [];
      if (Array.isArray(result)) return result;
      return [result];
    } catch (err: any) {
      log(`[csharp] definition error: ${err.message}`);
      return [];
    }
  }

  async getSignatureHelp(line: number, character: number): Promise<LspSignatureHelp | null> {
    if (!this.initialized) return null;
    try {
      return await this.sendRequest('textDocument/signatureHelp', {
        textDocument: { uri: this.programFileUri },
        position: { line, character },
      }, 10000);
    } catch (err: any) {
      log(`[csharp] signatureHelp error: ${err.message}`);
      return null;
    }
  }

  get isReady(): boolean {
    return this.initialized;
  }

  dispose(): void {
    this.kill();
    try {
      const files = fs.readdirSync(this.projectDir);
      for (const f of files) {
        try { fs.unlinkSync(path.join(this.projectDir, f)); } catch { /* ignore */ }
      }
      // Clean up obj/bin subdirs
      for (const sub of ['obj', 'bin']) {
        const subDir = path.join(this.projectDir, sub);
        try { fs.rmSync(subDir, { recursive: true, force: true }); } catch { /* ignore */ }
      }
      fs.rmdirSync(this.projectDir);
    } catch { /* ignore */ }
  }

  // --- Project file generation ---

  private writeCsproj(
    framework: string,
    packages: Array<{ name: string; version: string }>,
  ): void {
    let csproj = `<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <OutputType>Exe</OutputType>
    <TargetFramework>${framework}</TargetFramework>
    <ImplicitUsings>enable</ImplicitUsings>
  </PropertyGroup>
`;
    if (packages.length > 0) {
      csproj += '  <ItemGroup>\n';
      for (const pkg of packages) {
        csproj += `    <PackageReference Include="${pkg.name}" Version="${pkg.version}" />\n`;
      }
      csproj += '  </ItemGroup>\n';
    }
    csproj += '</Project>\n';

    // Only write if changed to avoid triggering unnecessary restores
    try {
      const existing = fs.readFileSync(this.csprojPath, 'utf-8');
      if (existing === csproj) return;
    } catch { /* file doesn't exist yet */ }

    fs.writeFileSync(this.csprojPath, csproj);
  }

  // --- JSON-RPC transport (same protocol as gopls) ---

  private sendRequest(method: string, params: any, timeoutMs = 15000): Promise<any> {
    return new Promise((resolve, reject) => {
      if (!this.process?.stdin?.writable) {
        reject(new Error('Roslyn not running'));
        return;
      }

      const id = this.nextId++;
      const message = JSON.stringify({ jsonrpc: '2.0', id, method, params });
      const header = `Content-Length: ${Buffer.byteLength(message)}\r\n\r\n`;

      this.pendingRequests.set(id, { resolve, reject });
      this.process.stdin.write(header + message);

      setTimeout(() => {
        if (this.pendingRequests.has(id)) {
          this.pendingRequests.delete(id);
          reject(new Error(`Roslyn request timed out: ${method}`));
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
              pending.reject(new Error(`Roslyn error: ${msg.error.message}`));
            } else {
              pending.resolve(msg.result);
            }
          }
        }
        // Ignore server notifications (diagnostics, etc.)
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
      }, 1000);

      this.process = null;
    }
    this.initialized = false;
    this.rejectAllPending(new Error('Roslyn stopped'));
  }

  private rejectAllPending(err: Error): void {
    for (const [, p] of this.pendingRequests) {
      p.reject(err);
    }
    this.pendingRequests.clear();
  }

  /**
   * Find the Roslyn language server binary.
   * Searches in the C# extension bundled with VS Code.
   */
  private findRoslynServer(): string | null {
    const homeDir = os.homedir();

    // Search for the C# extension's bundled Roslyn server
    const extensionsDir = path.join(homeDir, '.vscode', 'extensions');
    try {
      const entries = fs.readdirSync(extensionsDir);
      // Sort descending to get the latest version first
      const csharpExts = entries
        .filter(e => e.startsWith('ms-dotnettools.csharp-'))
        .sort()
        .reverse();

      for (const ext of csharpExts) {
        const roslynDir = path.join(extensionsDir, ext, '.roslyn');
        // Native binary (self-contained, e.g. darwin-arm64)
        const nativePath = path.join(roslynDir, 'Microsoft.CodeAnalysis.LanguageServer');
        if (fs.existsSync(nativePath)) {
          try {
            fs.accessSync(nativePath, fs.constants.X_OK);
            return nativePath;
          } catch { /* not executable */ }
        }
        // .dll fallback (needs dotnet to run)
        const dllPath = path.join(roslynDir, 'Microsoft.CodeAnalysis.LanguageServer.dll');
        if (fs.existsSync(dllPath)) {
          // Return the dll path — caller will need to run via dotnet
          return dllPath;
        }
      }
    } catch { /* ignore */ }

    // Also check VS Code Insiders
    const insidersDir = path.join(homeDir, '.vscode-insiders', 'extensions');
    try {
      const entries = fs.readdirSync(insidersDir);
      const csharpExts = entries
        .filter(e => e.startsWith('ms-dotnettools.csharp-'))
        .sort()
        .reverse();

      for (const ext of csharpExts) {
        const roslynDir = path.join(insidersDir, ext, '.roslyn');
        const nativePath = path.join(roslynDir, 'Microsoft.CodeAnalysis.LanguageServer');
        if (fs.existsSync(nativePath)) {
          try {
            fs.accessSync(nativePath, fs.constants.X_OK);
            return nativePath;
          } catch { /* not executable */ }
        }
      }
    } catch { /* ignore */ }

    log('[csharp] No Roslyn server found in C# extension');
    return null;
  }
}

// --- Conversion helpers (exported for intellisense.ts) ---

export function roslynCompletionToVscode(
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

export function roslynHoverToVscode(hover: LspHover): vscode.Hover {
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

export function roslynSignatureHelpToVscode(sigHelp: LspSignatureHelp): vscode.SignatureHelp {
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
