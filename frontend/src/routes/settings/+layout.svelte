<script lang='ts'>
  import { page } from '$app/stores'; 
  import { goto } from '$app/navigation';
  import { browser } from '$app/environment';
  import { onMount } from 'svelte';

  let activeLink: string;
  $: activeLink = $page.url.pathname;

  onMount(() => {
    if (browser && window.location.pathname === '/settings') {
      goto('/settings/account');
    }
  });

  function handleLinkClick(link: string) {
    goto(link);
  }
</script>

<div class="w-full min-h-screen bg-white">
  <!-- Header Section -->
  <div class="sticky top-0 left-0 border-b border-gray-200 bg-white z-10">
    <div class="max-w-3xl mx-auto px-4 pt-4 pb-1 h-[64px]">
      <div class="flex justify-between items-center h-full">
        <h1 class="text-3xl text-gray-900">Settings</h1>
        <nav class="flex gap-6 h-full">
          <a
            href="/settings/account"
            class="relative flex items-center text-base text-gray-600 hover:text-gray-900 transition-colors"
            class:active={activeLink === '/settings/account'}
            on:click={() => handleLinkClick('/settings/account')}
          >
            Account
          </a>
          <a
            href="/settings/profile"
            class="relative flex items-center text-base text-gray-600 hover:text-gray-900 transition-colors"
            class:active={activeLink === '/settings/profile'}
            on:click={() => handleLinkClick('/settings/profile')}
          >
            Profile
          </a>
          <a
            href="/settings/integration"
            class="relative flex items-center text-base text-gray-600 hover:text-gray-900 transition-colors"
            class:active={activeLink === '/settings/integration'}
            on:click={() => handleLinkClick('/settings/integration')}
          >
            Integrations
          </a>
        </nav>
      </div>
    </div>
  </div>
  <!-- Content -->
  <div class="max-w-3xl mx-auto px-4 py-6" >
    <div class="flex flex-col w-full">
      <div class="w-full bg-white rounded-lg">
        <main>
          <slot />
        </main>
      </div>
    </div>
  </div>
</div>

<style>
  a.active {
    color: #1a202c; 
  }
  a.active::after {
    content: '';
    position: absolute;
    bottom: 0;
    left: 0;
    width: 100%;
    height: 0.5px;
    background-color: #1a202c;
  }
</style>
