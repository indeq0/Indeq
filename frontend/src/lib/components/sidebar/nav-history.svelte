<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import { page } from '$app/stores';
  import { TrashIcon, LoaderIcon } from 'svelte-feather-icons';
  import { conversationStore } from '../../stores/conversationStore';
  import { goto } from '$app/navigation';
  import { slide, fade } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import { onMount } from 'svelte';
  
  // Extended type to allow for tracking previous loading state
  type ExtendedConversationItem = {
    id: string;
    title: string;
    is_loading: boolean;
    prev_is_loading?: boolean;
  }
  
  export let item: ExtendedConversationItem;
  export let expanded = true;
  $: url = $page.url.pathname;
  
  // States
  let titleToShow = ''; // Only one title variable to prevent any timing issues
  let animationInterval: ReturnType<typeof setInterval> | null = null;
  let showLoader = false;
  let initialLoad = true;
  
  // Initialize on mount
  onMount(() => {
    // Set initial values
    titleToShow = item.title;
    showLoader = item.is_loading;
    item.prev_is_loading = item.is_loading;
    
    // Mark initial load complete after a short delay
    setTimeout(() => {
      initialLoad = false;
    }, 100);
    
    // Cleanup on unmount
    return () => {
      if (animationInterval) clearInterval(animationInterval);
    };
  });
  
  // Simple animation function
  function animateTitleChange(newTitle: string) {
    // Clear any existing animation interval
    if (animationInterval) {
      clearInterval(animationInterval);
      animationInterval = null;
    }
    
    // Start with empty title and set up for animation
    titleToShow = '';
    let currentIndex = 0;
    let loaderHidden = false;
    
    // Very short delay before starting animation
    setTimeout(() => {
      // Animate character by character
      animationInterval = setInterval(() => {
        if (currentIndex <= newTitle.length) {
          titleToShow = newTitle.substring(0, currentIndex);
          currentIndex++;
          
          // Hide loader as soon as we start showing characters
          // but only if we're not in loading state
          if (!item.is_loading && !loaderHidden && currentIndex > 1) {
            showLoader = false;
            loaderHidden = true;
          }
        } else {
          // Animation complete
          if (animationInterval) {
            clearInterval(animationInterval);
            animationInterval = null;
          }
          
          // Ensure loader is hidden at the end of animation if not loading
          if (!item.is_loading) {
            showLoader = false;
          }
        }
      }, 10);
    }, 50); // Small delay to ensure clean transition
  }
  
  // Handle loading state changes
  $: if (!initialLoad && item.prev_is_loading !== item.is_loading) {
    // Update tracked state
    const oldState = item.prev_is_loading;
    item.prev_is_loading = item.is_loading;
    
    if (!oldState && item.is_loading) {
      showLoader = true;
    }
    
    if (oldState && !item.is_loading) {
      animateTitleChange(item.title);
    }
  }
  
  // Handle title changes when NOT loading
  $: if (!initialLoad && !item.is_loading && titleToShow !== item.title && !animationInterval) {
    animateTitleChange(item.title);
  }
  
  // Always sync title during loading
  $: if (item.is_loading && titleToShow !== item.title) {
    titleToShow = item.title;
  }
  
  // Function to delete a conversation
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
     class="w-full inline-flex justify-start p-0 text-sm hover:bg-[#e6e4e3] {url === `/chat/${item.id}` ? 'bg-[#e6e4e3] text-gray-700' : 'text-gray-500'} px-2 py-1.5 h-auto overflow-hidden"
     aria-label={item.id}
   >
    <!-- Loading spinner - now controlled by showLoader state -->
    {#if showLoader}
      <LoaderIcon size="14" class="text-gray-500 animate-spin mr-2" />
    {/if}
    
    <!-- Content area with fixed height to prevent layout shifts -->
    <div class="truncate w-full h-5 flex items-center">
      <span class="truncate">{titleToShow}</span>
    </div>
   </Button>
   <button 
     class="absolute right-1 top-1/2 -translate-y-1/2 opacity-0 bg-[#e6e4e3] group-hover:opacity-100 p-1 hover:bg-gray-100 rounded transition-opacity"
     on:click={deleteConversation}
     aria-label="Delete conversation"
   >
     <TrashIcon size="14" class="text-gray-500 transition-colors" />
   </button>
 </div>
 {/if}