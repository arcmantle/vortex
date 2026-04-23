import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';
import { createHighlighterCore, type HighlighterCore, type ThemedToken } from 'shiki/core';
import { createOnigurumaEngine } from 'shiki/engine/oniguruma';
import { extractEmbeddedRegions, type EmbeddedRegion } from './parser';
import { VortexIntellisenseProvider } from './intellisense';
import { startYamlLanguageServer, stopYamlLanguageServer } from './yaml-client';
import { log } from './log';

// Fine-grained language imports
import langJs from 'shiki/langs/javascript.mjs';
import langGo from 'shiki/langs/go.mjs';
import langCsharp from 'shiki/langs/csharp.mjs';

// Fallback themes (used only if we can't load the user's theme)
import themeDarkFallback from 'shiki/themes/github-dark-dimmed.mjs';
import themeLightFallback from 'shiki/themes/github-light.mjs';

/** Check if a document is a .vortex file (by extension, not languageId). */
function isVortexFile(document: vscode.TextDocument): boolean {
  return document.uri.fsPath.endsWith('.vortex');
}

/** Cache parsed regions per document version. */
const regionCache = new Map<string, { version: number; regions: EmbeddedRegion[] }>();

function getRegions(document: vscode.TextDocument): EmbeddedRegion[] {
  const key = document.uri.toString();
  const cached = regionCache.get(key);
  if (cached && cached.version === document.version) {
    return cached.regions;
  }
  const regions = extractEmbeddedRegions(document.getText());
  regionCache.set(key, { version: document.version, regions });
  return regions;
}

const LANG_MAP: Record<string, string> = {
  javascript: 'javascript',
  go: 'go',
  csharp: 'csharp',
};

/** Decoration type cache: color+style key -> decoration type. */
const decoTypeCache = new Map<string, vscode.TextEditorDecorationType>();

const FONT_ITALIC = 1;
const FONT_BOLD = 2;
const FONT_UNDERLINE = 4;

function getDecoType(color: string, fontStyle?: number): vscode.TextEditorDecorationType {
  const key = `${color}|${fontStyle || 0}`;
  let deco = decoTypeCache.get(key);
  if (!deco) {
    const options: vscode.DecorationRenderOptions = { color };
    if (fontStyle && (fontStyle & FONT_ITALIC)) options.fontStyle = 'italic';
    if (fontStyle && (fontStyle & FONT_BOLD)) options.fontWeight = 'bold';
    if (fontStyle && (fontStyle & FONT_UNDERLINE)) options.textDecoration = 'underline';
    deco = vscode.window.createTextEditorDecorationType(options);
    decoTypeCache.set(key, deco);
  }
  return deco;
}

// --- Theme discovery ---

const VORTEX_THEME_NAME = 'vortex-user-theme';

/**
 * Find and load the user's current VS Code color theme as a shiki-compatible
 * theme object. Searches through installed extensions for the theme JSON.
 */
/**
 * Strip JSONC (JSON with Comments) to valid JSON.
 * Handles // and /* comments only outside of string literals,
 * and removes trailing commas before } or ].
 */
function stripJsonc(text: string): string {
  let result = '';
  let i = 0;
  while (i < text.length) {
    // String literal — copy verbatim
    if (text[i] === '"') {
      let j = i + 1;
      while (j < text.length && text[j] !== '"') {
        if (text[j] === '\\') j++; // skip escaped char
        j++;
      }
      result += text.slice(i, j + 1);
      i = j + 1;
    // Line comment
    } else if (text[i] === '/' && text[i + 1] === '/') {
      const nl = text.indexOf('\n', i);
      i = nl === -1 ? text.length : nl;
    // Block comment
    } else if (text[i] === '/' && text[i + 1] === '*') {
      const end = text.indexOf('*/', i + 2);
      i = end === -1 ? text.length : end + 2;
    } else {
      result += text[i];
      i++;
    }
  }
  // Remove trailing commas
  return result.replace(/,(\s*[}\]])/g, '$1');
}

