import * as vscode from 'vscode';

const channel = vscode.window.createOutputChannel('Vortex');

export function log(msg: string): void {
  channel.appendLine(`[${new Date().toISOString()}] ${msg}`);
}
