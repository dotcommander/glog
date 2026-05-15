import { browser } from '$app/environment';
import type { FingerprintBucket, LogBucket, LogFilters, Log } from '$lib/types';

interface StatsStore {
  logBuckets: LogBucket[];
  fingerprintBuckets: FingerprintBucket[];
  loading: boolean;
  error: string | null;
}

const levels: Log['level'][] = ['trace', 'debug', 'info', 'warn', 'error', 'fatal'];

class StatsStore {
  logBuckets = $state<LogBucket[]>([]);
  fingerprintBuckets = $state<FingerprintBucket[]>([]);
  loading = $state(false);
  error = $state<string | null>(null);

  constructor() {
    if (browser) {
      this.fetchStats();
    }
  }

  buildQueryString(filters: LogFilters = {}): string {
    const params = new URLSearchParams();
    params.set('bucket', '1h');

    if (filters.host_id) {
      params.set('host_id', filters.host_id.toString());
    }

    if (filters.start_date) {
      params.set('from', filters.start_date);
    } else {
      params.set('from', new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString());
    }

    if (filters.end_date) {
      params.set('to', filters.end_date);
    }

    return params.toString();
  }

  async fetchStats(filters: LogFilters = {}) {
    this.loading = true;
    this.error = null;

    try {
      const queryString = this.buildQueryString(filters);
      const [logsResponse, fingerprintsResponse] = await Promise.all([
        fetch(`/api/v1/stats/logs?${queryString}`),
        fetch(`/api/v1/stats/fingerprints?${queryString}`)
      ]);

      if (!logsResponse.ok) {
        throw new Error(`Failed to fetch log stats: ${logsResponse.status}`);
      }
      if (!fingerprintsResponse.ok) {
        throw new Error(`Failed to fetch fingerprint stats: ${fingerprintsResponse.status}`);
      }

      const logsData = await logsResponse.json();
      const fingerprintsData = await fingerprintsResponse.json();

      this.logBuckets = logsData.buckets || [];
      this.fingerprintBuckets = fingerprintsData.buckets || [];
    } catch (err) {
      this.error = err instanceof Error ? err.message : 'Failed to fetch stats';
      console.error('Error fetching stats:', err);
    } finally {
      this.loading = false;
    }
  }

  get totalLogs(): number {
    return this.logBuckets.reduce((sum, bucket) => sum + bucket.count, 0);
  }

  get errorLogs(): number {
    return this.logBuckets
      .filter(bucket => bucket.level === 'error' || bucket.level === 'fatal')
      .reduce((sum, bucket) => sum + bucket.count, 0);
  }

  get errorRate(): number {
    if (this.totalLogs === 0) return 0;
    return Math.round((this.errorLogs / this.totalLogs) * 1000) / 10;
  }

  get levelTotals(): Array<{ level: Log['level']; count: number }> {
    return levels.map(level => ({
      level,
      count: this.logBuckets
        .filter(bucket => bucket.level === level)
        .reduce((sum, bucket) => sum + bucket.count, 0)
    }));
  }

  get hourlyTotals(): Array<{ hour: string; count: number }> {
    const totals = new Map<string, number>();

    for (const bucket of this.logBuckets) {
      totals.set(bucket.bucket_start, (totals.get(bucket.bucket_start) || 0) + bucket.count);
    }

    return [...totals.entries()]
      .sort(([a], [b]) => new Date(a).getTime() - new Date(b).getTime())
      .slice(-24)
      .map(([hour, count]) => ({ hour, count }));
  }

  get topFingerprints(): FingerprintBucket[] {
    const totals = new Map<string, FingerprintBucket>();

    for (const bucket of this.fingerprintBuckets) {
      const existing = totals.get(bucket.fingerprint);
      if (!existing) {
        totals.set(bucket.fingerprint, { ...bucket });
        continue;
      }

      existing.count += bucket.count;
      if (new Date(bucket.last_seen).getTime() > new Date(existing.last_seen).getTime()) {
        existing.last_seen = bucket.last_seen;
      }
      if (new Date(bucket.first_seen).getTime() < new Date(existing.first_seen).getTime()) {
        existing.first_seen = bucket.first_seen;
      }
    }

    return [...totals.values()]
      .sort((a, b) => b.count - a.count || a.fingerprint.localeCompare(b.fingerprint))
      .slice(0, 5);
  }
}

export const statsStore = new StatsStore();