function loadCurrentVSCodeTheme(): any | null {
  const themeName = vscode.workspace.getConfiguration('workbench').get<string>('colorTheme');
  if (!themeName) return null;

  for (const ext of vscode.extensions.all) {
    const themes = ext.packageJSON?.contributes?.themes;
    if (!Array.isArray(themes)) continue;

    for (const themeEntry of themes) {
      if (themeEntry.label === themeName || themeEntry.id === themeName) {
        const themePath = path.join(ext.extensionPath, themeEntry.path);
        try {
          const raw = fs.readFileSync(themePath, 'utf-8');
          const themeData = JSON.parse(stripJsonc(raw));

          // Resolve "include" one level deep
          if (themeData.include) {
            const includePath = path.join(path.dirname(themePath), themeData.include);
            try {
              const includeRaw = fs.readFileSync(includePath, 'utf-8');
              const includeData = JSON.parse(stripJsonc(includeRaw));
              themeData.colors = { ...includeData.colors, ...themeData.colors };
              if (includeData.tokenColors && !themeData.tokenColors) {
                themeData.tokenColors = includeData.tokenColors;
              } else if (includeData.tokenColors && themeData.tokenColors) {
                themeData.tokenColors = [...includeData.tokenColors, ...themeData.tokenColors];
              }
            } catch { /* ignore include errors */ }
          }

          // Normalize to shiki format
          themeData.name = VORTEX_THEME_NAME;
          if (!themeData.type) {
            const kind = vscode.window.activeColorTheme.kind;
            themeData.type = (kind === 1 || kind === 4) ? 'light' : 'dark';
          }

          return themeData;
        } catch {
          return null;
        }
      }
    }
  }
  return null;
}

// --- Highlighter management ---

let highlighter: HighlighterCore | null = null;
let highlighterPromise: Promise<HighlighterCore | null> | null = null;
let loadedThemeName: string | null = null;

async function ensureHighlighter(): Promise<HighlighterCore | null> {
  if (highlighter) return highlighter;
  if (highlighterPromise) return highlighterPromise;

  highlighterPromise = (async () => {
    try {
      const userTheme = loadCurrentVSCodeTheme();
      const themes: any[] = [];

      if (userTheme) {
        themes.push(userTheme);
        loadedThemeName = VORTEX_THEME_NAME;
      } else {
        themes.push(themeDarkFallback, themeLightFallback);
        const kind = vscode.window.activeColorTheme.kind;
        loadedThemeName = (kind === 1 || kind === 4)
		  	? 'github-light'
			: 'github-dark-dimmed';
      }

      const hl = await createHighlighterCore({
        engine: await createOnigurumaEngine(import('shiki/wasm')),
        themes,
        langs: [langJs, langGo, langCsharp],
      });
      highlighter = hl;
      return hl;
    } catch (e) {
      console.error('Vortex: Failed to create highlighter:', e);
      return null;
    }
  })();

  return highlighterPromise;
}

async function reloadHighlighter(): Promise<void> {
  if (highlighter) {
    highlighter.dispose();
    highlighter = null;
  }
  highlighterPromise = null;
  loadedThemeName = null;
  await ensureHighlighter();
}

function getActiveThemeName(): string {
  return loadedThemeName || 'github-dark-dimmed';
}

// --- Highlighting ---

async function applyHighlighting(editor: vscode.TextEditor): Promise<void> {
  const document = editor.document;
  if (!isVortexFile(document)) return;

  const hl = await ensureHighlighter();
  if (!hl) return;

  const regions = getRegions(document);
  if (regions.length === 0) {
    clearDecorations(editor);
    return;
  }

  const theme = getActiveThemeName();
  const decoRanges = new Map<vscode.TextEditorDecorationType, vscode.Range[]>();

  for (const region of regions) {
    const lang = LANG_MAP[region.languageId];
    if (!lang) continue;

    let tokenLines: ThemedToken[][];
    try {
      const result = hl.codeToTokens(region.text, { lang, theme });
      tokenLines = result.tokens;
    } catch {
      continue;
    }

    const textLines = region.text.split('\n');
    let lineStartOffset = 0;

    for (let lineIdx = 0; lineIdx < tokenLines.length; lineIdx++) {
      const docLine = region.startLine + lineIdx;
      if (docLine > region.endLine) break;

      const tokens = tokenLines[lineIdx];
      for (const token of tokens) {
        if (!token.color) continue;

        const localOffset = token.offset - lineStartOffset;
        const startCol = region.indent + localOffset;
        const endCol = startCol + token.content.length;

        const range = new vscode.Range(docLine, startCol, docLine, endCol);
        const decoType = getDecoType(token.color, token.fontStyle);

        let ranges = decoRanges.get(decoType);
        if (!ranges) {
          ranges = [];
          decoRanges.set(decoType, ranges);
        }
        ranges.push(range);
      }

      lineStartOffset += (textLines[lineIdx]?.length ?? 0) + 1;
    }
  }

  for (const [decoType, ranges] of decoRanges) {
    editor.setDecorations(decoType, ranges);
  }

  for (const [, decoType] of decoTypeCache) {
    if (!decoRanges.has(decoType)) {
      editor.setDecorations(decoType, []);
    }
  }
}

function clearDecorations(editor: vscode.TextEditor): void {
  for (const [, decoType] of decoTypeCache) {
    editor.setDecorations(decoType, []);
  }
}

