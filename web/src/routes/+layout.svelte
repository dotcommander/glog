<script lang="ts">
  import '../app.css';
  
  let { children } = $props();
</script>

<div class="min-h-screen bg-gray-50">
  <!-- Header -->
  <header class="bg-white shadow-sm border-b border-gray-200">
    <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
      <div class="flex items-center justify-between h-16">
        <div class="flex items-center">
          <h1 class="text-2xl font-bold text-gray-900">GLog</h1>
          <span class="ml-3 text-sm text-gray-500">Minimalist Log Aggregation</span>
        </div>
        <div class="flex items-center space-x-4">
          <span class="text-sm text-gray-500">Live Updates</span>
          <div id="connection-status" class="w-3 h-3 bg-green-500 rounded-full"></div>
        </div>
      </div>
    </div>
  </header>

  <!-- Main Content -->
  <main class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
    {@render children()}
  </main>
</div>
