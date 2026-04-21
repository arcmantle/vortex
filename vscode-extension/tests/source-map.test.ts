import { describe, it, expect } from 'vitest';
import { SourceMap, type LineMapping } from '../src/source-map';

describe('SourceMap', () => {
  it('maps vortex position to assembled and back', () => {
    const mappings: LineMapping[] = [
      { assembledLine: 0, vortexLine: 4, colOffset: 0, kind: 'import' },
      { assembledLine: 1, vortexLine: 6, colOffset: 0, kind: 'import' },
      { assembledLine: 2, vortexLine: -1, colOffset: 0, kind: 'synthetic' },
      { assembledLine: 3, vortexLine: 11, colOffset: 6, kind: 'function' },
      { assembledLine: 4, vortexLine: 12, colOffset: 6, kind: 'function' },
      { assembledLine: 5, vortexLine: 13, colOffset: 6, kind: 'function' },
    ];
    const sm = new SourceMap(mappings);

    // Forward mapping: vortex line 12, col 10 → assembled line 4, col 4
    const fwd = sm.toAssembled(12, 10);
    expect(fwd).toEqual({ line: 4, col: 4 });

    // Reverse mapping: assembled line 4, col 4 → vortex line 12, col 10
    const rev = sm.toVortex(4, 4);
    expect(rev).toEqual({ line: 12, col: 10 });
  });

  it('returns null for unmapped vortex lines', () => {
    const mappings: LineMapping[] = [
      { assembledLine: 0, vortexLine: 5, colOffset: 6, kind: 'function' },
    ];
    const sm = new SourceMap(mappings);
    expect(sm.toAssembled(99, 0)).toBeNull();
  });

  it('returns null for synthetic assembled lines', () => {
    const mappings: LineMapping[] = [
      { assembledLine: 0, vortexLine: -1, colOffset: 0, kind: 'synthetic' },
    ];
    const sm = new SourceMap(mappings);
    expect(sm.toVortex(0, 0)).toBeNull();
  });

  it('clamps column to 0 when vortexCol < colOffset', () => {
    const mappings: LineMapping[] = [
      { assembledLine: 0, vortexLine: 10, colOffset: 6, kind: 'function' },
    ];
    const sm = new SourceMap(mappings);
    const result = sm.toAssembled(10, 2);
    expect(result).toEqual({ line: 0, col: 0 });
  });

  it('isVortexLineMapped works', () => {
    const mappings: LineMapping[] = [
      { assembledLine: 0, vortexLine: -1, colOffset: 0, kind: 'synthetic' },
      { assembledLine: 1, vortexLine: 5, colOffset: 6, kind: 'function' },
      { assembledLine: 2, vortexLine: 6, colOffset: 6, kind: 'function' },
    ];
    const sm = new SourceMap(mappings);
    expect(sm.isVortexLineMapped(5)).toBe(true);
    expect(sm.isVortexLineMapped(6)).toBe(true);
    expect(sm.isVortexLineMapped(7)).toBe(false);
  });
});
