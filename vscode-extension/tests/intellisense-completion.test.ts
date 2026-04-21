import { describe, it, expect } from 'vitest';
import * as ts from 'typescript';
import * as path from 'path';
import { readFileSync } from 'fs';
import { assembleJsSource } from '../src/assembler';

/**
 * These tests prove that the assembled JS source is correct and that
 * TypeScript's language service can provide member completions (e.g.
 * ChildProcess properties after `child.`) when given the assembled content.
 *
 * This is the same pipeline that the VS Code extension uses, but without
 * the VS Code API layer — so we can test it in isolation.
 */

const VORTEX_SOURCE = `name: test-app

node:
  imports:
    - from: node:child_process
      names: [ spawn ]
    - from: node:path
      default: path

  functions:
    run: |
      export function run(spec) {
        const child = spawn(spec.command, spec.args ?? [], {
          cwd: path.resolve(process.cwd(), spec.cwd ?? "."),
          stdio: "inherit",
        });
        child.

      }

jobs:
  - id: build
    shell: node
    use: node
    command: |
      run({
        command: "dotnet",
        args: ["build"],
      });
`;

function createLanguageService(fileName: string, content: string): ts.LanguageService {
  const host: ts.LanguageServiceHost = {
    getScriptFileNames: () => [fileName],
    getScriptVersion: () => '1',
    getScriptSnapshot: (name) => {
      if (name === fileName) {
        return ts.ScriptSnapshot.fromString(content);
      }
      try {
        return ts.ScriptSnapshot.fromString(readFileSync(name, 'utf-8'));
      } catch {
        return undefined;
      }
    },
    getCurrentDirectory: () => path.resolve(process.cwd()),
    getCompilationSettings: () => ({
      target: ts.ScriptTarget.ES2022,
      module: ts.ModuleKind.ES2022,
      moduleResolution: ts.ModuleResolutionKind.Bundler,
      allowJs: true,
      checkJs: true,
      strict: false,
      noEmit: true,
    }),
    getDefaultLibFileName: ts.getDefaultLibFilePath,
    fileExists: ts.sys.fileExists,
    readFile: ts.sys.readFile,
    readDirectory: ts.sys.readDirectory,
    directoryExists: ts.sys.directoryExists,
    getDirectories: ts.sys.getDirectories,
  };
  return ts.createLanguageService(host, ts.createDocumentRegistry());
}

/** Convert line+col to byte offset within a text string. */
function getOffset(text: string, line: number, col: number): number {
  const lines = text.split('\n');
  let offset = 0;
  for (let i = 0; i < line; i++) {
    offset += lines[i].length + 1; // +1 for newline
  }
  return offset + col;
}