function debounce(fn: () => void, ms: number): () => void {
  let timer: ReturnType<typeof setTimeout> | undefined;
  return () => {
    if (timer) clearTimeout(timer);
    timer = setTimeout(fn, ms);
  };
}

let intellisenseProvider: VortexIntellisenseProvider | null = null;

const SCHEMA_URL = 'https://raw.githubusercontent.com/arcmantle/vortex/master/schemas/vortex.schema.json';

export function activate(context: vscode.ExtensionContext): void {
  log('Vortex extension activating');
  // Force .vortex files to 'vortex' language mode so the Red Hat YAML extension
  // doesn't interfere. Our bundled yaml-language-server handles YAML intellisense.
  function ensureVortexLanguage(doc: vscode.TextDocument): void {
    if (isVortexFile(doc) && doc.languageId !== 'vortex') {
      vscode.languages.setTextDocumentLanguage(doc, 'vortex');
    }
  }

  /** Force 2-space indentation for .vortex editors (overrides detectIndentation). */
  function ensureVortexEditorOptions(editor: vscode.TextEditor): void {
    if (isVortexFile(editor.document)) {
      editor.options = {
        insertSpaces: true,
        tabSize: 2,
      };
    }
  }

  const triggerHighlight = debounce(() => {
    const editor = vscode.window.activeTextEditor;
    if (editor && isVortexFile(editor.document)) {
      applyHighlighting(editor);
    }
  }, 150);

  // --- YAML Language Server ---
  startYamlLanguageServer(context, SCHEMA_URL).catch(e => {
    console.error('Vortex: Failed to start YAML language server:', e);
  });

  // --- Intellisense ---
  intellisenseProvider = new VortexIntellisenseProvider();

  const vortexSelector: vscode.DocumentSelector = [
    { language: 'vortex', scheme: 'file' },
  ];

  context.subscriptions.push(
    intellisenseProvider,
    vscode.languages.registerHoverProvider(vortexSelector, intellisenseProvider),
    vscode.languages.registerCompletionItemProvider(vortexSelector, intellisenseProvider, '.', '(', "'", '"'),
    vscode.languages.registerDefinitionProvider(vortexSelector, intellisenseProvider),
    vscode.languages.registerSignatureHelpProvider(vortexSelector, intellisenseProvider, '(', ','),
    vscode.languages.registerDocumentHighlightProvider(vortexSelector, intellisenseProvider),
    vscode.commands.registerCommand('vortex.viewAssembledSource', () => {
      const editor = vscode.window.activeTextEditor;
      if (editor && isVortexFile(editor.document)) {
        intellisenseProvider!.viewAssembledSource(editor.document);
      }
    }),
  );
  log('Registered intellisense providers (JS + Go + C#) for language: vortex');

  // --- Highlighting & Language Mode ---
  context.subscriptions.push(
    vscode.window.onDidChangeActiveTextEditor(editor => {
      if (editor && isVortexFile(editor.document)) {
        ensureVortexLanguage(editor.document);
        ensureVortexEditorOptions(editor);
        applyHighlighting(editor);
      }
    }),
    vscode.workspace.onDidOpenTextDocument(doc => {
      ensureVortexLanguage(doc);
    }),
    vscode.workspace.onDidChangeTextDocument(e => {
      if (isVortexFile(e.document)) {
        regionCache.delete(e.document.uri.toString());
        triggerHighlight();
        intellisenseProvider?.handleDocumentChange(e.document);
      }
    }),
    vscode.window.onDidChangeActiveColorTheme(async () => {
      regionCache.clear();
      for (const [, deco] of decoTypeCache) {
        deco.dispose();
      }
      decoTypeCache.clear();
      await reloadHighlighter();
      triggerHighlight();
    }),
    vscode.workspace.onDidCloseTextDocument(doc => {
      regionCache.delete(doc.uri.toString());
      intellisenseProvider?.removeDocument(doc.uri);
    })
  );

  // Initial setup for active editor
  const editor = vscode.window.activeTextEditor;
  if (editor && isVortexFile(editor.document)) {
    ensureVortexLanguage(editor.document);
    ensureVortexEditorOptions(editor);
    applyHighlighting(editor);
  }
}

export async function deactivate(): Promise<void> {
  regionCache.clear();
  for (const [, deco] of decoTypeCache) {
    deco.dispose();
  }
  decoTypeCache.clear();
  if (highlighter) {
    highlighter.dispose();
    highlighter = null;
  }
  if (intellisenseProvider) {
    intellisenseProvider.dispose();
    intellisenseProvider = null;
  }
  await stopYamlLanguageServer();
}
