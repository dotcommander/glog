<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { browser } from '$app/environment';
  import { logsStore } from '$lib/stores/logs.svelte';
  import { filtersStore } from '$lib/stores/filters.svelte';
  import { sseStore } from '$lib/stores/sse.svelte';
  import LogEntry from './LogEntry.svelte';

  let loadingMore = $state(false);
  let container: HTMLElement;

  // Auto-scroll to top when new logs arrive if we're near the top
  let nearTop = $state(true);
  let lastScrollTop = 0;

  function handleScroll() {
    if (!container) return;

    const scrollTop = container.scrollTop;
    const scrollHeight = container.scrollHeight;
    const clientHeight = container.clientHeight;

    // Check if we're near the top (within 100px)
    nearTop = scrollTop < 100;

    // Check if we're near the bottom (infinite scroll)
    const nearBottom = scrollTop + clientHeight >= scrollHeight - 100;

    if (nearBottom && !loadingMore && logsStore.hasMore) {
      loadMore();
    }

    lastScrollTop = scrollTop;
  }

  async function loadMore() {
    if (loadingMore || !logsStore.hasMore) return;

    loadingMore = true;
    await logsStore.loadMore(filtersStore.filters);
    loadingMore = false;
  }

  function scrollToTop() {
    if (container) {
      container.scrollTo({ top: 0, behavior: 'smooth' });
    }
  }

  // Watch for new logs and scroll to top if we're near the top
  $effect(() => {
    if (logsStore.logs.length > 0 && nearTop) {
      // Small delay to ensure DOM is updated
      setTimeout(() => {
        if (container && nearTop) {
          container.scrollTop = 0;
        }
      }, 50);
    }
  });

  // Watch for filter changes and refetch logs
  $effect(() => {
    // This will re-run when any filter changes
    const filters = filtersStore.filters;
    logsStore.fetchLogs(filters);
  });

  // Manual refresh handler
  async function refresh() {
    await logsStore.fetchLogs(filtersStore.filters);
  }

  // Listen for filter-by-host events
  function handleFilterByHost(event: CustomEvent) {
    filtersStore.setHost(event.detail.hostId);
  }

  onMount(() => {
    if (browser) {
      window.addEventListener('filter-by-host', handleFilterByHost as EventListener);
    }
  });

  onDestroy(() => {
    if (browser) {
      window.removeEventListener('filter-by-host', handleFilterByHost as EventListener);
    }
  });
</script>

<div class="flex h-full min-w-0 flex-col">
  <!-- Header with stats -->
  <div class="border-b border-gray-200 bg-white px-3 py-3 md:px-4">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div class="flex min-w-0 flex-wrap items-center gap-3">
        <h2 class="text-lg font-medium text-gray-900">
          Logs
          {#if logsStore.total > 0}
            <span class="ml-2 text-sm text-gray-500">
              ({logsStore.total} total)
            </span>
          {/if}
        </h2>

        {#if filtersStore.activeFilterCount > 0}
          <span class="px-2 py-1 text-xs font-medium bg-blue-100 text-blue-800 rounded-full">
            {filtersStore.activeFilterCount} filter{filtersStore.activeFilterCount > 1 ? 's' : ''}
          </span>
        {/if}
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <!-- Connection status -->
        <div class="mr-2 flex items-center gap-2 sm:mr-4">
          {#if sseStore.connected}
            <div class="h-2 w-2 rounded-full bg-green-500"></div>
            <span class="text-xs text-gray-500">Live</span>
          {:else}
            <div class="h-2 w-2 rounded-full bg-yellow-500"></div>
            <span class="text-xs text-gray-500">Disconnected</span>
          {/if}
        </div>

        <!-- Loading indicator -->
        {#if logsStore.loading}
          <div class="h-4 w-4 animate-spin rounded-full border-2 border-gray-300 border-t-blue-600"></div>
        {/if}

        <!-- Scroll to top button -->
        {#if !nearTop && logsStore.logs.length > 0}
          <button
            class="rounded-md bg-gray-100 px-3 py-1 text-xs text-gray-700 transition-colors hover:bg-gray-200"
            onclick={scrollToTop}>
            Scroll to Top
          </button>
        {/if}

        <!-- Refresh button -->
        <button
          class="rounded-md bg-blue-100 px-3 py-1 text-xs font-medium text-blue-800 transition-colors hover:bg-blue-200"
          onclick={refresh}
          disabled={logsStore.loading}>
          Refresh
        </button>
      </div>
    </div>
  </div>

  <!-- Logs list -->
  <div
    bind:this={container}
    class="min-w-0 flex-1 overflow-y-auto"
    onscroll={handleScroll}>

    {#if logsStore.error}
      <div class="m-4 rounded-md border border-red-200 bg-red-50 p-4">
        <p class="text-red-800">{logsStore.error}</p>
        <button
          class="mt-2 rounded-md bg-red-100 px-3 py-1 text-sm text-red-800 hover:bg-red-200"
          onclick={refresh}>
          Retry
        </button>
      </div>
    {/if}

    {#if logsStore.logs.length === 0 && !logsStore.loading}
      <div class="p-8 text-center text-gray-500">
        {#if filtersStore.hasActiveFilters}
          <p>No logs match your filters.</p>
          <button
            class="mt-2 text-blue-600 hover:text-blue-800"
            onclick={() => filtersStore.clearFilters()}>
            Clear filters
          </button>
        {:else}
          <p>No logs yet.</p>
          <p class="text-sm mt-2">Send some logs to see them here.</p>
        {/if}
      </div>
    {/if}

    {#each logsStore.logs as log (log.id)}
      <LogEntry {log} />
    {/each}

    <!-- Loading more indicator -->
    {#if loadingMore}
      <div class="p-4 text-center text-gray-500">
        <div class="mx-auto h-6 w-6 animate-spin rounded-full border-2 border-gray-300 border-t-blue-600"></div>
        <p class="mt-2 text-sm">Loading more logs...</p>
      </div>
    {/if}

    <!-- End of logs -->
    {#if !logsStore.hasMore && logsStore.logs.length > 0}
      <div class="p-4 text-center text-gray-400 text-sm">
        End of logs
      </div>
    {/if}
  </div>
</div>
