<script lang="ts">
  import { onMount } from 'svelte';
  import { hostsStore } from '$lib/stores/hosts.svelte';
  import { filtersStore } from '$lib/stores/filters.svelte';
  import type { Host } from '$lib/types';

  let showAddHost = $state(false);
  let newHostName = $state('');
  let newHostTags = $state('');

  function getStatusColor(status: string): string {
    switch (status) {
      case 'online':
        return 'bg-green-500';
      case 'offline':
        return 'bg-gray-400';
      case 'degraded':
        return 'bg-yellow-500';
      default:
        return 'bg-gray-300';
    }
  }

  function formatRelativeTime(timestamp: string): string {
    const date = new Date(timestamp);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);

    if (diffMins < 1) return 'just now';
    if (diffMins < 60) return `${diffMins}m ago`;
    if (diffHours < 24) return `${diffHours}h ago`;
    if (diffDays < 7) return `${diffDays}d ago`;
    return date.toLocaleDateString();
  }

  function filterByHost(hostId: number) {
    filtersStore.setHost(hostId);
  }

  function clearHostFilter() {
    filtersStore.setHost(undefined);
  }

  async function addHost() {
    if (!newHostName.trim()) return;

    const tags = newHostTags
      .split(',')
      .map(tag => tag.trim())
      .filter(tag => tag.length > 0);

    try {
      const response = await fetch('/api/v1/hosts', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: newHostName.trim(),
          tags: tags
        })
      });

      if (!response.ok) {
        throw new Error('Failed to create host');
      }

      const host = await response.json();
      hostsStore.addHost(host);

      // Reset form
      newHostName = '';
      newHostTags = '';
      showAddHost = false;

      // Show API key to user
      alert(`Host created! API Key: ${host.api_key}\n\nSave this key securely - it won't be shown again.`);
    } catch (error) {
      console.error('Error creating host:', error);
      alert('Failed to create host');
    }
  }

  // Sort hosts by status (online first) and then by last seen
  let hosts = $derived([...hostsStore.hosts].sort((a, b) => {
    // Online hosts first
    if (a.status === 'online' && b.status !== 'online') return -1;
    if (a.status !== 'online' && b.status === 'online') return 1;

    // Then by last seen (newest first)
    return new Date(b.last_seen).getTime() - new Date(a.last_seen).getTime();
  }));

  onMount(() => {
    hostsStore.fetchHosts();
  });
</script>

