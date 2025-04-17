<script lang="ts">
  import { goto } from '$app/navigation';
  let activeLink = '/profile/account';

  function handleLinkClick(link: string) {
    activeLink = link;
    goto(link);
  }
</script>

<div class="w-full min-h-screen">
  <div class="max-w-4xl mx-auto px-4 py-8">
    <div class="flex flex-col w-full">
      <!-- Header Section -->
      <div class="flex justify-between items-center">
        <h1 class="text-3xl text-gray-900">Profile</h1>
        <nav class="flex gap-6 border-b border-gray-200">
          <a
            href="/profile/account"
            class="relative pb-3 text-base text-gray-600 hover:text-gray-900 transition-colors {activeLink ===
            '/profile/account'
              ? 'text-gray-900 after:absolute after:bottom-0 after:left-0 after:w-full after:h-0.5 after:bg-gray-900'
              : ''}"
            on:click={() => handleLinkClick('/profile/account')}>Account</a
          >
          <a
            href="/profile/settings"
            class="relative pb-3 text-base text-gray-600 hover:text-gray-900 transition-colors {activeLink ===
            '/profile/settings'
              ? 'text-gray-900 after:absolute after:bottom-0 after:left-0 after:w-full after:h-0.5 after:bg-gray-900'
              : ''}"
            on:click={() => handleLinkClick('/profile/settings')}>Settings</a
          >
          <a
            href="/profile/integration"
            class="relative pb-3 text-base text-gray-600 hover:text-gray-900 transition-colors {activeLink ===
            '/profile/integration'
              ? 'text-gray-900 after:absolute after:bottom-0 after:left-0 after:w-full after:h-0.5 after:bg-gray-900'
              : ''}"
            on:click={() => handleLinkClick('/profile/integration')}>Integrations</a
          >
        </nav>
      </div>
      <hr />
      <!-- Content Section -->
      <div class="w-full rounded-lg">
        <main class="p-6">
          <slot />
        </main>
      </div>
    </div>
  </div>
</div>
