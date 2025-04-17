<script lang="ts">
  import { SendIcon } from "svelte-feather-icons";
  import { onDestroy, onMount } from 'svelte';
  import "katex/dist/katex.min.css";
  import { initialize, startPolling, stopPolling, desktopIntegration } from '$lib/stores/desktopIntegration';
  import type { DesktopIntegration } from "$lib/types/desktopIntegration";
  import { isIntegrated } from "$lib/utils/integration";
  import { goto } from "$app/navigation";
  import { modelStore } from "$lib/stores/modelStore";
  
  let userQuery = '';
  let isLoading = false;
  let conversationId = '';
  let requestId = '';
  let chatInput: HTMLTextAreaElement;
  let isNavigating = false;

  export let data: { 
    integrations: string[],
    desktopInfo: DesktopIntegration,
  };

  onMount(() => {
    initialize(data.desktopInfo);
    if (data.desktopInfo.isCrawling) {
      startPolling();
    }
    // Focus the chat input when the component mounts
    chatInput?.focus();
  });

  const handleQuery = async () => {
    if (userQuery.trim() === '') {
      return;
    }

    const res = await fetch('/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ query: userQuery, conversation_id: '', model: $modelStore })
    });

    if (!res.ok) {
      const msg = await res.text();
      console.error('Error from /chat POST:', msg);
      isLoading = false;
      return;
    }

    const chatData = await res.json();
    conversationId = chatData.conversation_id;

    if (conversationId) {
      document.cookie = "chatSource=query; path=/; max-age=5"; // 5 second expiry
      goto(`/chat/${conversationId}`);
    } else {
      console.error('No conversation ID returned from server');
      isLoading = false;
    }
  }
  
  onDestroy(() => {
    stopPolling();
  });

</script>

<svelte:head>
  <title>Indeq</title>
  <meta name="description" content="Chat with Indeq" />
</svelte:head>

<main class="min-h-[calc(100vh-60px)] flex flex-col items-center px-6">
  <div class="flex-1 flex flex-col w-full max-w-3xl items-center mt-[calc(33vh)]">
    <div class="w-full p-4 mb-3 text-center welcome-text" style="view-transition-name: welcome-text;">
      <div class="flex items-center justify-center gap-3">
        <p class="text-3xl text-gray-700 font-light">How will you be productive today, Patrick?</p>
      </div>
    </div>
    
    <!-- Chat Input -->
    <div class="w-full flex justify-center z-10 opacity-95 chat-input-container">
      <div class="w-full max-w-3xl p-4 pt-0 pb-0">
        <div class="relative bg-white rounded-xl shadow-lg border border-gray-200 overflow-hidden">
          <textarea
            bind:this={chatInput}
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
                handleQuery();
              }
            }}
          ></textarea>
          
          <div class="absolute pr-2 bottom-0 left-0 right-0 bg-white p-2 px-4 flex items-center justify-between">
            <!-- Integration Badges -->
            <div class="flex gap-2">
              <!-- Desktop Integration -->
              <div class="flex items-center gap-1 bg-gray-50 px-2 py-1 rounded-full">
                <div class="relative">
                  <div
                    class="w-2 h-2 rounded-full"
                    style="background-color: {$desktopIntegration && $desktopIntegration.isCrawling && $desktopIntegration.crawledFiles != $desktopIntegration.totalFiles ? 'orange' : $desktopIntegration && $desktopIntegration.isOnline ? 'green' : 'red'}" 
                  ></div>
                  <div
                    class="w-2 h-2 rounded-full absolute top-0 animate-ping"
                    style="background-color: {$desktopIntegration && $desktopIntegration.isCrawling && $desktopIntegration.crawledFiles != $desktopIntegration.totalFiles ? 'orange' : $desktopIntegration && $desktopIntegration.isOnline ? 'green' : 'red'}"
                  ></div>
                </div>
                <span class="text-xs text-gray-600 ml-1">Desktop {$desktopIntegration && $desktopIntegration.isCrawling && $desktopIntegration.crawledFiles != $desktopIntegration.totalFiles ? $desktopIntegration.crawledFiles + ' / ' + $desktopIntegration.totalFiles + ' files' : ""}</span>
              </div>
              
              <!-- Google -->
              <div class="flex items-center gap-1 bg-gray-50 px-2 py-1 rounded-full">
                <div class="relative">
                  <div
                    class="w-2 h-2 rounded-full"
                    style="background-color: {isIntegrated(data.integrations, 'GOOGLE') ? 'green' : 'red'}"
                  ></div>
                  <div
                    class="w-2 h-2 rounded-full absolute top-0 animate-ping"
                    style="background-color: {isIntegrated(data.integrations, 'GOOGLE') ? 'green' : 'red'}"
                  ></div>
                </div>
                <span class="text-xs text-gray-600 ml-1">Google</span>
              </div>
              <!-- Microsoft -->
              <div class="flex items-center gap-1 bg-gray-50 px-2 py-1 rounded-full">
                <div class="relative">
                  <div
                    class="w-2 h-2 rounded-full"
                    style="background-color: {isIntegrated(data.integrations, 'MICROSOFT') ? 'green' : 'red'}"
                  ></div>
                  <div
                    class="w-2 h-2 rounded-full absolute top-0 animate-ping"
                    style="background-color: {isIntegrated(data.integrations, 'MICROSOFT') ? 'green' : 'red'}"
                  ></div>
                </div>
                <span class="text-xs text-gray-600 ml-1">Microsoft</span>
              </div>
              <!-- Notion -->
              <div class="flex items-center gap-1 bg-gray-50 px-2 py-1 rounded-full">
                <div class="relative">
                  <div
                    class="w-2 h-2 rounded-full"
                    style="background-color: {isIntegrated(data.integrations, 'NOTION') ? 'green' : 'red'}"
                  ></div>
                  <div
                    class="w-2 h-2 rounded-full absolute top-0 animate-ping"
                    style="background-color: {isIntegrated(data.integrations, 'NOTION') ? 'green' : 'red'}"
                  ></div>
                </div>
                <span class="text-xs text-gray-600 ml-1">Notion</span>
              </div>
            </div>
            
            <!-- Send Button -->
            <button
              class="p-1.5 rounded-lg bg-primary text-white hover:bg-blue-600 transition-colors flex items-center justify-center"
              style="width: 32px; height: 32px;"
              on:click={handleQuery}
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
  </div>
</main>

<style>
  :global(html, body) {
    overflow-x: hidden;
    position: relative;
    width: 100%;
  }

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

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  /* New pulse loader styles */
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

  /* Welcome text animation */
  .welcome-text {
    animation: fly-in-from-top 0.5s cubic-bezier(0.4, 0, 0.2, 1) forwards;
  }

  @keyframes fly-in-from-top {
    from {
      opacity: 0;
      transform: translateY(-50px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
