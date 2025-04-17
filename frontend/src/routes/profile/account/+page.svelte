<script lang="ts">
  import { writable } from 'svelte/store';
  // Store for settings
  const settings = writable({
    theme: 'system',
    model: 'claude-3',
    markdownMode: false,
    enterToSubmit: true
  });

  // Available models
  const models = [
    {
      id: 'claude-3',
      name: 'Claude-3',
      description: 'Most capable model, best for complex tasks'
    },
    { id: 'gpt-4', name: 'GPT-4', description: 'Balanced between speed and capability' },
    { id: 'mixtral', name: 'Mixtral', description: 'Fast and efficient for simple tasks' }
  ];

  const themes = ['light', 'dark', 'system'];

  function handleThemeChange(theme: string) {
    settings.update((s) => ({ ...s, theme }));
  }

  function handleModelChange(modelId: string) {
    settings.update((s) => ({ ...s, model: modelId }));
  }
</script>

<div class="min-h-screen">
  <div class="max-w-4xl mx-auto px-4 py-8">
    <!-- Profile Section -->
    <section class="mb-12">
      <h2 class="text-xl font-medium mb-4 text-gray-900 dark:text-white">Appearance</h2>
      <div class="dark:bg-gray-800 rounded-lg p-4 shadow-sm">
        <!-- Profile Card -->
        <div class="flex items-center justify-between">
          <div class="space-y-1">
            <h3 class="font-medium text-gray-900 dark:text-white">Avatar</h3>
          </div>

          <div class="flex items-center gap-6">
            <img
              src="/google.svg"
              alt="Avatar"
              class="w-12 h-12 rounded-full ring-2 ring-gray-500 dark:ring-gray-700"
            />
            <button
              class="flex items-center gap-2 px-4 py-2 text-sm font-medium text-gray-700 dark:text-gray-200 bg-gray-100 dark:bg-gray-800 rounded-lg hover:bg-gray-200 dark:hover:bg-gray-700 transition-colors"
            >
              Upload
            </button>
          </div>
        </div>
      </div>
    </section>

    <!-- Theme Section -->
    <section class="mb-12">
      <h2 class="text-xl font-medium mb-4 text-gray-900 dark:text-white">Appearance</h2>
      <div class="bg-gray-100 dark:bg-gray-800 rounded-lg p-4 shadow-sm">
        <div class="flex items-center justify-between">
          <div>
            <h3 class="font-medium text-gray-900 dark:text-white">Theme</h3>
            <p class="text-sm text-gray-600 dark:text-gray-400">Select your preferred theme</p>
          </div>
          <div class="flex gap-2">
            {#each themes as theme}
              <button
                class="px-4 py-2 rounded-md text-sm font-medium
                       {$settings.theme === theme
                  ? 'bg-blue-500 text-white'
                  : 'bg-gray-300 dark:bg-gray-700 text-gray-700 dark:text-gray-300'}"
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
                   {$settings.model === model.id ? 'ring-2 ring-blue-500' : ''}"
            on:click={() => handleModelChange(model.id)}
          >
            <div class="flex items-center justify-between">
              <div>
                <h3 class="font-medium text-gray-900 dark:text-white">
                  {model.name}
                </h3>
                <p class="text-sm text-gray-600 dark:text-gray-400">
                  {model.description}
                </p>
              </div>
              <div class="flex items-center">
                <div
                  class="w-4 h-4 rounded-full border-2 border-gray-300 dark:border-gray-600
                          {$settings.model === model.id ? 'bg-blue-500 border-blue-500' : ''}"
                ></div>
              </div>
            </div>
          </div>
        {/each}
      </div>
    </section>

    <!-- Additional Settings -->
    <section class="mb-12">
      <h2 class="text-xl font-medium mb-4 text-gray-900 dark:text-white">Chat Preferences</h2>
      <div class="space-y-4">
        <div class="bg-gray-100 dark:bg-gray-800 rounded-lg p-4 shadow-sm">
          <div class="flex items-center justify-between">
            <div>
              <h3 class="font-medium text-gray-900 dark:text-white">Markdown Mode</h3>
              <p class="text-sm text-gray-600 dark:text-gray-400">
                Enable markdown formatting in responses
              </p>
            </div>
            <button
              class="w-12 h-6 rounded-full transition-colors duration-200 ease-in-out
                     {$settings.markdownMode ? 'bg-blue-500' : 'bg-gray-300 dark:bg-gray-700'}"
              on:click={() => settings.update((s) => ({ ...s, markdownMode: !s.markdownMode }))}
            >
              <div
                class="w-5 h-5 rounded-full transform transition-transform duration-200 ease-in-out
                       {$settings.markdownMode ? 'translate-x-6' : 'translate-x-1'}"
              ></div>
            </button>
          </div>
        </div>

        <div class="bg-gray-100 dark:bg-gray-800 rounded-lg p-4 shadow-sm">
          <div class="flex items-center justify-between">
            <div>
              <h3 class="font-medium text-gray-900 dark:text-white">Enter to Submit</h3>
              <p class="text-sm text-gray-600 dark:text-gray-400">Use Enter key to send messages</p>
            </div>
            <button
              class="w-12 h-6 rounded-full transition-colors duration-200 ease-in-out
                     {$settings.enterToSubmit ? 'bg-blue-500' : 'bg-gray-300 dark:bg-gray-700'}"
              on:click={() => settings.update((s) => ({ ...s, enterToSubmit: !s.enterToSubmit }))}
            >
              <div
                class="w-5 h-5 rounded-full transform transition-transform duration-200 ease-in-out
                       {$settings.enterToSubmit ? 'translate-x-6' : 'translate-x-1'}"
              ></div>
            </button>
          </div>
        </div>
      </div>
    </section>
  </div>
</div>
