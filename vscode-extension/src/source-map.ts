/**
 * Bidirectional line+column source map between a .vortex file and its assembled source.
 *
 * Pure data structures — no VS Code dependencies, fully testable.
 */

/** A single line mapping between the assembled source and the original .vortex file. */
export interface LineMapping {
  /** 0-based line in the assembled output. */
  assembledLine: number;
  /** 0-based line in the .vortex source. -1 if synthetic (no vortex origin). */
  vortexLine: number;
  /**
   * Column offset: how many characters of indentation were removed from the
   * vortex line to produce the assembled line.
   *
   * To map vortex col → assembled col:  assembledCol = vortexCol - colOffset
   * To map assembled col → vortex col:  vortexCol = assembledCol + colOffset
   */
  colOffset: number;
  /** What this line represents. */
  kind: 'import' | 'function' | 'command' | 'synthetic';
}

export class SourceMap {
  /** Indexed by assembled line number. */
  private readonly mappings: LineMapping[];
  /** Reverse index: vortexLine → assembledLine (first occurrence). */
  private readonly reverseIndex: Map<number, number>;

  constructor(mappings: LineMapping[]) {
    this.mappings = mappings;
    this.reverseIndex = new Map();
    for (const m of mappings) {
      if (m.vortexLine >= 0 && !this.reverseIndex.has(m.vortexLine)) {
        this.reverseIndex.set(m.vortexLine, m.assembledLine);
      }
    }
  }

  /** Total number of lines in the assembled output. */
  get lineCount(): number {
    return this.mappings.length;
  }

  /** Get the mapping for a given assembled line. */
  getByAssembledLine(line: number): LineMapping | null {
    return this.mappings[line] ?? null;
  }

  /** Get the assembled line for a given vortex line, or null if unmapped. */
  getAssembledLineForVortex(vortexLine: number): number | null {
    return this.reverseIndex.get(vortexLine) ?? null;
  }

  /**
   * Map a position in the .vortex file to the assembled source.
   * Returns null if the vortex line has no mapping.
   */
  toAssembled(vortexLine: number, vortexCol: number): { line: number; col: number } | null {
    const assembledLine = this.reverseIndex.get(vortexLine);
    if (assembledLine === undefined) return null;
    const mapping = this.mappings[assembledLine];
    const col = Math.max(0, vortexCol - mapping.colOffset);
    return { line: assembledLine, col };
  }

  /**
   * Map a position in the assembled source back to the .vortex file.
   * Returns null if the assembled line is synthetic.
   */
  toVortex(assembledLine: number, assembledCol: number): { line: number; col: number } | null {
    const mapping = this.mappings[assembledLine];
    if (!mapping || mapping.vortexLine < 0) return null;
    const col = assembledCol + mapping.colOffset;
    return { line: mapping.vortexLine, col };
  }

  /**
   * Check if a vortex line is inside any mapped code region.
   */
  isVortexLineMapped(vortexLine: number): boolean {
    return this.reverseIndex.has(vortexLine);
  }

  /** Get all mappings (for debugging/testing). */
  allMappings(): readonly LineMapping[] {
    return this.mappings;
  }
}
