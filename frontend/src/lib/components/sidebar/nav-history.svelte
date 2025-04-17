<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  
  export let item: {
    id: string;
    title: string;
  };
  export let expanded = true;
 </script>
 
 {#if expanded}
 <Button
   variant="ghost"
   class="w-full h-6 inline-flex justify-start p-0 text-xs text-gray-500"
   aria-label={item.id}
   style=
   "overflow: hidden; white-space: nowrap; text-overflow: ellipsis;"
 >
  <span class="truncate">- {item.title}</span>
 </Button>
 {/if}