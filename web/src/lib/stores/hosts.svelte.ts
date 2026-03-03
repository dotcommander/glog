import { browser } from '$app/environment';
import type { Host } from '$lib/types';

interface HostsStore {
  hosts: Host[];
  loading: boolean;
  error: string | null;
  currentHostId: number | null;
}

class HostsStore {
  hosts = $state<Host[]>([]);
  loading = $state(false);
  error = $state<string | null>(null);
  currentHostId = $state<number | null>(null);

  constructor() {
    if (browser) {
      this.fetchHosts();
    }
  }

  async fetchHosts() {
    this.loading = true;
    this.error = null;

    try {
      const response = await fetch('/api/v1/hosts');
      if (!response.ok) {
        throw new Error(`Failed to fetch hosts: ${response.status}`);
      }

      const data = await response.json();
      this.hosts = data.hosts || [];
    } catch (err) {
      this.error = err instanceof Error ? err.message : 'Failed to fetch hosts';
      console.error('Error fetching hosts:', err);
    } finally {
      this.loading = false;
    }
  }

  addHost(host: Host) {
    // Avoid duplicates
    if (!this.hosts.find(h => h.id === host.id)) {
      this.hosts = [...this.hosts, host];
    }
  }

  updateHost(updatedHost: Host) {
    this.hosts = this.hosts.map(host =>
      host.id === updatedHost.id ? updatedHost : host
    );
  }

  setCurrentHost(hostId: number | null) {
    this.currentHostId = hostId;
  }

  get currentHost(): Host | null {
    return this.hosts.find(h => h.id === this.currentHostId) || null;
  }

  get onlineHosts(): Host[] {
    return this.hosts.filter(h => h.status === 'online');
  }

  get offlineHosts(): Host[] {
    return this.hosts.filter(h => h.status === 'offline');
  }
}

export const hostsStore = new HostsStore();
