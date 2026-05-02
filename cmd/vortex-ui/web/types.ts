/** Represents a user-spawned shell terminal. */
export interface ShellInfo {
  id: string;
  label: string;
  profile_id?: string;
  color?: string;
  icon?: string;
  pid: number;
}

/** A shell profile for the picker. */
export interface ShellProfile {
  id: string;
  name: string;
  command: string;
  args?: string[];
  color?: string;
  icon?: string;
  default?: boolean;
  fontFamily?: string;
  fontSize?: number;
}

export interface TerminalInfo {
  id: string;
  label: string;
  command: string;
  group: string;
  needs: string[];
  status: 'pending' | 'running' | 'success' | 'failure' | 'skipped';
}

export interface LineDTO {
  t: number;
  data: string;
}
