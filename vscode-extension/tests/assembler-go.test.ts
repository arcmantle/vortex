import { describe, it, expect } from 'vitest';
import { assembleGoSource } from '../src/assembler';
import { readFileSync } from 'fs';

describe('assembleGoSource', () => {
  const vortexSource = `name: go-test

go:
  imports:
    - path: fmt
      version: std
    - path: os/exec
      version: std

  vars:
    apiBase: "http://localhost:3000"

  functions:
    run: |
      func run(name string, args ...string) {
        cmd := exec.Command(name, args...)
        cmd.Stdout = os.Stdout
        cmd.Stderr = os.Stderr
        cmd.Run()
      }

jobs:
  - id: build
    shell: go
    use: go
    command: |
      run("go", "build", "./...")
      fmt.Println("done")
`;

  it('returns non-null for valid Go vortex source', () => {
    const result = assembleGoSource(vortexSource);
    expect(result).not.toBeNull();
    expect(result!.languageId).toBe('go');
  });

  it('starts with package main', () => {
    const result = assembleGoSource(vortexSource)!;
    const lines = result.text.split('\n');
    expect(lines[0]).toBe('package main');
  });

  it('assembles import block correctly', () => {
    const result = assembleGoSource(vortexSource)!;
    expect(result.text).toContain('import (');
    expect(result.text).toContain('\t"fmt"');
    expect(result.text).toContain('\t"os/exec"');
    expect(result.text).toContain(')');
  });

  it('assembles var declaration', () => {
    const result = assembleGoSource(vortexSource)!;
    expect(result.text).toContain('var apiBase = "http://localhost:3000"');
  });

  it('assembles function body', () => {
    const result = assembleGoSource(vortexSource)!;
    expect(result.text).toContain('func run(name string, args ...string)');
    expect(result.text).toContain('cmd := exec.Command(name, args...)');
  });

  it('wraps job commands in func main()', () => {
    const result = assembleGoSource(vortexSource)!;
    expect(result.text).toContain('func main() {');
    expect(result.text).toContain('run("go", "build", "./...")');
    expect(result.text).toContain('fmt.Println("done")');
    // func main() should be closed
    const lines = result.text.split('\n');
    const mainClose = lines.findIndex(l => l === '}');
    expect(mainClose).toBeGreaterThan(0);
  });

  it('maps function lines back to vortex correctly', () => {
    const result = assembleGoSource(vortexSource)!;
    const vortexLines = vortexSource.split('\n');

    // Find "cmd := exec.Command" in vortex
    const vortexCmdLine = vortexLines.findIndex(l => l.includes('cmd := exec.Command'));
    expect(vortexCmdLine).toBeGreaterThan(0);

    // Map forward
    const col = vortexLines[vortexCmdLine].indexOf('cmd');
    const fwd = result.sourceMap.toAssembled(vortexCmdLine, col);
    expect(fwd).not.toBeNull();

    // Check assembled text at that position
    const assembledLines = result.text.split('\n');
    expect(assembledLines[fwd!.line].includes('cmd := exec.Command')).toBe(true);

    // Map back
    const rev = result.sourceMap.toVortex(fwd!.line, fwd!.col);
    expect(rev).not.toBeNull();
    expect(rev!.line).toBe(vortexCmdLine);
  });

  it('maps job command lines correctly', () => {
    const result = assembleGoSource(vortexSource)!;
    const vortexLines = vortexSource.split('\n');

    const runLine = vortexLines.findIndex(l => l.includes('run("go", "build"'));
    expect(runLine).toBeGreaterThan(0);

    const col = vortexLines[runLine].indexOf('run');
    const fwd = result.sourceMap.toAssembled(runLine, col);
    expect(fwd).not.toBeNull();

    const assembledLines = result.text.split('\n');
    expect(assembledLines[fwd!.line]).toContain('run("go", "build"');
  });

  it('returns null for non-Go vortex source', () => {
    const result = assembleGoSource('name: test\nnode:\n  functions:\n    x: |\n      console.log("hi")\n');
    expect(result).toBeNull();
  });
});

describe('assembleGoSource with go-demo.vortex', () => {
  const vortexSource = readFileSync(
    '/Users/roen/Developer/Personal/vortex/mock/go-demo.vortex',
    'utf-8'
  );

  it('assembles successfully', () => {
    const result = assembleGoSource(vortexSource);
    expect(result).not.toBeNull();
    console.log('=== Assembled Go ===');
    console.log(result!.text);
    console.log('');
    console.log('=== Source Map ===');
    for (const m of result!.sourceMap.allMappings()) {
      console.log(`  assembled:${m.assembledLine} -> vortex:${m.vortexLine} colOffset:${m.colOffset} kind:${m.kind}`);
    }
  });

  it('round-trips all code positions', () => {
    const result = assembleGoSource(vortexSource)!;

    for (const mapping of result.sourceMap.allMappings()) {
      if (mapping.vortexLine < 0) continue;

      const testCol = mapping.colOffset;
      const fwd = result.sourceMap.toAssembled(mapping.vortexLine, testCol);
      expect(fwd).not.toBeNull();
      expect(fwd!.line).toBe(mapping.assembledLine);
      expect(fwd!.col).toBe(0);

      const rev = result.sourceMap.toVortex(fwd!.line, fwd!.col);
      expect(rev).not.toBeNull();
      expect(rev!.line).toBe(mapping.vortexLine);
      expect(rev!.col).toBe(testCol);
    }
  });
});