describe('intellisense completions via TypeScript language service', () => {

  it('assembles correctly and maps child. position to assembled source', () => {
    const result = assembleJsSource(VORTEX_SOURCE)!;
    expect(result).not.toBeNull();

    const vortexLines = VORTEX_SOURCE.split('\n');
    const childDotLine = vortexLines.findIndex(l => l.trim() === 'child.');
    expect(childDotLine).toBeGreaterThan(0);

    // Position after the dot
    const dotCol = vortexLines[childDotLine].indexOf('child.') + 'child.'.length;
    const mapped = result.sourceMap.toAssembled(childDotLine, dotCol);
    expect(mapped).not.toBeNull();

    // Verify the assembled line contains "child." at the right position
    const assembledLines = result.text.split('\n');
    const assembledLine = assembledLines[mapped!.line];
    expect(assembledLine.trim()).toBe('child.');
    // The 6 chars before the cursor (mapped!.col) should be "child."
    expect(assembledLine.substring(mapped!.col - 6, mapped!.col)).toBe('child.');
  });

  it('provides member completions for child. (ChildProcess properties)', () => {
    const result = assembleJsSource(VORTEX_SOURCE)!;
    const fileName = path.resolve(process.cwd(), '_test_assembled.js');
    const service = createLanguageService(fileName, result.text);

    // Map vortex position to assembled position
    const vortexLines = VORTEX_SOURCE.split('\n');
    const childDotLine = vortexLines.findIndex(l => l.trim() === 'child.');
    const dotCol = vortexLines[childDotLine].indexOf('child.') + 'child.'.length;
    const mapped = result.sourceMap.toAssembled(childDotLine, dotCol)!;

    // Get completions at the position after the dot
    const offset = getOffset(result.text, mapped.line, mapped.col);
    const completions = service.getCompletionsAtPosition(fileName, offset, {});

    expect(completions).not.toBeNull();
    const names = completions!.entries.map(e => e.name);

    console.log(`Got ${names.length} completions at child.`);
    console.log('First 30:', names.slice(0, 30).join(', '));

    // ChildProcess member properties — these prove the TS service resolved
    // the type of `spawn()` return value correctly
    expect(names).toContain('on');
    expect(names).toContain('kill');
    expect(names).toContain('pid');
    expect(names).toContain('stdin');
    expect(names).toContain('stdout');
    expect(names).toContain('stderr');
    expect(names).toContain('addListener');
    expect(names).toContain('removeListener');

    // Member completions should be significantly fewer than global completions
    // (globals are typically 1000+ items)
    expect(names.length).toBeLessThan(500);
  });

  it('provides global completions at c| (before dot, not member access)', () => {
    const result = assembleJsSource(VORTEX_SOURCE)!;
    const fileName = path.resolve(process.cwd(), '_test_assembled.js');
    const service = createLanguageService(fileName, result.text);

    // Map to position after 'c' (not after the dot)
    const vortexLines = VORTEX_SOURCE.split('\n');
    const childDotLine = vortexLines.findIndex(l => l.trim() === 'child.');
    const cCol = vortexLines[childDotLine].indexOf('child.') + 1; // after 'c'
    const mapped = result.sourceMap.toAssembled(childDotLine, cCol)!;

    const offset = getOffset(result.text, mapped.line, mapped.col);
    const completions = service.getCompletionsAtPosition(fileName, offset, {});

    expect(completions).not.toBeNull();
    const names = completions!.entries.map(e => e.name);

    console.log(`Got ${names.length} completions at c|`);
    console.log('First 20:', names.slice(0, 20).join(', '));

    // Global scope — should have lots of completions
    expect(names).toContain('child');
    expect(names).toContain('console');
    expect(names.length).toBeGreaterThan(100);
  });

  it('demonstrates the bug: wrong position gives wrong completions', () => {
    // This test shows what happens when the TS service gets the WRONG
    // position or stale content — it returns globals instead of members.
    const result = assembleJsSource(VORTEX_SOURCE)!;
    const fileName = path.resolve(process.cwd(), '_test_assembled.js');

    // Simulate what happens when TS service has WRONG content:
    // give it just "  c" at the line where "  child." should be
    const assembledLines = result.text.split('\n');
    const vortexLines = VORTEX_SOURCE.split('\n');
    const childDotLine = vortexLines.findIndex(l => l.trim() === 'child.');
    const dotCol = vortexLines[childDotLine].indexOf('child.') + 'child.'.length;
    const mapped = result.sourceMap.toAssembled(childDotLine, dotCol)!;

    // Replace the "  child." line with "  c" to simulate stale content
    const brokenLines = [...assembledLines];
    brokenLines[mapped.line] = '  c';
    const brokenContent = brokenLines.join('\n');

    const service = createLanguageService(fileName, brokenContent);
    // Request completions at col 3 (after "  c")
    const offset = getOffset(brokenContent, mapped.line, 3);
    const completions = service.getCompletionsAtPosition(fileName, offset, {});

    expect(completions).not.toBeNull();
    const names = completions!.entries.map(e => e.name);

    console.log(`Stale content: got ${names.length} completions at "  c|"`);

    // With stale content, we get globals — NOT member completions
    // This is exactly the bug we see in the extension
    expect(names).toContain('child');
    expect(names).toContain('console');
    expect(names).not.toContain('stdin'); // no member completions
    expect(names.length).toBeGreaterThan(500);
  });
});
