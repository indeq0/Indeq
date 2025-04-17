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

<div class="min-h-screen">
  <div class="max-w-4xl mx-auto px-4 py-8">
    <h2 class="text-xl font-medium mb-4 text-gray-900 dark:text-white">Integrations</h2>
    <main>
      {#each integrations as integration}
        <div class="card">
          <div class="content">
            <div class="logo-container">
              <img src={integration.logo} alt="{integration.name} Logo" class="logo" />
            </div>
            <div>
              <span class="text-lg font-medium text-gray-900">{integration.name}</span>
              <p class="text-gray-600 text-sm mt-1 leading-relaxed">
                {integration.description}
              </p>
            </div>
          </div>
          <Integration_Button
            company={integration.company}
            isIntegrated={isProviderIntegrated(integration.company)}
          />
        </div>
      {/each}
    </main>
  </div>
</div>

<style>
  main {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: 1rem;
    width: 100%;
  }

  .card {
    display: flex;
    flex-direction: row;
    justify-content: space-between;
    align-items: center;
    background: rgb(243, 244, 246);
    backdrop-filter: blur(10px);
    padding: 1rem;
    border-radius: 8px;
    width: 100%;
  }

  .content {
    display: flex;
    align-items: center;
    gap: 1rem;
  }

  .logo-container {
    padding: 0.5rem;
    border-radius: 50%;
  }

  .logo {
    height: 50px;
    width: 50px;
  }

  @media (max-width: 768px) {
    main {
      flex-direction: column;
      align-items: center;
    }

    .card {
      flex-direction: column;
      text-align: center;
      height: auto;
    }

    .content {
      flex-direction: column;
      align-items: center;
    }
  }
</style>