<div class="bg-white border-r border-gray-200 w-80 flex flex-col">
  <!-- Header -->
  <div class="p-4 border-b border-gray-200">
    <div class="flex items-center justify-between">
      <h2 class="text-lg font-medium text-gray-900">
        Hosts
        {#if hosts.length > 0}
          <span class="ml-2 text-sm text-gray-500">
            ({hosts.length})
          </span>
        {/if}
      </h2>
      <button
        class="px-2 py-1 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 transition-colors"
        onclick={() => showAddHost = !showAddHost}>
        +
      </button>
    </div>

    <!-- Add host form -->
    {#if showAddHost}
      <div class="mt-3 p-3 bg-gray-50 rounded">
        <input
          type="text"
          bind:value={newHostName}
          placeholder="Host name"
          class="w-full px-2 py-1 text-sm border border-gray-300 rounded mb-2"
        />
        <input
          type="text"
          bind:value={newHostTags}
          placeholder="Tags (comma-separated)"
          class="w-full px-2 py-1 text-sm border border-gray-300 rounded mb-2"
        />
        <div class="flex space-x-2">
          <button
            class="flex-1 px-2 py-1 text-sm bg-blue-600 text-white rounded hover:bg-blue-700"
            onclick={addHost}>
            Add
          </button>
          <button
            class="px-2 py-1 text-sm bg-gray-300 text-gray-700 rounded hover:bg-gray-400"
            onclick={() => showAddHost = false}>
            Cancel
          </button>
        </div>
      </div>
    {/if}

    <!-- Current filter indicator -->
    {#if filtersStore.filters.host_id}
      {@const filteredHost = hosts.find(h => h.id === filtersStore.filters.host_id)}
      {#if filteredHost}
        <div class="mt-2 p-2 bg-blue-50 rounded">
          <div class="flex items-center justify-between">
            <span class="text-sm text-blue-800">
              Filtered by: {filteredHost.name}
            </span>
            <button
              class="text-xs text-blue-600 hover:text-blue-800"
              onclick={clearHostFilter}>
              Clear
            </button>
          </div>
        </div>
      {/if}
    {/if}
  </div>

  <!-- Host list -->
  <div class="flex-1 overflow-y-auto">
    {#if hostsStore.loading}
      <div class="p-4 text-center text-gray-500">
        <div class="w-6 h-6 border-2 border-gray-300 border-t-blue-600 rounded-full animate-spin mx-auto"></div>
        <p class="mt-2 text-sm">Loading hosts...</p>
      </div>
    {/if}

    {#if hosts.length === 0 && !hostsStore.loading}
      <div class="p-4 text-center text-gray-500">
        <p>No hosts registered.</p>
        <p class="text-sm mt-1">Add a host to start sending logs.</p>
      </div>
    {/if}

    {#each hosts as host}
      <div
        role="button"
        tabindex="0"
        class="p-3 border-b border-gray-100 hover:bg-gray-50 cursor-pointer transition-colors {filtersStore.filters.host_id === host.id ? 'bg-blue-50' : ''}"
        onclick={() => filterByHost(host.id)}
        onkeydown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            filterByHost(host.id);
          }
        }}
        aria-label="Filter logs by host {host.name}">

        <!-- Host name and status -->
        <div class="flex items-center justify-between mb-2">
          <div class="flex items-center space-x-2">
            <div class="w-2 h-2 rounded-full {getStatusColor(host.status)}"></div>
            <h3 class="font-medium text-gray-900">{host.name}</h3>
          </div>
          <span class="text-xs text-gray-500">
            {formatRelativeTime(host.last_seen)}
          </span>
        </div>

        <!-- Tags -->
        {#if host.tags && host.tags.length > 0}
          <div class="flex flex-wrap gap-1 mb-2">
            {#each host.tags as tag}
              <span class="px-2 py-0.5 text-xs bg-gray-100 text-gray-700 rounded-full">
                {tag}
              </span>
            {/each}
          </div>
        {/if}

        <!-- Stats -->
        <div class="grid grid-cols-3 gap-2 text-xs text-gray-600">
          <div class="text-center">
            <div class="font-medium">{host.log_count}</div>
            <div>logs</div>
          </div>
          <div class="text-center">
            <div class="font-medium text-red-600">{host.error_count}</div>
            <div>errors</div>
          </div>
          <div class="text-center">
            <div class="font-medium">{host.error_rate}%</div>
            <div>error rate</div>
          </div>
        </div>

        <!-- Error rate bar -->
        {#if host.error_rate > 0}
          <div class="mt-2 h-1 bg-gray-200 rounded-full overflow-hidden">
            <div
              class="h-full bg-red-500 transition-all"
              style="width: {Math.min(host.error_rate, 100)}%"></div>
          </div>
        {/if}
      </div>
    {/each}

    {#if hostsStore.error}
      <div class="p-4 bg-red-50 border border-red-200 rounded m-2">
        <p class="text-red-800 text-sm">{hostsStore.error}</p>
      </div>
    {/if}
  </div>

  <!-- Summary stats -->
  {#if hosts.length > 0}
    <div class="p-4 border-t border-gray-200 bg-gray-50">
      <div class="grid grid-cols-4 gap-2 text-center text-xs">
        <div>
          <div class="font-medium text-gray-900">{hosts.length}</div>
          <div class="text-gray-600">Total</div>
        </div>
        <div>
          <div class="font-medium text-green-600">
            {hosts.filter(h => h.status === 'online').length}
          </div>
          <div class="text-gray-600">Online</div>
        </div>
        <div>
          <div class="font-medium text-gray-600">
            {hosts.filter(h => h.status === 'offline').length}
          </div>
          <div class="text-gray-600">Offline</div>
        </div>
        <div>
          <div class="font-medium text-yellow-600">
            {hosts.filter(h => h.status === 'degraded').length}
          </div>
          <div class="text-gray-600">Degraded</div>
        </div>
      </div>
    </div>
  {/if}
</div>
