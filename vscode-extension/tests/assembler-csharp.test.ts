import { describe, it, expect } from 'vitest';
import { assembleCSharpSource } from '../src/assembler';
import { readFileSync } from 'fs';

describe('assembleCSharpSource', () => {
  const vortexSource = `name: cs-test

csharp:
  usings:
    - System.Diagnostics

  vars:
    apiBase: "http://localhost:3000"
    maxRetries: 3
    verbose: true

  functions:
    Run: |
      public static void Run(string name, params string[] args)
      {
          var psi = new ProcessStartInfo(name);
          using var proc = Process.Start(psi)!;
          proc.WaitForExit();
      }

jobs:
  - id: build
    shell: csharp
    use: csharp
    command: |
      Run("dotnet", "build");
      Console.WriteLine("done");
`;

  it('returns non-null for valid C# vortex source', () => {
    const result = assembleCSharpSource(vortexSource);
    expect(result).not.toBeNull();
    expect(result!.languageId).toBe('csharp');
  });

  it('emits using directives', () => {
    const result = assembleCSharpSource(vortexSource)!;
    expect(result.text).toContain('using System.Diagnostics;');
    expect(result.text).toContain('using static Vortex;');
  });

  it('emits job commands as top-level statements before the class', () => {
    const result = assembleCSharpSource(vortexSource)!;
    const lines = result.text.split('\n');
    const runLine = lines.findIndex(l => l.includes('Run("dotnet", "build")'));
    const classLine = lines.findIndex(l => l.includes('static class Vortex'));
    expect(runLine).toBeGreaterThan(0);
    expect(classLine).toBeGreaterThan(runLine);
  });

  it('emits vars as public static readonly fields', () => {
    const result = assembleCSharpSource(vortexSource)!;
    expect(result.text).toContain('public static readonly string apiBase = "http://localhost:3000";');
    expect(result.text).toContain('public static readonly int maxRetries = 3;');
    expect(result.text).toContain('public static readonly bool verbose = true;');
  });

  it('emits static class Vortex with closing brace', () => {
    const result = assembleCSharpSource(vortexSource)!;
    expect(result.text).toContain('static class Vortex');
    expect(result.text).toContain('{');
    const lines = result.text.split('\n');
    // Last non-empty line should be }
    const lastContent = lines.filter(l => l.trim()).pop();
    expect(lastContent).toBe('}');
  });

  it('emits function bodies inside the class', () => {
    const result = assembleCSharpSource(vortexSource)!;
    const lines = result.text.split('\n');
    const classLine = lines.findIndex(l => l.includes('static class Vortex'));
    const funcLine = lines.findIndex(l => l.includes('public static void Run'));
    expect(funcLine).toBeGreaterThan(classLine);
  });

  it('maps function lines back to vortex correctly', () => {
    const result = assembleCSharpSource(vortexSource)!;
    const vortexLines = vortexSource.split('\n');

    // Find "var psi = new ProcessStartInfo" in vortex
    const vortexPsiLine = vortexLines.findIndex(l => l.includes('var psi = new ProcessStartInfo'));
    expect(vortexPsiLine).toBeGreaterThan(0);

    const col = vortexLines[vortexPsiLine].indexOf('var');
    const fwd = result.sourceMap.toAssembled(vortexPsiLine, col);
    expect(fwd).not.toBeNull();

    const assembledLines = result.text.split('\n');
    expect(assembledLines[fwd!.line]).toContain('var psi = new ProcessStartInfo');

    const rev = result.sourceMap.toVortex(fwd!.line, fwd!.col);
    expect(rev).not.toBeNull();
    expect(rev!.line).toBe(vortexPsiLine);
  });

  it('maps job command lines correctly', () => {
    const result = assembleCSharpSource(vortexSource)!;
    const vortexLines = vortexSource.split('\n');

    const runLine = vortexLines.findIndex(l => l.includes('Run("dotnet", "build")'));
    expect(runLine).toBeGreaterThan(0);

    const col = vortexLines[runLine].indexOf('Run');
    const fwd = result.sourceMap.toAssembled(runLine, col);
    expect(fwd).not.toBeNull();

    const assembledLines = result.text.split('\n');
    expect(assembledLines[fwd!.line]).toContain('Run("dotnet", "build")');
  });

  it('returns null for non-C# vortex source', () => {
    const result = assembleCSharpSource('name: test\nnode:\n  functions:\n    x: |\n      console.log("hi")\n');
    expect(result).toBeNull();
  });
});

describe('assembleCSharpSource with csharp-demo.vortex', () => {
  const vortexSource = readFileSync(
    '/Users/roen/Developer/Personal/vortex/mock/csharp-demo.vortex',
    'utf-8'
  );

  it('assembles successfully', () => {
    const result = assembleCSharpSource(vortexSource);
    expect(result).not.toBeNull();
    console.log('=== Assembled C# ===');
    console.log(result!.text);
    console.log('');
    console.log('=== Source Map ===');
    for (const m of result!.sourceMap.allMappings()) {
      console.log(`  assembled:${m.assembledLine} -> vortex:${m.vortexLine} colOffset:${m.colOffset} kind:${m.kind}`);
    }
  });

  it('includes NuGet package reference comment or usings', () => {
    const result = assembleCSharpSource(vortexSource)!;
    // We emit usings from the config
    expect(result.text).toContain('using System.Diagnostics;');
    expect(result.text).toContain('using System.Text.Json;');
  });

  it('round-trips all code positions', () => {
    const result = assembleCSharpSource(vortexSource)!;

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
