<script>
  import { Button } from '$lib/components/ui/button';
  export let company = 'Fill';
  export let isIntegrated = false;

  function handleAction() {
    if (isIntegrated) {
      window.location.href = `/profile/integration/disconnect?provider=${company.toUpperCase()}`;
    } else {
      window.location.href = `/auth/${company.toUpperCase()}`;
    }
  }
</script>

<Button variant="outline" on:click={handleAction}>
  <span class="company-name">
    {#if isIntegrated}
      Disconnect
    {:else}
      Connect
    {/if}
  </span>
</Button>
