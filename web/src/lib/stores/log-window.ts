import type { Log, LogFilters } from '../types';

// Keep a bounded viewport; history remains available through the server's offset pages.
export const logWindowSize = 200;

export function matchesLiveFilters(log: Log, filters: LogFilters): boolean {
  if (filters.search) return false; // SQL also searches serialized fields; only the server can decide.
  if (filters.host_id && log.host_id !== filters.host_id) return false;
  if (filters.level?.length && !filters.level.includes(log.level)) return false;
  // The server compares raw SQLite timestamp strings; do not reinterpret local date inputs.
  if (filters.start_date || filters.end_date) return false;
  return true;
}

export function mergeLiveLog(logs: Log[], log: Log): Log[] {
  if (logs.some(item => item.id === log.id)) return logs;
  return [log, ...logs].slice(0, logWindowSize);
}

export function mergeOlderPage(logs: Log[], older: Log[]): Log[] {
  const ids = new Set(logs.map(log => log.id));
  return [...logs, ...older.filter(log => !ids.has(log.id))].slice(-logWindowSize);
}
