import { writable } from 'svelte/store';
import type { LogEntry } from '../types';

export const activeTab = writable<string>('cockpit'); // "cockpit", "kanban", "settings", "office"
export const workspaceFolder = writable<string>('');
export const orchestratorRunning = writable<bool>(false);
export const isThinking = writable<bool>(false);
export const logs = writable<LogEntry[]>([]);

export function addLog(msg: string, level = 'INFO', time = '') {
  const t = time || new Date().toLocaleTimeString('en-US', { hour12: false });
  logs.update((l) => [...l.slice(-1000), { message: msg, level, time: t }]);
}
