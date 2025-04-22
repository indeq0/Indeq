<script lang="ts">
  import Integration_Button from '$lib/components/integration/integration-card.svelte';
  import { page } from '$app/stores';
  import { onMount } from 'svelte';
  import { toast } from 'svelte-sonner';

  const capitalize = (s: string) => {
    return s.charAt(0).toUpperCase() + s.slice(1).toLowerCase();
  };

  onMount(() => {
    const params = new URLSearchParams(window.location.search);
    const status = params.get('status');
    const action = params.get('action');
    const provider = params.get('provider');

    if (!status || !action || !provider) {
      return;
    }

    if (status === 'success') {
      if (action === 'connect') {
        toast.success(`Connected successfully with ${capitalize(provider)}!`);
      } else if (action === 'disconnect') {
        toast.success(`Disconnected successfully from ${capitalize(provider)}!`);
      }
    } else if (status === 'error') {
      if (action === 'connect') {
        toast.error(`Failed to connect with ${capitalize(provider)}!`);
      } else if (action === 'disconnect') {
        toast.error(`Failed to disconnect from ${capitalize(provider)}!`);
      }
    }

    const url = new URL(window.location.href);
    url.searchParams.delete('status');
    url.searchParams.delete('action');
    url.searchParams.delete('provider');
    window.history.replaceState({}, '', url.toString());
  });

  // List of companies and their data
  const integrations = [
    {
      name: 'Google',
      logo: '/google.svg',
      description: 'Connect all of your docs, sheets, slides, drawings, and other files.',
      company: 'GOOGLE'
    },
    {
      name: 'Microsoft',
      logo: '/microsoft.svg',
      description: 'Connect all of your Office 365 apps, including Word, Excel, and PowerPoint.',
      company: 'MICROSOFT'
    },
    {
      name: 'Notion',
      logo: '/notion.svg',
      description: 'Integrate your notes, tasks, and projects in one place.',
      company: 'NOTION'
    }
  ];

  function isProviderIntegrated(provider: string) {
    const { connectedProviders } = $page.data;
    if (!connectedProviders || !Array.isArray(connectedProviders)) {
      return false;
    }
    return connectedProviders.includes(provider.toUpperCase());
  }
</script>

<div class="h-full overflow-hidden mt-7 px-5">
  <div class="w-full mx-auto py-8 overflow-auto h-full">
    <h2 class="text-xl font-medium mb-4 text-gray-900 dark:text-white">Integrations</h2>
    <section class="mb-12 mt-4">
      {#each integrations as integration}
        <div class="bg-white dark:bg-gray-800 rounded-lg shadow-sm p-6 mb-6">
          <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
            <div class="flex items-center gap-3">
              <div class="w-12 h-12 flex items-center justify-center">
                <img src={integration.logo} alt="{integration.name} Logo" class="w-10 h-10" />
              </div>
              <div class="space-y-1">
                <h4 class="text-lg font-medium text-gray-900 dark:text-gray-100">{integration.name}</h4>
                <p class="text-sm text-gray-600 dark:text-gray-400">{integration.description}</p>
              </div>
            </div>
            <div>
              <Integration_Button
                company={integration.company}
                isIntegrated={isProviderIntegrated(integration.company)}
              />
            </div>
          </div>
        </div>
      {/each}
    </section>
  </div>
</div>
