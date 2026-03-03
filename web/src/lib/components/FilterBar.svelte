<script lang="ts">
  import { filtersStore } from '$lib/stores/filters.svelte';
  import { hostsStore } from '$lib/stores/hosts.svelte';

  let searchInput = $state(filtersStore.filters.search || '');
  let showDatePicker = $state(false);
  let startDate = $state(filtersStore.filters.start_date || '');
  let endDate = $state(filtersStore.filters.end_date || '');

  const levels = [
    { value: 'trace', label: 'Trace', color: 'purple' },
    { value: 'debug', label: 'Debug', color: 'blue' },
    { value: 'info', label: 'Info', color: 'green' },
    { value: 'warn', label: 'Warn', color: 'yellow' },
    { value: 'error', label: 'Error', color: 'red' },
    { value: 'fatal', label: 'Fatal', color: 'gray' }
  ];

  function applySearch() {
    filtersStore.setSearch(searchInput);
  }

  function clearSearch() {
    searchInput = '';
    filtersStore.setSearch('');
  }

  function toggleLevel(level: string) {
    if (filtersStore.filters.level?.includes(level)) {
      filtersStore.removeLevel(level);
    } else {
      filtersStore.addLevel(level);
    }
  }

  function applyDateFilter() {
    filtersStore.setDateRange(startDate || undefined, endDate || undefined);
    showDatePicker = false;
  }

  function clearDateFilter() {
    startDate = '';
    endDate = '';
    filtersStore.setDateRange(undefined, undefined);
    showDatePicker = false;
  }

  // Keyboard shortcut: Ctrl/Cmd + K to focus search
  function handleKeydown(event: KeyboardEvent) {
    if ((event.ctrlKey || event.metaKey) && event.key === 'k') {
      event.preventDefault();
      const searchInput = document.getElementById('search-input') as HTMLInputElement;
      searchInput?.focus();
    }

    // Escape to clear search when focused
    if (event.key === 'Escape' && document.activeElement?.id === 'search-input') {
      clearSearch();
      (document.activeElement as HTMLElement)?.blur();
    }
  }

  // Focus search on mount
  import { onMount } from 'svelte';
  onMount(() => {
    document.addEventListener('keydown', handleKeydown);
    return () => document.removeEventListener('keydown', handleKeydown);
  });
</script>

