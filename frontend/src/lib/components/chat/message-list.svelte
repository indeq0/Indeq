<script lang="ts">
  import type { ChatMessage } from "$lib/types/chat";
  import Message from "./message.svelte";
  
  export let messages: ChatMessage[] = [];
  export let isReasoning: boolean = false;
  
  let conversationContainer: HTMLDivElement;
  
  function updateMessages(newMessages: ChatMessage[]) {
    messages = newMessages;
  }
</script>

<div 
  bind:this={conversationContainer}
  class="conversation-container flex-1 overflow-y-auto !overflow-x-hidden p-4 space-y-6 pb-32 max-w-full w-full" 
  style="height: calc(100vh - 100px);"
>
  {#each messages as message, messageIndex (messageIndex)}
    <Message 
      {message} 
      {messageIndex} 
      {messages} 
      {isReasoning}
      {updateMessages}
    />
  {/each}
</div>

<style>
  .conversation-container {
    scrollbar-width: thin;
    -ms-overflow-style: none;
    scroll-behavior: smooth;
  }

  .conversation-container::-webkit-scrollbar {
    width: 6px;
  }

  .conversation-container::-webkit-scrollbar-track {
    background: #f1f1f1;
    border-radius: 3px;
    margin: 0;
  }

  .conversation-container::-webkit-scrollbar-thumb {
    background: #888;
    border-radius: 3px;
  }

  .conversation-container::-webkit-scrollbar-thumb:hover {
    background: #666;
  }
</style> 