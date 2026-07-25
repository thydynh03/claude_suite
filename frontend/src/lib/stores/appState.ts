import { writable } from 'svelte/store';
import type { LogEntry } from '../types';

export const activeTab = writable<string>('cockpit'); // "cockpit", "kanban", "settings", "office"
export const workspaceFolder = writable<string>('');
export const orchestratorRunning = writable<boolean>(false);
export const sidebarCollapsed = writable<boolean>(false);
export const isThinking = writable<boolean>(false);
export const logs = writable<LogEntry[]>([]);

// Global persistent stores — survive tab switching (component destroy/remount)
export const tasksStore = writable<any[]>([]);
export const agentsStore = writable<any[]>([]);

// Per-task streaming logs keyed by task_id (fed by the backend "task_log" event).
export const taskLogsStore = writable<Record<string, LogEntry[]>>({});

// Per-task E2E screenshot (data URL) keyed by task_id.
export const taskScreenshotsStore = writable<Record<string, string>>({});
export function setTaskScreenshot(taskId: string, dataUrl: string) {
  if (!taskId || !dataUrl) return;
  taskScreenshotsStore.update((m) => ({ ...m, [taskId]: dataUrl }));
}

export function addLog(msg: string, level = 'INFO', time = '') {
  const t = time || new Date().toLocaleTimeString('en-US', { hour12: false });
  logs.update((l) => [...l.slice(-1000), { message: msg, level, time: t }]);
}

export function addTaskLog(taskId: string, msg: string, level = 'INFO', time = '') {
  if (!taskId) return;
  const t = time || new Date().toLocaleTimeString('en-US', { hour12: false });
  taskLogsStore.update((m) => {
    const prev = m[taskId] || [];
    return { ...m, [taskId]: [...prev.slice(-500), { message: msg, level, time: t }] };
  });
}

// Transient toast notifications.
export type Toast = { id: number; message: string; level: string };
export const toasts = writable<Toast[]>([]);
let toastSeq = 0;

export function addToast(message: string, level = 'INFO', ttlMs = 4000) {
  const id = ++toastSeq;
  toasts.update((t) => [...t, { id, message, level }]);
  setTimeout(() => {
    toasts.update((t) => t.filter((x) => x.id !== id));
  }, ttlMs);
}

export function dismissToast(id: number) {
  toasts.update((t) => t.filter((x) => x.id !== id));
}

export function clearTaskLog(taskId: string) {
  taskLogsStore.update((m) => {
    const next = { ...m };
    delete next[taskId];
    return next;
  });
}
