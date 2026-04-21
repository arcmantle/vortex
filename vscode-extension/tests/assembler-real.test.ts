import { describe, it, expect } from 'vitest';
import { readFileSync } from 'fs';
import { assembleJsSource } from '../src/assembler';

describe('assembleJsSource with run-all.vortex', () => {
  const vortexSource = readFileSync(
    '/Users/roen/Developer/Eyeshare/es-env-test/env1/.vscode/scripts/run-all.vortex',
    'utf-8'
  );

  it('assembles successfully', () => {
    const result = assembleJsSource(vortexSource);
    expect(result).not.toBeNull();
    console.log('=== Assembled JS ===');
    console.log(result!.text);
    console.log('');
    console.log('=== Source Map ===');
    for (const m of result!.sourceMap.allMappings()) {
      console.log(`  assembled:${m.assembledLine} -> vortex:${m.vortexLine} colOffset:${m.colOffset} kind:${m.kind}`);
    }
  });

  it('round-trips all code positions', () => {
    const result = assembleJsSource(vortexSource)!;
    const vortexLines = vortexSource.split('\n');

    for (const mapping of result.sourceMap.allMappings()) {
      if (mapping.vortexLine < 0) continue;

      const vortexLineText = vortexLines[mapping.vortexLine];
      const testCol = mapping.colOffset; // Start of content

      // Forward
      const fwd = result.sourceMap.toAssembled(mapping.vortexLine, testCol);
      expect(fwd).not.toBeNull();
      expect(fwd!.line).toBe(mapping.assembledLine);
      expect(fwd!.col).toBe(0); // colOffset - colOffset = 0

      // Reverse
      const rev = result.sourceMap.toVortex(fwd!.line, fwd!.col);
      expect(rev).not.toBeNull();
      expect(rev!.line).toBe(mapping.vortexLine);
      expect(rev!.col).toBe(testCol);
    }
  });

  it('maps child.on position correctly', () => {
    const result = assembleJsSource(vortexSource)!;
    const vortexLines = vortexSource.split('\n');

    // Find a line with "child.on" in vortex
    const childOnLine = vortexLines.findIndex(l => l.includes('child.on("exit"'));
    expect(childOnLine).toBeGreaterThan(0);

    // "child" starts at some column in the vortex line
    const childCol = vortexLines[childOnLine].indexOf('child');

    // Map to assembled
    const fwd = result.sourceMap.toAssembled(childOnLine, childCol);
    expect(fwd).not.toBeNull();

    // Verify the assembled line has "child" at the mapped column
    const assembledLines = result.text.split('\n');
    const assembledLine = assembledLines[fwd!.line];
    console.log(`Vortex line ${childOnLine}, col ${childCol}: "${vortexLines[childOnLine]}"`);
    console.log(`Assembled line ${fwd!.line}, col ${fwd!.col}: "${assembledLine}"`);
    expect(assembledLine.substring(fwd!.col, fwd!.col + 5)).toBe('child');

    // Map back
    const rev = result.sourceMap.toVortex(fwd!.line, fwd!.col);
    expect(rev).not.toBeNull();
    expect(rev!.line).toBe(childOnLine);
    expect(rev!.col).toBe(childCol);
  });
});
