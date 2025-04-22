<script lang="ts">
  import { writable } from 'svelte/store';
  // Store for settings
  const settings = writable({
    theme: 'system',
    model: 'claude-3',
    citationStyle: 'inline',
    markdownMode: false,
    enterToSubmit: true
  });

  // Available models
  const models = [
    { id: 'claude-3', name: 'Claude-3', description: '' },
    { id: 'gpt-4', name: 'GPT-4', description: '' },
    { id: 'deepseek-r1', name: 'DeepSeek-R1', description: '' }
  ];

  const themes = ['light', 'dark', 'system'];
  const citationStyles = ['inline', 'academic', 'numbered'];

  function handleThemeChange(theme: string) {
    settings.update((s) => ({ ...s, theme }));
  }

  function handleModelChange(modelId: string) {
    settings.update((s) => ({ ...s, model: modelId }));
  }
</script>

<div class="min-h-screen dark:bg-gray-100">
  <div class="max-w-4xl mx-auto px-4 py-8">
    <!-- Theme Section -->
    <section class="mb-12">
      <h2 class="text-xl font-medium mb-4 text-gray-900 dark:text-white">Appearance</h2>
      <div class="bg-gray-200 dark:bg-gray-800 rounded-lg p-4">
        <div class="flex items-center justify-between">
          <div>
            <h3 class="font-medium text-gray-900 dark:text-white">Theme</h3>
            <p class="text-sm text-gray-500 dark:text-gray-400">Select your preferred theme</p>
          </div>
          <div class="flex gap-2">
            {#each themes as theme}
              <button
                class="px-4 py-2 rounded-md text-sm font-medium
                         {$settings.theme === theme
                  ? 'bg-blue-600 text-white'
                  : 'bg-gray-200 dark:bg-gray-700 text-gray-700 dark:text-gray-300'}"
                on:click={() => handleThemeChange(theme)}
              >
                {theme.charAt(0).toUpperCase() + theme.slice(1)}
              </button>
            {/each}
          </div>
        </div>
      </div>
    </section>

    <!-- Model Selection -->
    <section class="mb-12">
      <h2 class="text-xl font-medium mb-4 text-gray-900 dark:text-white">Model Preferences</h2>
      <div class="space-y-4">
        {#each models as model}
          <div
            class="bg-gray-100 dark:bg-gray-800 rounded-lg p-4 cursor-pointer
                     hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors
                     {$settings.model === model.id ? 'ring-2 ring-blue-600' : ''}"
            on:click={() => handleModelChange(model.id)}
          >
            <div class="flex items-center justify-between">
              <div>
                <h3 class="font-medium text-gray-900 dark:text-white">
                  {model.name}
                </h3>
                <p class="text-sm text-gray-500 dark:text-gray-400">
                  {model.description}
                </p>
              </div>
              <div class="flex items-center">
                <div
                  class="w-4 h-4 rounded-full border-2 border-gray-300 dark:border-gray-600
                            {$settings.model === model.id ? 'bg-blue-600 border-blue-600' : ''}"
                ></div>
              </div>
            </div>
          </div>
        {/each}
      </div>
    </section>
  </div>
</div>
