<script lang="ts">
  import { format } from 'date-fns';
  import type { Log } from '$lib/types';

  interface Props {
    log: Log;
  }

  let { log }: Props = $props();
  let expanded = $state(false);

  // Format milliseconds
  function formatTime(timestamp: string): string {
    try {
      return format(new Date(timestamp), 'HH:mm:ss.SSS');
    } catch {
      return timestamp.split('T')[1]?.split('.')[0] || timestamp;
    }
  }

  // Get level color classes
  function getLevelClasses(level: string): string {
    const base = 'px-2 py-0.5 text-xs font-mono font-bold rounded uppercase';
    switch (level.toLowerCase()) {
      case 'trace':
        return `${base} bg-purple-100 text-purple-800`;
      case 'debug':
        return `${base} bg-blue-100 text-blue-800`;
      case 'info':
        return `${base} bg-green-100 text-green-800`;
      case 'warn':
        return `${base} bg-yellow-100 text-yellow-800`;
      case 'error':
        return `${base} bg-red-100 text-red-800`;
      case 'fatal':
        return `${base} bg-gray-900 text-white`;
      default:
        return `${base} bg-gray-100 text-gray-800`;
    }
  }

  // Format JSON
  function formatJSON(value: any): string {
    try {
      return JSON.stringify(value, null, 2);
    } catch {
      return String(value);
    }
  }

  // Check if has fields
  function hasFields(): boolean {
    return log.fields && Object.keys(log.fields).length > 0;
  }
</script>

<div class="log-entry group border-b border-gray-100 transition-colors hover:border-gray-300">
  <!-- Main log line -->
  <div
    class="flex cursor-pointer flex-wrap items-start gap-x-3 gap-y-1 px-3 py-2 hover:bg-gray-50 md:flex-nowrap md:px-4 {expanded ? 'bg-gray-50' : ''}"
    onclick={() => expanded = !expanded}
    onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); expanded = !expanded; } }}
    role="button"
    tabindex="0"
    aria-expanded={expanded}>
    <!-- Timestamp -->
    <div class="w-24 flex-shrink-0 font-mono text-xs text-gray-400 md:w-32">{formatTime(log.timestamp)}</div>

    <!-- Level -->
    <div class="w-16 flex-shrink-0">
      <span class={getLevelClasses(log.level)}>{log.level}</span>
    </div>

    <!-- Host and Source -->
    <div class="order-3 min-w-0 flex-1 truncate md:order-none md:w-40 md:flex-none">
      {#if log.host}
        <span class="font-mono text-sm font-semibold text-gray-700">{log.host.name}</span>
      {/if}
      {#if log.derived_source}
        <span class="ml-2 font-mono text-xs text-gray-500">{log.derived_source}</span>
      {/if}
    </div>

    <!-- Message -->
    <div class="order-5 min-w-0 flex-1 basis-full md:order-none md:ml-4 md:basis-auto">
      <div class="truncate whitespace-pre-wrap font-mono text-sm text-gray-800 hover:text-clip">
        {log.message}
      </div>

      <!-- Tags -->
      <div class="mt-1 flex flex-wrap items-center gap-2">
        {#if log.derived_category}
          <span class="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-600">{log.derived_category}</span>
        {/if}
        {#if expanded}
          <span class="text-xs text-gray-400">expanded</span>
        {/if}
      </div>
    </div>

    <!-- Expand arrow -->
    <div class="order-4 w-6 flex-shrink-0 text-center md:order-none">
      <span class="text-xs text-gray-400 transition-transform {expanded ? 'rotate-90' : ''}">▶</span>
    </div>
  </div>

  <!-- Expanded details -->
  {#if expanded}
    <div class="border-t border-gray-200 bg-gray-50 px-3 py-3 md:ml-32 md:px-4">
      <!-- Fields -->
      {#if hasFields()}
        <div class="mb-3">
          <div class="text-xs text-gray-500 font-semibold mb-1">FIELDS</div>
          <pre class="overflow-x-auto rounded-md bg-gray-100 p-2 text-xs text-gray-700">{formatJSON(log.fields)}</pre>
        </div>
      {/if}

      <!-- HTTP Context -->
      {#if log.method || log.path || log.status_code}
        <div class="mb-3">
          <div class="text-xs text-gray-500 font-semibold mb-1">HTTP CONTEXT</div>
          <div class="flex flex-wrap items-center gap-3 text-xs text-gray-600 md:gap-4">
            {#if log.method}
              <span><span class="font-mono font-bold">{log.method}</span> {log.path || ''}</span>
            {/if}
            {#if log.status_code}
              <span>Status: <span class="font-mono font-bold">{log.status_code}</span></span>
            {/if}
            {#if log.duration_ms}
              <span>Duration: <span class="font-mono font-bold">{log.duration_ms}ms</span></span>
            {/if}
          </div>
        </div>
      {/if}

      <!-- Derived Metadata -->
      {#if log.derived_source || log.derived_category}
        <div>
          <div class="text-xs text-gray-500 font-semibold mb-1">DERIVED METADATA</div>
          <div class="flex flex-wrap items-center gap-2 text-xs">
            {#if log.derived_level}
              <span class="rounded bg-blue-100 px-2 py-1 text-blue-800">level: {log.derived_level}</span>
            {/if}
            {#if log.derived_source}
              <span class="rounded bg-violet-100 px-2 py-1 text-violet-800">source: {log.derived_source}</span>
            {/if}
            {#if log.derived_category}
              <span class="rounded bg-gray-100 px-2 py-1 text-gray-800">category: {log.derived_category}</span>
            {/if}
          </div>
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .log-entry:hover .truncate {
    white-space: normal;
    word-break: break-word;
  }
</style>
