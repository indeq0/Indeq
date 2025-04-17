<script lang="ts">
	import '../app.css';
	import { Toaster } from "$lib/components/ui/sonner";
    import AppSidebar from '$lib/components/sidebar/app-sidebar.svelte';    
    import { injectAnalytics } from '@vercel/analytics/sveltekit'
    import { page } from '$app/stores';
    import { sidebarExpanded } from '$lib/stores/sidbarStore';
    import { isValidRoute } from '$lib/config/sidebar-routes';
    import { browser } from '$app/environment';
    import { PUBLIC_APP_ENV } from '$env/static/public';
  let { children } = $props();

  const siteUrl = 'https://indeq.app';
  const ogImage = `${siteUrl}/meta-image.png`;

  if (PUBLIC_APP_ENV === 'PRODUCTION') {
    // Vercel Analytics
    injectAnalytics({
      debug: false,
      mode: 'production'
    });
  }
</script>

<svelte:head>
  <title>Indeq | Cross-Platform File Search</title>
  <meta
    name="description"
    content="Indeq: Search Google Drive, Gmail, Office, and Notion fast. Join the waitlist for launch!"
  />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <!-- Open Graph / Social Media Meta Tags -->
  <meta property="og:title" content="Indeq | Cross-Platform File Search" />
  <meta
    property="og:description"
    content="Search Google Drive, Gmail, Office, and Notion fast. Join the waitlist for launch!"
  />
  <meta property="og:type" content="website" />
  <meta property="og:url" content={siteUrl} />
  <meta property="og:image" content={ogImage} />
  <!-- Twitter Card data -->
  <meta name="twitter:card" content="summary_large_image" />
  <meta name="twitter:title" content="Indeq | Cross-Platform File Search" />
  <meta
    name="twitter:description"
    content="Search Google Drive, Gmail, Office, and Notion fast. Join the waitlist for launch!"
  />
  <meta name="twitter:image" content={ogImage} />
</svelte:head>

<Toaster theme="light" />
{#if browser && isValidRoute($page.url.pathname)}
    <AppSidebar isExpanded={$sidebarExpanded}>
        <main>{@render children()}</main>
    </AppSidebar>
{:else if browser}
    <main>{@render children()}</main>
{/if}