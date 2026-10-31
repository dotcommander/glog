import { describe, expect, test } from 'bun:test';
import type { Log } from '../types';
import { logWindowSize, matchesLiveFilters, mergeLiveLog, mergeOlderPage } from './log-window';

const log = (id: number): Log => ({
  id, host_id: 1, level: 'info', message: `event ${id}`,
  fields: {}, timestamp: '2026-05-20T12:00:00Z', host: {} as Log['host']
});

describe('bounded log viewport', () => {
  test('caps SSE inserts, avoids duplicate ids, and keeps newest rows', () => {
    let logs: Log[] = [];
    for (let id = 1; id <= logWindowSize + 20; id++) logs = mergeLiveLog(logs, log(id));
    expect(logs).toHaveLength(logWindowSize);
    expect(logs[0].id).toBe(logWindowSize + 20);
    expect(logs.at(-1)?.id).toBe(21);
    expect(mergeLiveLog(logs, log(logWindowSize + 20))).toBe(logs);
  });

  test('older pages deduplicate overlapping ids and evict newest when window fills', () => {
    const logs = Array.from({ length: logWindowSize }, (_, i) => log(logWindowSize - i));
    const older = [log(1), log(0), log(-1)];
    const merged = mergeOlderPage(logs, older);
    expect(merged).toHaveLength(logWindowSize);
    expect(merged[0].id).toBe(logWindowSize - 2);
    expect(merged.at(-1)?.id).toBe(-1);
  });

  test('rejects events with server-only search/date semantics and filters host and level', () => {
    expect(matchesLiveFilters(log(1), { search: 'event' })).toBe(false);
    expect(matchesLiveFilters(log(1), { host_id: 2 })).toBe(false);
    expect(matchesLiveFilters(log(1), { level: ['error'] })).toBe(false);
    expect(matchesLiveFilters(log(1), { start_date: '2026-05-21' })).toBe(false);
    expect(matchesLiveFilters(log(1), { end_date: '2026-05-19' })).toBe(false);
    expect(matchesLiveFilters(log(1), { host_id: 1, level: ['info'] })).toBe(true);
  });
});
