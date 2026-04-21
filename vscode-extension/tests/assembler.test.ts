import { describe, it, expect } from 'vitest';
import { assembleJsSource } from '../src/assembler';

describe('assembleJsSource', () => {
  const vortexSource = `name: test-app

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

  it('returns non-null for valid vortex source', () => {
    const result = assembleJsSource(vortexSource);
    expect(result).not.toBeNull();
  });

  it('assembles imports correctly', () => {
    const result = assembleJsSource(vortexSource)!;
    const lines = result.text.split('\n');
    expect(lines[0]).toBe("import { spawn } from 'node:child_process';");
    expect(lines[1]).toBe("import path from 'node:path';");
  });

  it('assembles function body', () => {
    const result = assembleJsSource(vortexSource)!;
    const lines = result.text.split('\n');
    // After imports + synthetic separator, function body starts
    expect(lines.some(l => l.includes('export function run(spec)'))).toBe(true);
  });

  it('assembles job command', () => {
    const result = assembleJsSource(vortexSource)!;
    const lines = result.text.split('\n');
    expect(lines.some(l => l.includes('run({'))).toBe(true);
  });

  it('maps function line back to correct vortex line', () => {
    const result = assembleJsSource(vortexSource)!;
    const vortexLines = vortexSource.split('\n');

    // Find the line "export function run(spec) {" in vortex — it's on line 11 (0-based)
    const vortexFnLine = vortexLines.findIndex(l => l.includes('export function run(spec)'));
    expect(vortexFnLine).toBe(11);

    // Map vortex line 11 to assembled
    const assembled = result.sourceMap.toAssembled(vortexFnLine, 6); // col 6 = start of content after indent
    expect(assembled).not.toBeNull();

    // Map back
    const backToVortex = result.sourceMap.toVortex(assembled!.line, assembled!.col);
    expect(backToVortex).not.toBeNull();
    expect(backToVortex!.line).toBe(vortexFnLine);
    expect(backToVortex!.col).toBe(6);
  });

  it('maps job command line back to correct vortex line', () => {
    const result = assembleJsSource(vortexSource)!;
    const vortexLines = vortexSource.split('\n');

    // Find "run({" in vortex
    const vortexCmdLine = vortexLines.findIndex(l => l.includes('run({'));
    expect(vortexCmdLine).toBeGreaterThan(0);

    const assembled = result.sourceMap.toAssembled(vortexCmdLine, 6);
    expect(assembled).not.toBeNull();

    const backToVortex = result.sourceMap.toVortex(assembled!.line, assembled!.col);
    expect(backToVortex).not.toBeNull();
    expect(backToVortex!.line).toBe(vortexCmdLine);
  });

  it('round-trips column mapping for indented content', () => {
    const result = assembleJsSource(vortexSource)!;
    const vortexLines = vortexSource.split('\n');

    // Line with "const child = spawn(..." — indented 8 in vortex (6 yaml indent + 2 code indent)
    const childLine = vortexLines.findIndex(l => l.includes('const child = spawn'));
    expect(childLine).toBeGreaterThan(0);

    // The "c" in "const" is at some column in vortex
    const vortexCol = vortexLines[childLine].indexOf('const');
    expect(vortexCol).toBeGreaterThan(0);

    // Map to assembled
    const assembled = result.sourceMap.toAssembled(childLine, vortexCol);
    expect(assembled).not.toBeNull();

    // The assembled line should have "const" starting at the mapped col
    const assembledLines = result.text.split('\n');
    const assembledLineText = assembledLines[assembled!.line];
    expect(assembledLineText.charAt(assembled!.col)).toBe('c');

    // Map back
    const back = result.sourceMap.toVortex(assembled!.line, assembled!.col);
    expect(back).not.toBeNull();
    expect(back!.line).toBe(childLine);
    expect(back!.col).toBe(vortexCol);
  });

  it('returns null for non-code vortex lines', () => {
    const result = assembleJsSource(vortexSource)!;
    // Line 0 is "name: test-app" — not in any code region
    expect(result.sourceMap.toAssembled(0, 0)).toBeNull();
    // Line 1 is blank
    expect(result.sourceMap.toAssembled(1, 0)).toBeNull();
  });

  it('returns null for non-JS runtime', () => {
    const noJsSource = `name: test
jobs:
  - id: build
    shell: bash
    command: echo hello
`;
    const result = assembleJsSource(noJsSource);
    expect(result).toBeNull();
  });
});
