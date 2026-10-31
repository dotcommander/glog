import type { Log, LogFilters } from '$lib/types';
import { logWindowSize, matchesLiveFilters, mergeLiveLog, mergeOlderPage } from './log-window';

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
  pageOffset = $state(0);
  viewingOlder = $state(false);
  private nextOffset = 0;
  private requestId = 0;
  private controller: AbortController | null = null;
  private pendingLive: Log[] = [];
  private snapshotID: string | null = null;
  private snapshotTotal = 0;
  private filteredRefreshTimer: ReturnType<typeof setTimeout> | undefined;
  private refreshAfterLoad = false;
  private activeFilters: LogFilters = {};

  buildQueryString(filters: LogFilters = {}, snapshotID: string | null = null): string {
    const params = new URLSearchParams();
    filters.level?.forEach(level => params.append('level', level));
    if (filters.search) params.set('search', filters.search);
    if (filters.host_id) params.set('host_id', filters.host_id.toString());
    if (filters.start_date) params.set('from', filters.start_date);
    if (filters.end_date) params.set('to', filters.end_date);
    const limit = Number.isFinite(filters.limit) && filters.limit! > 0
      ? Math.min(Math.floor(filters.limit!), logWindowSize) : 50;
    params.set('limit', limit.toString());
    if (filters.offset) params.set('offset', filters.offset.toString());
    params.set('snapshot_id', String(snapshotID ?? 0));
    return params.toString();
  }

  async fetchLogs(filters: LogFilters = {}, append = false) {
    const id = ++this.requestId;
    this.controller?.abort();
    const controller = new AbortController();
    this.controller = controller;
    this.pendingLive = [];
    if (!append) {
      this.snapshotID = null;
      clearTimeout(this.filteredRefreshTimer);
      this.filteredRefreshTimer = undefined;
    }
    this.activeFilters = { ...filters, level: [...(filters.level || [])] };
    this.loading = true;
    this.error = null;
    const offset = append ? this.nextOffset : (filters.offset || 0);

    try {
      const query = this.buildQueryString({ ...filters, offset }, append ? this.snapshotID : null);
      const response = await fetch(`/api/v1/logs${query ? '?' + query : ''}`, { signal: controller.signal });
      if (!response.ok) throw new Error(`Failed to fetch logs: ${response.status}`);
      const data = await response.json();
      if (id !== this.requestId) return;

      const page: Log[] = data.logs || [];
      this.snapshotID = data.snapshot_id;
      const previous = append ? this.logs : [];
      const combined = append && previous.length < logWindowSize
        ? mergeOlderPage(previous, page)
        : page.slice(0, logWindowSize);
      this.viewingOlder = append || offset > 0;
      this.logs = this.viewingOlder ? combined : this.pendingLive.reduce((logs, log) => mergeLiveLog(logs, log), combined);
      // Snapshot offsets count only server rows; live rows are outside the snapshot.
      this.nextOffset = offset + page.length;
      this.pageOffset = Math.max(0, this.nextOffset - this.logs.length);
      this.snapshotTotal = data.total || 0;
      this.total = this.snapshotTotal + (this.viewingOlder ? 0 : this.pendingLive.filter(log =>
        !page.some(item => item.id === log.id)
      ).length);
      this.hasMore = page.length > 0 && this.nextOffset < this.snapshotTotal;
      this.pendingLive = [];
    } catch (err) {
      if (id !== this.requestId || controller.signal.aborted) return;
      this.error = err instanceof Error ? err.message : 'Failed to fetch logs';
      console.error('Error fetching logs:', err);
    } finally {
      if (id === this.requestId) {
        this.loading = false;
        this.controller = null;
        if (this.refreshAfterLoad) {
          this.refreshAfterLoad = false;
          this.queueFilteredRefresh();
        }
      }
    }
  }

  private queueFilteredRefresh() {
    if (this.filteredRefreshTimer || this.viewingOlder) return;
    this.filteredRefreshTimer = setTimeout(() => {
      this.filteredRefreshTimer = undefined;
      if (this.loading) {
        this.refreshAfterLoad = true;
      } else {
        void this.fetchLogs(this.activeFilters);
      }
    }, 300);
  }

  async loadMore(filters: LogFilters = {}) {
    if (!this.hasMore || this.loading) return;
    await this.fetchLogs({ ...filters, offset: this.nextOffset }, true);
  }

  addLog(log: Log) {
    if ((this.activeFilters.search || this.activeFilters.start_date || this.activeFilters.end_date) && !this.viewingOlder) {
      // Search also matches serialized fields in SQL. Let the server decide once per burst.
      this.queueFilteredRefresh();
      return;
    }
    if (!matchesLiveFilters(log, this.activeFilters) || this.logs.some(item => item.id === log.id)) return;
    // Do not interrupt an older page with live rows; the Newer action retrieves them.
    if (this.viewingOlder) {
      return;
    }
    if (this.loading) {
      this.pendingLive = [...this.pendingLive, log].slice(-logWindowSize);
    }
    this.logs = mergeLiveLog(this.logs, log);
    this.total++;
    // Do not advance the snapshot cursor for newly inserted rows.
  }

  removeLog(id: number) {
    if (!this.logs.some(log => log.id === id)) return;
    this.logs = this.logs.filter(log => log.id !== id);
    this.pendingLive = this.pendingLive.filter(log => log.id !== id);
  }

  updateLog(updatedLog: Log) {
    this.logs = this.logs.map(log => log.id === updatedLog.id ? updatedLog : log);
  }

  clearLogs() {
    this.controller?.abort();
    this.requestId++;
    this.controller = null;
    this.pendingLive = [];
    clearTimeout(this.filteredRefreshTimer);
    this.filteredRefreshTimer = undefined;
    this.refreshAfterLoad = false;
    this.snapshotID = null;
    this.snapshotTotal = 0;
    this.loading = false;
    this.logs = [];
    this.total = 0;
    this.nextOffset = 0;
    this.pageOffset = 0;
    this.viewingOlder = false;
    this.hasMore = true;
  }

  getLogsByLevel(level: string): Log[] {
    return this.logs.filter(log => log.level === level);
  }

  get errors(): Log[] {
    return this.logs.filter(log => log.level === 'error' || log.level === 'fatal');
  }

  getLogsByHost(hostId: number): Log[] {
    return this.logs.filter(log => log.host_id === hostId);
  }
}

export const logsStore = new LogsStore();
