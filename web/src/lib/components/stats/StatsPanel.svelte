<script lang="ts">
  import { statsStore } from '$lib/stores/stats.svelte';
  import { filtersStore } from '$lib/stores/filters.svelte';

  const levelStyles: Record<string, string> = {
    trace: 'bg-violet-500',
    debug: 'bg-sky-500',
    info: 'bg-emerald-500',
    warn: 'bg-amber-500',
    error: 'bg-rose-500',
    fatal: 'bg-neutral-900'
  };

  let maxHourlyCount = $derived(
    Math.max(1, ...statsStore.hourlyTotals.map(hour => hour.count))
  );

  function formatHour(value: string): string {
    try {
      return new Date(value).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    } catch {
      return value;
    }
  }

  function filterFingerprint(fingerprint: string) {
    filtersStore.setSearch(fingerprint);
  }

  $effect(() => {
    const filters = filtersStore.filters;
    statsStore.fetchStats(filters);
  });
</script>

<section class="border-b border-gray-200 bg-white px-3 py-3 md:px-4">
  <div class="grid min-w-0 gap-3 xl:grid-cols-[minmax(0,1fr)_minmax(22rem,28rem)]">
    <div class="grid min-w-0 gap-3 sm:grid-cols-2">
      <div class="min-w-0 rounded-md border border-gray-200 bg-gray-50 px-3 py-2">
        <div class="text-xs font-medium uppercase tracking-wide text-gray-500">24h logs</div>
        <div class="mt-1 flex items-end gap-2">
          <div class="text-2xl font-semibold tabular-nums text-gray-950">{statsStore.totalLogs}</div>
          {#if statsStore.loading}
            <div class="mb-1 h-3 w-3 animate-spin rounded-full border-2 border-gray-300 border-t-blue-600"></div>
          {/if}
        </div>
      </div>

      <div class="min-w-0 rounded-md border border-gray-200 bg-gray-50 px-3 py-2">
        <div class="text-xs font-medium uppercase tracking-wide text-gray-500">error rate</div>
        <div class="mt-1 flex flex-wrap items-baseline gap-x-2 gap-y-1">
          <div class="text-2xl font-semibold tabular-nums text-gray-950">{statsStore.errorRate}%</div>
          <div class="text-xs text-gray-500">{statsStore.errorLogs} events</div>
        </div>
      </div>

      <div class="min-w-0 rounded-md border border-gray-200 bg-gray-50 px-3 py-2 sm:col-span-2">
        <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
          <div class="text-xs font-medium uppercase tracking-wide text-gray-500">levels</div>
          {#if statsStore.error}
            <div class="text-xs text-rose-600">stats unavailable</div>
          {/if}
        </div>
        <div class="grid grid-cols-3 gap-2 sm:grid-cols-6">
          {#each statsStore.levelTotals as item}
            <div class="min-w-0">
              <div class="mb-1 h-1.5 rounded-full bg-gray-200">
                <div
                  class="h-full rounded-full {levelStyles[item.level]}"
                  style="width: {statsStore.totalLogs ? Math.max(4, (item.count / statsStore.totalLogs) * 100) : 0}%">
                </div>
              </div>
              <div class="truncate text-[11px] font-medium uppercase text-gray-500">{item.level}</div>
              <div class="text-sm font-semibold tabular-nums text-gray-900">{item.count}</div>
            </div>
          {/each}
        </div>
      </div>
    </div>

    <div class="grid min-w-0 gap-3 md:grid-cols-2 xl:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)]">
      <div class="min-w-0 rounded-md border border-gray-200 bg-gray-50 px-3 py-2">
        <div class="mb-2 text-xs font-medium uppercase tracking-wide text-gray-500">hourly volume</div>
        <div class="flex h-16 items-end gap-1">
          {#if statsStore.hourlyTotals.length === 0}
            <div class="flex h-full w-full items-center text-xs text-gray-400">No aggregate data yet</div>
          {:else}
            {#each statsStore.hourlyTotals as hour}
              <div class="group relative flex min-w-1 flex-1 items-end">
                <div
                  class="w-full rounded-t bg-blue-500"
                  style="height: {Math.max(4, (hour.count / maxHourlyCount) * 100)}%">
                </div>
                <div class="pointer-events-none absolute bottom-full left-1/2 z-10 mb-1 hidden -translate-x-1/2 whitespace-nowrap rounded bg-gray-950 px-2 py-1 text-xs text-white group-hover:block">
                  {formatHour(hour.hour)} · {hour.count}
                </div>
              </div>
            {/each}
          {/if}
        </div>
      </div>

      <div class="min-w-0 rounded-md border border-gray-200 bg-gray-50 px-3 py-2">
        <div class="mb-2 text-xs font-medium uppercase tracking-wide text-gray-500">top fingerprints</div>
        <div class="space-y-1">
          {#if statsStore.topFingerprints.length === 0}
            <div class="text-xs text-gray-400">No repeated patterns yet</div>
          {:else}
            {#each statsStore.topFingerprints as item}
              <button
                class="flex w-full items-center justify-between gap-2 rounded px-1 py-0.5 text-left hover:bg-white"
                title="Filter logs by fingerprint"
                onclick={() => filterFingerprint(item.fingerprint)}>
                <span class="truncate font-mono text-xs text-gray-700">{item.fingerprint}</span>
                <span class="shrink-0 text-xs font-semibold tabular-nums text-gray-900">{item.count}</span>
              </button>
            {/each}
          {/if}
        </div>
      </div>
    </div>
  </div>
</section>
