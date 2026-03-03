import { browser } from '$app/environment';
import { logsStore } from './logs.svelte';
import { hostsStore } from './hosts.svelte';
import type { SSEEvent } from '$lib/types';

class SSEStore {
  eventSource: EventSource | null = $state(null);
  connected = $state(false);
  reconnectAttempts = $state(0);
  maxReconnectAttempts = 5;
  reconnectDelay = 1000; // Start with 1 second

  constructor() {
    if (browser) {
      this.connect();
    }
  }

  connect(apiKey?: string) {
    if (!browser) return;

    // Close existing connection if any
    this.disconnect();

    // Build URL with API key if provided
    let url = '/api/v1/events';
    if (apiKey) {
      url += `?api_key=${encodeURIComponent(apiKey)}`;
    }

    try {
      this.eventSource = new EventSource(url);

      this.eventSource.onopen = () => {
        console.log('SSE connection opened');
        this.connected = true;
        this.reconnectAttempts = 0;
        this.reconnectDelay = 1000; // Reset delay
      };

      this.eventSource.onerror = (error) => {
        console.error('SSE connection error:', error);
        this.connected = false;

        // Attempt to reconnect with exponential backoff
        if (this.reconnectAttempts < this.maxReconnectAttempts) {
          this.reconnectAttempts++;
          const delay = this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1);

          console.log(`Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts}/${this.maxReconnectAttempts})`);

          setTimeout(() => {
            this.connect(apiKey);
          }, delay);
        } else {
          console.error('Max reconnection attempts reached');
        }
      };

      // Listen for log events
      this.eventSource.addEventListener('log.created', (event) => {
        try {
          const logData = JSON.parse(event.data);
          logsStore.addLog(logData);
        } catch (err) {
          console.error('Failed to parse log event:', err);
        }
      });

      // Listen for log deleted events
      this.eventSource.addEventListener('log.deleted', (event) => {
        try {
          const logData = JSON.parse(event.data);
          // Remove log from store
          logsStore.logs = logsStore.logs.filter(log => log.id !== logData.id);
          logsStore.total = Math.max(0, logsStore.total - 1);
        } catch (err) {
          console.error('Failed to parse log deleted event:', err);
        }
      });

      // Listen for host events
      this.eventSource.addEventListener('host.registered', (event) => {
        try {
          const hostData = JSON.parse(event.data);
          hostsStore.addHost(hostData);
        } catch (err) {
          console.error('Failed to parse host event:', err);
        }
      });

      this.eventSource.addEventListener('host.updated', (event) => {
        try {
          const hostData = JSON.parse(event.data);
          hostsStore.updateHost(hostData);
        } catch (err) {
          console.error('Failed to parse host updated event:', err);
        }
      });

    } catch (error) {
      console.error('Failed to create EventSource:', error);
      this.connected = false;
    }
  }

  disconnect() {
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
      this.connected = false;
    }
  }

  reconnect() {
    this.disconnect();
    this.connect();
  }

  // Manual refresh (poll) when SSE is not available
  async refresh() {
    if (!this.connected) {
      console.log('SSE not connected, refreshing data manually');
      await Promise.all([
        hostsStore.fetchHosts(),
        logsStore.fetchLogs()
      ]);
    }
  }
}

export const sseStore = new SSEStore();

// Auto-refresh every 30 seconds as fallback
if (browser) {
  setInterval(() => {
    if (!sseStore.connected) {
      sseStore.refresh();
    }
  }, 30000);
}
