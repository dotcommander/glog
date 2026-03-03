import { browser } from '$app/environment';
import { page } from '$app/stores';
import type { LogFilters } from '$lib/types';

class FiltersStore {
  filters = $state<LogFilters>({
    level: [],
    search: '',
    host_id: undefined,
    start_date: undefined,
    end_date: undefined,
    limit: 50,
    offset: 0
  });

  constructor() {
    if (browser) {
      // Load filters from URL on initialization
      this.loadFromURL();

      // Watch for URL changes
      page.subscribe(($page) => {
        this.loadFromURL();
      });
    }
  }

  loadFromURL() {
    if (!browser) return;

    const params = new URLSearchParams(window.location.search);
    const newFilters: LogFilters = {
      level: params.getAll('level'),
      search: params.get('search') || '',
      host_id: params.get('host_id') ? parseInt(params.get('host_id')!) : undefined,
      start_date: params.get('start_date') || undefined,
      end_date: params.get('end_date') || undefined,
      limit: params.get('limit') ? parseInt(params.get('limit')!) : 50,
      offset: params.get('offset') ? parseInt(params.get('offset')!) : 0
    };

    this.filters = newFilters;
  }

  updateURL() {
    if (!browser) return;

    const params = new URLSearchParams();

    // Add level filters
    this.filters.level?.forEach(level => params.append('level', level));

    // Add search filter
    if (this.filters.search) {
      params.set('search', this.filters.search);
    }

    // Add host filter
    if (this.filters.host_id) {
      params.set('host_id', this.filters.host_id.toString());
    }

    // Add date filters
    if (this.filters.start_date) {
      params.set('start_date', this.filters.start_date);
    }
    if (this.filters.end_date) {
      params.set('end_date', this.filters.end_date);
    }

    // Update URL without reloading
    const newUrl = `${window.location.pathname}?${params.toString()}`;
    window.history.replaceState({}, '', newUrl);
  }

  setFilter<K extends keyof LogFilters>(key: K, value: LogFilters[K]) {
    this.filters[key] = value;
    this.filters.offset = 0; // Reset pagination when filters change
    this.updateURL();
  }

  addLevel(level: string) {
    if (!this.filters.level?.includes(level)) {
      this.filters.level = [...(this.filters.level || []), level];
      this.filters.offset = 0;
      this.updateURL();
    }
  }

  removeLevel(level: string) {
    this.filters.level = this.filters.level?.filter(l => l !== level) || [];
    this.filters.offset = 0;
    this.updateURL();
  }

  setSearch(search: string) {
    this.setFilter('search', search);
  }

  setHost(hostId: number | undefined) {
    this.setFilter('host_id', hostId);
  }

  setDateRange(startDate?: string, endDate?: string) {
    this.filters.start_date = startDate;
    this.filters.end_date = endDate;
    this.filters.offset = 0;
    this.updateURL();
  }

  clearFilters() {
    this.filters = {
      level: [],
      search: '',
      host_id: undefined,
      start_date: undefined,
      end_date: undefined,
      limit: 50,
      offset: 0
    };
    this.updateURL();
  }

  // Check if any filters are active
  get hasActiveFilters(): boolean {
    return (
      (this.filters.level && this.filters.level.length > 0) ||
      !!this.filters.search ||
      !!this.filters.host_id ||
      !!this.filters.start_date ||
      !!this.filters.end_date
    );
  }

  // Get active filter count
  get activeFilterCount(): number {
    let count = 0;
    if (this.filters.level && this.filters.level.length > 0) count++;
    if (this.filters.search) count++;
    if (this.filters.host_id) count++;
    if (this.filters.start_date || this.filters.end_date) count++;
    return count;
  }
}

export const filtersStore = new FiltersStore();
