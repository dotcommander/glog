<script lang="ts">
  import FilterBar from '$lib/components/FilterBar.svelte';
  import HostList from '$lib/components/hosts/HostList.svelte';
  import LogViewer from '$lib/components/logs/LogViewer.svelte';
  import { logsStore } from '$lib/stores/logs.svelte';
  import { sseStore } from '$lib/stores/sse.svelte';

  // Dashboard is ready - stores and SSE are automatically initialized
</script>

<div class="flex h-screen bg-gray-50">
  <!-- Hosts sidebar -->
  <div class="flex-shrink-0">
    <HostList />
  </div>

  <!-- Main content -->
  <div class="flex-1 flex flex-col">
    <!-- Filter bar -->
    <FilterBar />

    <!-- Log viewer -->
    <div class="flex-1 min-h-0">
      <LogViewer />
    </div>
  </div>
</div>

<!-- Global styles for the app -->
<style>
  :global(html, body) {
    height: 100%;
    overflow: hidden;
  }
</style>
