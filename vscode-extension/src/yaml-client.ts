import * as vscode from 'vscode';
import * as path from 'path';
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
  TransportKind,
} from 'vscode-languageclient/node';
import { extractEmbeddedRegions } from './parser';
import { log } from './log';

let client: LanguageClient | null = null;

/**
 * Check if a position falls inside an embedded code region.
 */
function isInsideEmbeddedRegion(document: vscode.TextDocument, position: vscode.Position): boolean {
  try {
    const regions = extractEmbeddedRegions(document.getText());
    const line = position.line;
    const inside = regions.some(r => line >= r.startLine && line <= r.endLine);
    log(`[yaml] isInsideEmbeddedRegion line=${line} regions=${regions.length} inside=${inside}`);
    return inside;
  } catch (e) {
    log(`[yaml] isInsideEmbeddedRegion error: ${e}`);
    return false;
  }
}

/**
 * Start the bundled yaml-language-server for .vortex files.
 * Registers it for the 'vortex' language so it doesn't conflict with
 * the Red Hat YAML extension (which handles 'yaml' language).
 */
export async function startYamlLanguageServer(
  context: vscode.ExtensionContext,
  schemaUrl: string
): Promise<void> {
  // Resolve the bundled yaml-language-server (webpack-bundled standalone file)
  const serverModule = context.asAbsolutePath(
    path.join('out', 'yaml-server.js')
  );

  const serverOptions: ServerOptions = {
    run: { module: serverModule, transport: TransportKind.ipc },
    debug: { module: serverModule, transport: TransportKind.ipc },
  };

  const clientOptions: LanguageClientOptions = {
    documentSelector: [{ language: 'vortex', scheme: 'file' }],
    synchronize: {
      fileEvents: vscode.workspace.createFileSystemWatcher('**/*.vortex'),
    },
    middleware: {
      provideHover: async (document, position, token, next) => {
        if (isInsideEmbeddedRegion(document, position)) return undefined;
        const hover = await next(document, position, token);
        if (!hover) return hover;
        return filterHoverContent(hover);
      },
      provideCompletionItem: async (document, position, context, token, next) => {
        const inside = isInsideEmbeddedRegion(document, position);
        log(`[yaml] provideCompletionItem called, inside=${inside}`);
        if (inside) return [];
        return next(document, position, context, token);
      },
      provideSignatureHelp: async (document, position, context, token, next) => {
        if (isInsideEmbeddedRegion(document, position)) return undefined;
        return next(document, position, context, token);
      },
      provideDocumentHighlights: () => {
        return Promise.resolve([]);
      },
      provideCodeLenses: () => {
        // Suppress schema-association CodeLens from the yaml-language-server
        return Promise.resolve([]);
      },
    },
    initializationOptions: {
      schemas: {
        [schemaUrl]: '*.vortex',
      },
      hover: true,
      completion: true,
      validate: true,
      format: { enable: true },
      schemaStore: { enable: false },
    },
  };

  client = new LanguageClient(
    'vortex-yaml',
    'Vortex YAML Language Server',
    serverOptions,
    clientOptions
  );

  // Remove the ExecuteCommandFeature to prevent our yaml client from registering
  // commands like 'jumpToSchema' that conflict with the Red Hat YAML extension.
  // Also remove DocumentHighlightFeature since we suppress highlights via middleware.
  const features = (client as any)._features as any[] | undefined;
  if (features) {
    const removeTypes = ['workspace/executeCommand', 'textDocument/documentHighlight'];
    for (let i = features.length - 1; i >= 0; i--) {
      const reg = features[i]?.registrationType;
      if (reg && removeTypes.includes(reg.method)) {
        features.splice(i, 1);
      }
    }
  }

  await client.start();

  // Send schema association after the server is ready
  client.sendNotification('yaml/registerCustomSchemaRequest');
}

/**
 * Filter hover content to remove the "Source: ..." attribution line.
 */
function filterHoverContent(hover: vscode.Hover): vscode.Hover {
  const filtered = hover.contents.map(content => {
    if (typeof content === 'string') {
      return removeSourceLine(content);
    }
    if (content instanceof vscode.MarkdownString) {
      const cleaned = removeSourceLine(content.value);
      const md = new vscode.MarkdownString(cleaned);
      md.isTrusted = content.isTrusted;
      md.supportHtml = content.supportHtml;
      return md;
    }
    return content;
  }).filter(c => {
    // Remove empty content after filtering
    if (typeof c === 'string') return c.trim().length > 0;
    if (c instanceof vscode.MarkdownString) return c.value.trim().length > 0;
    return true;
  });

  // Strip the range — prevents VS Code from highlighting the entire block scalar in blue
  return new vscode.Hover(filtered);
}

function removeSourceLine(text: string): string {
  // Remove lines like "Source: vortex.schema.json" or "Source: [vortex.schema.json](...)"
  return text
    .split('\n')
    .filter(line => !line.match(/^Source:\s/i))
    .join('\n')
    .trim();
}

export async function stopYamlLanguageServer(): Promise<void> {
  if (client) {
    await client.stop();
    client = null;
  }
}
