<script lang="ts">
  import { inboxStore } from '$lib/stores/inbox.svelte';
  import type { InboxItem } from '$lib/types';

  let title = $state('');
  let body = $state('');
  let kind = $state<InboxItem['kind']>('task');
  let priority = $state<InboxItem['priority']>('normal');
  let repo = $state('');
  let sessionId = $state('');
  let saving = $state(false);

  const statuses: Array<{ value: string; label: string }> = [
    { value: 'open', label: 'Open' },
    { value: 'in_progress', label: 'In Progress' },
    { value: 'blocked', label: 'Blocked' },
    { value: 'done', label: 'Done' },
    { value: '', label: 'All' }
  ];

  async function addItem() {
    if (!title.trim() || saving) return;
    saving = true;
    try {
      await inboxStore.createItem({
        title: title.trim(),
        body: body.trim(),
        kind,
        priority,
        repo: repo.trim(),
        session_id: sessionId.trim(),
        source: 'agent'
      });
      title = '';
      body = '';
    } catch (err) {
      inboxStore.error = err instanceof Error ? err.message : 'Failed to create inbox item';
    } finally {
      saving = false;
    }
  }

  function formatTime(value: string): string {
    try {
      return new Date(value).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
    } catch {
      return value;
    }
  }

  function priorityClass(value: string): string {
    if (value === 'high') return 'bg-red-100 text-red-700';
    if (value === 'low') return 'bg-gray-100 text-gray-600';
    return 'bg-blue-100 text-blue-700';
  }
</script>

<div class="flex h-full min-w-0 flex-col bg-gray-50">
  <div class="border-b border-gray-200 bg-white px-3 py-3 md:px-4">
    <div class="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
      <div class="min-w-0">
        <h2 class="text-lg font-medium text-gray-900">Agent Inbox</h2>
        <p class="text-sm text-gray-500">Notes, tasks, questions, and handoffs linked to debugging work.</p>
      </div>
      <div class="flex gap-2 overflow-x-auto pb-1 lg:pb-0">
        {#each statuses as status}
          <button
            class="shrink-0 rounded-md px-3 py-1.5 text-sm font-medium transition-colors {inboxStore.status === status.value ? 'bg-blue-600 text-white' : 'bg-gray-100 text-gray-700 hover:bg-gray-200'}"
            onclick={() => inboxStore.fetchItems(status.value)}>
            {status.label}
          </button>
        {/each}
      </div>
    </div>
  </div>

  <div class="grid min-h-0 flex-1 gap-4 overflow-y-auto p-3 lg:grid-cols-[22rem_minmax(0,1fr)] lg:p-4">
    <form class="rounded-md border border-gray-200 bg-white p-3" onsubmit={(e) => { e.preventDefault(); addItem(); }}>
      <div class="mb-3 text-sm font-semibold text-gray-900">New item</div>
      <input
        class="mb-2 h-9 w-full rounded-md border border-gray-300 px-3 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
        bind:value={title}
        placeholder="Title"
      />
      <textarea
        class="mb-2 min-h-24 w-full resize-y rounded-md border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
        bind:value={body}
        placeholder="Details, evidence, or next step">
      </textarea>
      <div class="mb-2 grid grid-cols-2 gap-2">
        <select class="h-9 rounded-md border border-gray-300 px-2 text-sm" bind:value={kind}>
          <option value="task">Task</option>
          <option value="note">Note</option>
          <option value="question">Question</option>
          <option value="bug">Bug</option>
          <option value="handoff">Handoff</option>
        </select>
        <select class="h-9 rounded-md border border-gray-300 px-2 text-sm" bind:value={priority}>
          <option value="normal">Normal</option>
          <option value="high">High</option>
          <option value="low">Low</option>
        </select>
      </div>
      <input
        class="mb-2 h-9 w-full rounded-md border border-gray-300 px-3 text-sm"
        bind:value={repo}
        placeholder="Repo or workspace"
      />
      <input
        class="mb-3 h-9 w-full rounded-md border border-gray-300 px-3 text-sm"
        bind:value={sessionId}
        placeholder="Session ID"
      />
      <button
        class="h-9 w-full rounded-md bg-blue-600 px-3 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50"
        disabled={!title.trim() || saving}>
        {saving ? 'Adding...' : 'Add Item'}
      </button>
    </form>

    <div class="min-w-0 rounded-md border border-gray-200 bg-white">
      <div class="flex items-center justify-between border-b border-gray-200 px-3 py-2">
        <div class="text-sm font-semibold text-gray-900">{inboxStore.total} item{inboxStore.total === 1 ? '' : 's'}</div>
        {#if inboxStore.loading}
          <div class="h-4 w-4 animate-spin rounded-full border-2 border-gray-300 border-t-blue-600"></div>
        {/if}
      </div>

      {#if inboxStore.error}
        <div class="m-3 rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-800">{inboxStore.error}</div>
      {/if}

      {#if inboxStore.items.length === 0 && !inboxStore.loading}
        <div class="p-8 text-center text-sm text-gray-500">No inbox items for this view.</div>
      {/if}

      <div class="divide-y divide-gray-100">
        {#each inboxStore.items as item}
          <article class="p-3 hover:bg-gray-50">
            <div class="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <h3 class="min-w-0 truncate font-medium text-gray-950">{item.title}</h3>
                  <span class="rounded px-2 py-0.5 text-xs font-medium {priorityClass(item.priority)}">{item.priority}</span>
                  <span class="rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-600">{item.kind}</span>
                </div>
                {#if item.body}
                  <p class="mt-1 whitespace-pre-wrap text-sm text-gray-700">{item.body}</p>
                {/if}
                <div class="mt-2 flex flex-wrap gap-2 text-xs text-gray-500">
                  {#if item.repo}<span>repo: {item.repo}</span>{/if}
                  {#if item.session_id}<span>session: {item.session_id}</span>{/if}
                  {#if item.source}<span>source: {item.source}</span>{/if}
                  <span>updated {formatTime(item.updated_at)}</span>
                </div>
              </div>
              <div class="flex shrink-0 gap-2">
                {#if item.status !== 'in_progress'}
                  <button
                    class="rounded-md bg-gray-100 px-2 py-1 text-xs text-gray-700 hover:bg-gray-200"
                    onclick={() => inboxStore.updateStatus(item, 'in_progress')}>
                    Start
                  </button>
                {/if}
                {#if item.status !== 'done'}
                  <button
                    class="rounded-md bg-green-100 px-2 py-1 text-xs font-medium text-green-800 hover:bg-green-200"
                    onclick={() => inboxStore.updateStatus(item, 'done')}>
                    Done
                  </button>
                {/if}
              </div>
            </div>
          </article>
        {/each}
      </div>
    </div>
  </div>
</div>
