<script lang="ts">
  import { page } from '$app/state';
  import { filtersStore } from '$lib/stores/filters.svelte';
  import FilterBar from '$lib/components/FilterBar.svelte';
  import HostList from '$lib/components/hosts/HostList.svelte';
  import InboxPanel from '$lib/components/inbox/InboxPanel.svelte';
  import LogViewer from '$lib/components/logs/LogViewer.svelte';
  import StatsPanel from '$lib/components/stats/StatsPanel.svelte';

  let showMobileHosts = $state(false);
  let view = $state<'logs' | 'inbox'>('logs');

  // Keep filters in sync with client-side navigation and browser history.
  $effect(() => {
    page.url.href;
    filtersStore.loadFromURL();
  });

  // Dashboard is ready - stores and SSE are automatically initialized
</script>

<div class="flex h-screen min-w-0 bg-gray-50">
  <!-- Desktop hosts sidebar -->
  <aside class="hidden flex-shrink-0 lg:block">
    <HostList />
  </aside>

  <!-- Main content -->
  <div class="flex min-w-0 flex-1 flex-col">
    <!-- Mobile app bar -->
    <div class="flex items-center justify-between border-b border-gray-200 bg-white px-3 py-2 lg:hidden">
      <div>
        <div class="text-sm font-semibold text-gray-950">GLog</div>
        <div class="text-xs text-gray-500">Local debugging workspace</div>
      </div>
      <button
        class="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700"
        onclick={() => showMobileHosts = true}>
        Hosts
      </button>
    </div>

    <div class="flex items-center gap-2 border-b border-gray-200 bg-white px-3 py-2 md:px-4">
      <button
        class="rounded-md px-3 py-1.5 text-sm font-medium {view === 'logs' ? 'bg-blue-600 text-white' : 'text-gray-700 hover:bg-gray-100'}"
        onclick={() => view = 'logs'}>
        Logs
      </button>
      <button
        class="rounded-md px-3 py-1.5 text-sm font-medium {view === 'inbox' ? 'bg-blue-600 text-white' : 'text-gray-700 hover:bg-gray-100'}"
        onclick={() => view = 'inbox'}>
        Inbox
      </button>
    </div>

    {#if view === 'logs'}
      <!-- Filter bar -->
      <FilterBar />

      <!-- Aggregate stats -->
      <StatsPanel />

      <!-- Log viewer -->
      <div class="flex-1 min-h-0">
        <LogViewer />
      </div>
    {:else}
      <div class="min-h-0 flex-1">
        <InboxPanel />
      </div>
    {/if}
  </div>
</div>

{#if showMobileHosts}
  <div class="fixed inset-0 z-40 lg:hidden">
    <button
      class="absolute inset-0 bg-gray-950/30"
      aria-label="Close hosts drawer"
      onclick={() => showMobileHosts = false}>
    </button>
    <aside class="absolute inset-y-0 left-0 w-[min(22rem,90vw)] bg-white shadow-xl">
      <div class="flex items-center justify-between border-b border-gray-200 px-4 py-3">
        <div class="text-sm font-semibold text-gray-950">Hosts</div>
        <button
          class="rounded-md px-2 py-1 text-sm text-gray-600 hover:bg-gray-100"
          onclick={() => showMobileHosts = false}>
          Close
        </button>
      </div>
      <div class="h-[calc(100%-49px)]">
        <HostList />
      </div>
    </aside>
  </div>
{/if}

<!-- Global styles for the app -->
<style>
  :global(html, body) {
    height: 100%;
    overflow: hidden;
  }
</style>
