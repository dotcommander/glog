export interface Host {
  id: number;
  name: string;
  tags: string[];
  status: 'online' | 'offline' | 'degraded' | 'unknown';
  created_at: string;
  last_seen: string;
  last_log_id: number | null;
  log_count: number;
  error_count: number;
  error_rate: number;
  description: string | null;
  hostname: string | null;
  ip: string | null;
  user_agent: string | null;
  metadata: Record<string, any>;
}

export interface Log {
  id: number;
  host_id: number;
  level: 'trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal';
  message: string;
  fields: Record<string, any>;
  timestamp: string;
  host: Host;

  // HTTP context
  method?: string;
  path?: string;
  status_code?: number;
  duration_ms?: number;

  // Derived metadata
  derived_level?: string;
  derived_source?: string;
  derived_category?: string;
}

export interface LogFilters {
  level?: string[];
  search?: string;
  host_id?: number;
  start_date?: string;
  end_date?: string;
  limit?: number;
  offset?: number;
}

export interface LogBucket {
  bucket_start: string;
  bucket_width: string;
  host_id: number;
  level: Log['level'];
  count: number;
}

export interface FingerprintBucket {
  bucket_start: string;
  bucket_width: string;
  fingerprint: string;
  level: Log['level'];
  count: number;
  first_seen: string;
  last_seen: string;
}

export interface InboxItem {
  id: number;
  title: string;
  body?: string;
  status: 'open' | 'in_progress' | 'blocked' | 'done' | 'dismissed';
  kind: 'note' | 'task' | 'question' | 'bug' | 'handoff';
  priority: 'low' | 'normal' | 'high';
  source?: string;
  repo?: string;
  session_id?: string;
  tags: string[];
  linked_log_ids: string[];
  linked_fingerprints: string[];
  created_by?: string;
  claimed_by?: string;
  created_at: string;
  updated_at: string;
  due_at?: string;
}

export interface SSEEvent {
  event: string;
  data: any;
}
