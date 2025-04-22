<script lang="ts">
  import type { ChatMessage, ChatSource, ChatState } from "$lib/types/chat";
  import ChatInput from '$lib/components/chat/chat-input.svelte';
  import MessageList from '$lib/components/chat/message-list.svelte';
  import "katex/dist/katex.min.css";
  
  import { onMount } from "svelte";
  import { conversationStore } from "$lib/stores/conversationStore";
  import { modelStore } from "$lib/stores/modelStore";
  import { processOutputMessage, processReasoningMessage, processSource } from "$lib/utils/chat";

  export let data: { 
    id: string, 
    title: string, 
    conversation: ChatMessage[], 
    integrations: string[], 
    newConversation: boolean
  };
  
  let messages: ChatMessage[] = [];
  let isReasoning = false;
  let isStreaming = false;
  let isLoading = false;
  let eventSource: EventSource | null = null;
  let currentConversationId: string | null = null;
  let chatInputComponent: ChatInput;

  $: {
    if (data.id) {
      if (currentConversationId !== data.id) {
        currentConversationId = data.id;
        
        eventSource?.close();
        messages = [];
                
        if (data.newConversation) { // new conversation
          messages = [{ text: data.title, sender: "user", reasoning: [], reasoningSectionCollapsed: false, sources: [] }];
          messages = [...messages, { text: "", sender: "bot", reasoning: [], reasoningSectionCollapsed: false, sources: [], model: $modelStore }];
          streamResponse();
        } else {
          messages = data.conversation;          
        }
      }
    }
  }

  function updateMessages(newMessages: ChatMessage[]) {
    messages = newMessages;
  }

  function retryMessage(query: string) {
    if (chatInputComponent) {
      chatInputComponent.setInputValue(query);
    }
  }

  onMount(() => {
    if (data.id) {
      currentConversationId = data.id;
          
      if (data.newConversation) { // new conversation
        messages = [{ text: data.title, sender: "user", reasoning: [], reasoningSectionCollapsed: false, sources: [] }];
        messages = [...messages, { text: "", sender: "bot", reasoning: [], reasoningSectionCollapsed: false, sources: [], model: $modelStore }];
        streamResponse();
        conversationStore.fetchConversations(true);
      } else {
        messages = data.conversation;
      }
    }

    return () => {
      eventSource?.close();
      eventSource = null;
      isLoading = false;
    };
  });
  
  async function handleSendMessage(event: { detail: { query: string } }) {
    try {
      isLoading = true;
      const userQuery = event.detail.query;

      const res = await fetch('/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ query: userQuery, conversation_id: data.id, model: $modelStore })
      });

      if (!res.ok) {
        const msg = await res.text();
        console.error('Error from /chat POST:', msg);
        isLoading = false;
        return;
      }

      currentConversationId = data.id;
      
      // Add user message to conversation
      const userMessage = { 
        text: userQuery, 
        sender: "user", 
        reasoning: [], 
        reasoningSectionCollapsed: false, 
        sources: [] 
      };
      messages = [...messages, userMessage];
      
      // Add empty bot message that will be updated when streaming
      const botMessage = { 
        text: "", 
        sender: "bot", 
        reasoning: [], 
        reasoningSectionCollapsed: false, 
        sources: [],
        sourcesScrollAtEnd: false,
        isScrollable: false,
        model: $modelStore
      };
      messages = [...messages, botMessage];
      
      // Start streaming the response
      isStreaming = true;
      streamResponse();
    } catch (err) {
      console.error('sendMessage error:', err);
      isLoading = false;
    }
  }

  function streamResponse() {

    // Close any existing connection
    eventSource?.close();
    isReasoning = false;

    const url = `/chat?conversationId=${encodeURIComponent(data.id)}`;
    eventSource = new EventSource(url);
    let botMessage : ChatMessage;

    // Reference the last message in the messages array if it's a bot message
    // This ensures we're updating the correct message when streaming
    if (messages.length > 0 && messages[messages.length - 1].sender === 'bot') {
      botMessage = messages[messages.length - 1];
    } else {
      // This shouldn't happen normally, but just in case
      botMessage = { 
        text: "", 
        sender: "bot", 
        reasoning: [] as {text: string; collapsed: boolean}[],
        reasoningSectionCollapsed: false,
        sources: [] as ChatSource[],
        sourcesScrollAtEnd: false,
        isScrollable: false,
        model: $modelStore
      };
      messages = [...messages, botMessage];
    }

    eventSource.addEventListener('message', (evt) => {
      const payload = JSON.parse(evt.data);
      const type = payload.type;

      switch (type) {
        case "think_start":
            isReasoning = true;
            return;
        case "think_end":
            isReasoning = false;
            return;
        case "source":
            processSource(payload, botMessage);
            setTimeout(async () => {
                const scrollContainer = document.querySelector(`.scroll-container:last-child`) as HTMLElement;
                if (scrollContainer) {
                    const isScrollable = scrollContainer.scrollWidth > scrollContainer.clientWidth;
                    // Find the index of the bot message we're updating
                    const botIndex = messages.findIndex(m => m === botMessage);
                    if (botIndex !== -1) {
                        messages = messages.map((msg, idx) => 
                            idx === botIndex
                                ? {...msg, isScrollable}
                                : msg
                        );
                    }
                }
            }, 50);
            return;
        case "end":
            if (eventSource) {
                eventSource.close();
                isLoading = false;
            }
            isStreaming = false;
            // Find the loading conversation and update its status directly
            const loadingConversation = $conversationStore.headers.find(h => h.is_loading);
            if (loadingConversation) {
              // Update just the title for the conversation that was loading
              conversationStore.updateConversationTitle(loadingConversation.conversation_id);
            }
            return;
        case "token":
            // state object to pass to the processing functions
            const state : ChatState = { messages };
            
            if (isReasoning) {
                processReasoningMessage(payload.token, botMessage, state);
                messages = state.messages;
            } else {
                processOutputMessage(payload.token, botMessage, state);
                messages = state.messages;
            }
          }
        });

    eventSource.addEventListener('error', (err) => {
      console.error('SSE error:', err);
      eventSource?.close();
      eventSource = null;
      isLoading = false;
    });
  }

  // Handle cleanup when component is destroyed
  onMount(() => {
    return () => {
      eventSource?.close();
      eventSource = null;
      isLoading = false;
    };
  });
</script>

<svelte:head>
  <title>{data.title} - Indeq</title>
  <meta name="description" content="Chat with Indeq" />
</svelte:head>

<main class="min-h-[calc(100vh-60px)] flex flex-col items-center justify-center px-6">
  <div class="flex-1 flex flex-col w-full max-w-3xl h-screen">
    <MessageList {messages} {isReasoning} {updateMessages} {retryMessage} />
    <ChatInput 
      on:send={handleSendMessage} 
      {isLoading} 
      integrations={data.integrations}
      bind:this={chatInputComponent}
    />
  </div>
</main>

<style>
  :global(html, body) {
    overflow-x: hidden;
    position: relative;
    width: 100%;
  }
</style>