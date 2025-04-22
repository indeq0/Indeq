<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import { page } from '$app/stores';
  import { TrashIcon } from 'svelte-feather-icons';
  import { conversationStore } from '../../stores/conversationStore';
  import { goto } from '$app/navigation';
  import { slide } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  
  export let item: {
    id: string;
    title: string;
  };
  export let expanded = true;
  $: url = $page.url.pathname;
  
  function deleteConversation(e: MouseEvent) {
    e.preventDefault();
    e.stopPropagation();
    conversationStore.deleteConversation(item.id);
    if (url === `/chat/${item.id}`) {
      goto('/chat');
    }
  }
 </script>
 
 {#if expanded}
 <div 
   class="group relative"
   in:slide={{ duration: 300, easing: cubicOut }} 
   out:slide={{ duration: 250, easing: cubicOut }}
 >
   <Button
     variant="ghost"
     href={`/chat/${item.id}`}
     class="w-full inline-flex justify-start p-0 text-sm hover:bg-[#e6e4e3] {url === `/chat/${item.id}` ? 'bg-[#e6e4e3] text-gray-700' : 'text-gray-500'} px-2 py-1.5 h-auto"
     aria-label={item.id}
   >
    <span class="truncate w-full">{item.title}</span>
   </Button>
   <button 
     class="absolute right-1 top-1/2 -translate-y-1/2 opacity-0 group-hover:opacity-100 p-1 hover:bg-gray-100 rounded transition-opacity"
     on:click={deleteConversation}
     aria-label="Delete conversation"
   >
     <TrashIcon size="14" class="text-gray-500 transition-colors" />
   </button>
 </div>
 {/if}