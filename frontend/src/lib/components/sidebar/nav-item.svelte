<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import * as Tooltip from "$lib/components/ui/tooltip";
  import { page } from "$app/stores";
  import { fade } from 'svelte/transition';
  export let item: {
    label: string;
    url: string;
    icon: any;
  };
  export let expanded = true;

  $: isActive = $page.url.pathname === item.url;
</script>

{#if expanded}
  <div class="transition-all duration-300 ease-in-out" in:fade={{ delay: 150 }}>
    <Button
      href={item.url}
      variant={"ghost"}
      class={`w-full justify-start gap-2 rounded-lg hover:bg-[#e6e4e3] ${isActive ? "bg-[#e6e4e3] text-gray-700" : "text-gray-500"}`}
      aria-label={item.label}
    >
      <svelte:component this={item.icon} class="size-5 ml-0.5"/>
      <span class="font-sm ml-1">{item.label}</span>
    </Button>

  </div>
  
{:else}
  <Tooltip.Root>
    <Tooltip.Trigger asChild let:builder>
      <Button
        href={item.url}
        variant={"ghost"}
        size="default"
        class="rounded-full hover:bg-[#e6e4e3]"
        aria-label={item.label}
        builders={[builder]}
      >
      </Button>
    </Tooltip.Trigger>
    <Tooltip.Content side="right" sideOffset={5}>{item.label}</Tooltip.Content>
  </Tooltip.Root>
{/if} 