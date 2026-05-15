import { browser } from '$app/environment';
import type { InboxItem } from '$lib/types';

class InboxStore {
  items = $state<InboxItem[]>([]);
  loading = $state(false);
  error = $state<string | null>(null);
  status = $state('open');
  total = $state(0);

  constructor() {
    if (browser) {
      this.fetchItems();
    }
  }

  async fetchItems(status = this.status) {
    this.loading = true;
    this.error = null;
    this.status = status;

    try {
      const params = new URLSearchParams();
      if (status) {
        params.set('status', status);
      }
      params.set('limit', '100');
      const response = await fetch(`/api/v1/inbox?${params.toString()}`);
      if (!response.ok) {
        throw new Error(`Failed to fetch inbox: ${response.status}`);
      }
      const data = await response.json();
      this.items = data.items || [];
      this.total = data.total || 0;
    } catch (err) {
      this.error = err instanceof Error ? err.message : 'Failed to fetch inbox';
      console.error('Error fetching inbox:', err);
    } finally {
      this.loading = false;
    }
  }

  async createItem(input: Partial<InboxItem> & { title: string }) {
    this.error = null;
    const response = await fetch('/api/v1/inbox', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input)
    });
    if (!response.ok) {
      throw new Error(`Failed to create inbox item: ${response.status}`);
    }
    const item = await response.json();
    if (!this.status || item.status === this.status) {
      this.items = [item, ...this.items];
    }
    this.total += 1;
    return item;
  }

  async updateStatus(item: InboxItem, status: InboxItem['status']) {
    const response = await fetch(`/api/v1/inbox/${item.id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ...item, status })
    });
    if (!response.ok) {
      throw new Error(`Failed to update inbox item: ${response.status}`);
    }
    const updated = await response.json();
    if (this.status && updated.status !== this.status) {
      this.items = this.items.filter(existing => existing.id !== updated.id);
    } else {
      this.items = this.items.map(existing => existing.id === updated.id ? updated : existing);
    }
    return updated;
  }
}

export const inboxStore = new InboxStore();
