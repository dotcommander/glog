// UI types for log viewer design
export interface LogEntryUI {
  id: number;
  timestamp: string;
  level: string;
  host: string;
  source?: string;
  category?: string;
  message: string;
  fields?: Record<string, any>;
  expanded: boolean;
}

export interface FilterUI {
  search: string;
  levels: string[];
  hostId?: number;
  startDate?: string;
  endDate?: string;
}

export interface HostUI {
  id: number;
  name: string;
  status: 'online' | 'offline' | 'degraded' | 'unknown';
  tags: string[];
  logCount: number;
  errorCount: number;
  errorRate: number;
  lastSeen: string;
}
