<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import { page } from '$app/stores';
  export let item: {
    id: string;
    title: string;
  };
  export let expanded = true;
  $: url = $page.url.pathname;
 </script>
 
 {#if expanded}
 <Button
   variant="ghost"
   href={`/chat/${item.id}`}
   class="w-full inline-flex justify-start p-0 text-sm hover:bg-[#e6e4e3] {url === `/chat/${item.id}` ? 'bg-[#e6e4e3] text-gray-700' : 'text-gray-500'} px-2 py-1.5 h-auto"
   aria-label={item.id}
 >
  <span class="truncate w-full">{item.title}</span>
 </Button>
 {/if}