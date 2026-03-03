import { browser } from '$app/environment';
import type { Log, LogFilters } from '$lib/types';

interface LogsStore {
  logs: Log[];
  loading: boolean;
  error: string | null;
  hasMore: boolean;
  total: number;
}

class LogsStore {
  logs = $state<Log[]>([]);
  loading = $state(false);
  error = $state<string | null>(null);
  hasMore = $state(true);
  total = $state(0);

  constructor() {
    if (browser) {
      this.fetchLogs();
    }
  }

  buildQueryString(filters: LogFilters = {}): string {
    const params = new URLSearchParams();

    if (filters.level && filters.level.length > 0) {
      filters.level.forEach(level => params.append('level', level));
    }

    if (filters.search) {
      params.set('search', filters.search);
    }

    if (filters.host_id) {
      params.set('host_id', filters.host_id.toString());
    }

    if (filters.start_date) {
      params.set('start_date', filters.start_date);
    }

    if (filters.end_date) {
      params.set('end_date', filters.end_date);
    }

    if (filters.limit) {
      params.set('limit', filters.limit.toString());
    }

    if (filters.offset) {
      params.set('offset', filters.offset.toString());
    }

    return params.toString();
  }

  async fetchLogs(filters: LogFilters = {}, append = false) {
    this.loading = true;
    this.error = null;

    try {
      const queryString = this.buildQueryString(filters);
      const url = `/api/v1/logs${queryString ? '?' + queryString : ''}`;

      const response = await fetch(url);
      if (!response.ok) {
        throw new Error(`Failed to fetch logs: ${response.status}`);
      }

      const data = await response.json();

      if (append) {
        this.logs = [...this.logs, ...(data.logs || [])];
      } else {
        this.logs = data.logs || [];
      }

      this.total = data.total || 0;
      this.hasMore = data.has_more || false;
    } catch (err) {
      this.error = err instanceof Error ? err.message : 'Failed to fetch logs';
      console.error('Error fetching logs:', err);
    } finally {
      this.loading = false;
    }
  }

  async loadMore(filters: LogFilters = {}) {
    if (!this.hasMore || this.loading) return;

    const nextOffset = this.logs.length;
    await this.fetchLogs({ ...filters, offset: nextOffset }, true);
  }

  addLog(log: Log) {
    // Insert at the beginning for reverse chronological order
    this.logs = [log, ...this.logs];
    this.total += 1;
  }

  updateLog(updatedLog: Log) {
    this.logs = this.logs.map(log =>
      log.id === updatedLog.id ? updatedLog : log
    );
  }

  clearLogs() {
    this.logs = [];
    this.total = 0;
    this.hasMore = true;
  }

  // Get logs by level
  getLogsByLevel(level: string): Log[] {
    return this.logs.filter(log => log.level === level);
  }

  // Get error logs
  get errors(): Log[] {
    return this.logs.filter(log => log.level === 'error' || log.level === 'fatal');
  }

  // Get logs for a specific host
  getLogsByHost(hostId: number): Log[] {
    return this.logs.filter(log => log.host_id === hostId);
  }
}

export const logsStore = new LogsStore();