<div class="bg-white border-b border-gray-200 p-4">
  <div class="flex flex-col space-y-4">
    <!-- Search and main controls -->
    <div class="flex items-center space-x-4">
      <!-- Search -->
      <div class="flex-1 relative">
        <input
          id="search-input"
          type="text"
          bind:value={searchInput}
          oninput={applySearch}
          placeholder="Search logs... (Ctrl+K)"
          class="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
        />
        {#if searchInput}
          <button
            class="absolute right-2 top-2 text-gray-400 hover:text-gray-600"
            onclick={clearSearch}>
            ✕
          </button>
        {/if}
      </div>

      <!-- Host selector -->
      <select
        class="px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
        value={filtersStore.filters.host_id || ''}
        onchange={(e) => {
          const value = (e.target as HTMLSelectElement).value;
          filtersStore.setHost(value ? parseInt(value) : undefined);
        }}>
        <option value="">All Hosts</option>
        {#each hostsStore.hosts as host}
          <option value={host.id}>{host.name}</option>
        {/each}
      </select>

      <!-- Date range picker -->
      <div class="relative">
        <button
          class="px-3 py-2 text-sm bg-gray-100 hover:bg-gray-200 rounded-md transition-colors"
          onclick={() => showDatePicker = !showDatePicker}>
          📅
          {#if filtersStore.filters.start_date || filtersStore.filters.end_date}
            <span class="ml-2 text-xs bg-blue-500 text-white px-2 py-0.5 rounded-full">
              1
            </span>
          {/if}
        </button>

        {#if showDatePicker}
          <div class="absolute top-10 right-0 bg-white border border-gray-200 rounded-lg shadow-lg p-4 z-10 min-w-80">
            <div class="space-y-3">
              <div>
                <label for="start-date" class="block text-sm font-medium text-gray-700 mb-1">From</label>
                <input
                  id="start-date"
                  type="datetime-local"
                  bind:value={startDate}
                  class="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
              </div>
              <div>
                <label for="end-date" class="block text-sm font-medium text-gray-700 mb-1">To</label>
                <input
                  id="end-date"
                  type="datetime-local"
                  bind:value={endDate}
                  class="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                />
              </div>
              <div class="flex space-x-2">
                <button
                  class="flex-1 px-3 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 transition-colors"
                  onclick={applyDateFilter}>
                  Apply
                </button>
                <button
                  class="px-3 py-2 bg-gray-200 text-gray-700 rounded-md hover:bg-gray-300 transition-colors"
                  onclick={clearDateFilter}>
                  Clear
                </button>
              </div>
            </div>
          </div>
        {/if}
      </div>

      <!-- Clear all filters -->
      {#if filtersStore.hasActiveFilters}
        <button
          class="px-3 py-2 text-sm text-gray-600 hover:text-gray-800 transition-colors"
          onclick={() => {
            filtersStore.clearFilters();
            searchInput = '';
            startDate = '';
            endDate = '';
          }}>
          Clear All
        </button>
      {/if}
    </div>

    <!-- Level filters -->
    <div class="flex items-center space-x-2">
      <span class="text-sm text-gray-600 mr-2">Level:</span>
      {#each levels as level}
        <label class="flex items-center space-x-1 cursor-pointer">
          <input
            type="checkbox"
            checked={filtersStore.filters.level?.includes(level.value) || false}
            onchange={() => toggleLevel(level.value)}
            class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
          />
          <span class="text-sm px-2 py-0.5 rounded
            {level.value === 'trace' ? 'bg-purple-100 text-purple-700' :
             level.value === 'debug' ? 'bg-blue-100 text-blue-700' :
             level.value === 'info' ? 'bg-green-100 text-green-700' :
             level.value === 'warn' ? 'bg-yellow-100 text-yellow-700' :
             level.value === 'error' ? 'bg-red-100 text-red-700' :
             'bg-gray-100 text-gray-700'}">
            {level.label}
          </span>
        </label>
      {/each}
    </div>

    <!-- Active filters summary -->
    {#if filtersStore.hasActiveFilters}
      <div class="flex items-center space-x-2 text-sm text-gray-600">
        <span>Active filters:</span>
        <div class="flex items-center space-x-2">
          {#if filtersStore.filters.level && filtersStore.filters.level.length > 0}
            <span class="px-2 py-0.5 bg-gray-100 rounded">
              Level: {filtersStore.filters.level.join(', ')}
            </span>
          {/if}
          {#if filtersStore.filters.search}
            <span class="px-2 py-0.5 bg-gray-100 rounded">
              Search: "{filtersStore.filters.search}"
            </span>
          {/if}
          {#if filtersStore.filters.host_id}
            {@const host = hostsStore.hosts.find(h => h.id === filtersStore.filters.host_id)}
            {#if host}
              <span class="px-2 py-0.5 bg-gray-100 rounded">
                Host: {host.name}
              </span>
            {/if}
          {/if}
          {#if filtersStore.filters.start_date || filtersStore.filters.end_date}
            <span class="px-2 py-0.5 bg-gray-100 rounded">
              Date: {filtersStore.filters.start_date || 'Start'} to {filtersStore.filters.end_date || 'End'}
            </span>
          {/if}
        </div>
      </div>
    {/if}
  </div>
</div>

<!-- Click outside to close date picker -->
{#if showDatePicker}
  <div
    class="fixed inset-0 z-0"
    onclick={() => showDatePicker = false}
    onkeydown={(e) => { if (e.key === 'Escape') showDatePicker = false; }}
    role="button"
    aria-label="Close date picker"
    tabindex="-1">
  </div>
{/if}
