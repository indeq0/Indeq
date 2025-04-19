<script lang="ts">
    import { createEventDispatcher } from 'svelte';
    import { SendIcon } from "svelte-feather-icons";
    import { desktopIntegration } from '$lib/stores/desktopIntegration';
    import { isIntegrated } from "$lib/utils/integration";
    import IntegrationStatus from './integration-status.svelte';
    
    export let isLoading = false;
    export let integrations = [];
    
    let userQuery = '';
    const dispatch = createEventDispatcher();
    
    function handleSubmit() {
      if (!userQuery.trim()) return;
      
      dispatch('send', { query: userQuery });
      userQuery = '';
      
      // Reset textarea height
      setTimeout(() => {
        const textarea = document.querySelector('textarea');
        if (textarea) {
          textarea.style.height = 'auto';
          textarea.rows = 1;
        }
      }, 0);
    }
</script>
  
<div class="sticky bottom-0 left-0 right-0 flex justify-center z-10 opacity-95 focus-within:opacity-100 chat-input-container">
    <div class="w-full max-w-3xl p-4 pt-0">
      <div class="relative bg-white rounded-xl shadow-lg border border-gray-200 overflow-hidden w-full">
        <textarea
          bind:value={userQuery}
          placeholder="Ask me anything..."
          class="w-full px-4 py-3 pb-14 focus:outline-none prose prose-lg resize-none overflow-y-auto textarea-scrollbar border-none"
          rows="1"
          on:input={(e) => {
            const target = e.target as HTMLTextAreaElement;
            target.style.height = 'auto';
            const newHeight = target.scrollHeight;
            const maxHeight = 150;
            target.style.height = Math.min(newHeight, maxHeight) + 'px';
            target.style.overflowY = newHeight > maxHeight ? 'auto' : 'hidden';
          }}
          on:keydown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault();
              handleSubmit();
            }
          }}
        ></textarea>
        
        <div class="absolute pr-2 bottom-0 left-0 right-0 bg-white p-2 px-4 flex items-center justify-between">
          <!-- Integration Badges -->
          <IntegrationStatus 
            {integrations} 
            desktopIntegration={$desktopIntegration} 
          />
          
          <!-- Send Button -->
          <button
            class="p-1.5 rounded-lg bg-primary text-white hover:bg-blue-600 transition-colors flex items-center justify-center"
            style="width: 32px; height: 32px;"
            on:click={handleSubmit}
            disabled={isLoading}
          >
            {#if isLoading}
              <div class="pulse-loader">
                <div class="bar"></div>
                <div class="bar"></div>
                <div class="bar"></div>
              </div>
            {:else}
              <SendIcon size="18" />
            {/if}
          </button>
        </div>
      </div>
    </div>
</div>
  
<style>
    /* Textarea scrollbar styling */
    .textarea-scrollbar {
      scrollbar-width: thin;
      -ms-overflow-style: none;
      scroll-behavior: smooth;
    }
  
    .textarea-scrollbar::-webkit-scrollbar {
      width: 6px;
    }
  
    .textarea-scrollbar::-webkit-scrollbar-track {
      background: #f1f1f1;
      border-radius: 3px;
      margin: 0;
    }
  
    .textarea-scrollbar::-webkit-scrollbar-thumb {
      background: #888;
      border-radius: 3px;
    }
  
    .textarea-scrollbar::-webkit-scrollbar-thumb:hover {
      background: #666;
    }
  
    /* Pulse loader styles */
    .pulse-loader {
      display: flex;
      align-items: center;
      gap: 2px;
      height: 20px;
      width: 20px;
      justify-content: center;
      position: relative;
    }
  
    .pulse-loader .bar {
      width: 3px;
      background-color: white;
      border-radius: 1px;
      animation: pulse 0.6s ease-in-out infinite;
    }
  
    .pulse-loader .bar:nth-child(1) {
      height: 5px;
      animation-delay: 0s;
    }
  
    .pulse-loader .bar:nth-child(2) {
      height: 8px;
      animation-delay: 0.15s;
    }
  
    .pulse-loader .bar:nth-child(3) {
      height: 6px;
      animation-delay: 0.3s;
    }
  
    @keyframes pulse {
      0%, 100% {
        transform: scaleY(1);
      }
      50% {
        transform: scaleY(1.35);
      }
    }
</style>