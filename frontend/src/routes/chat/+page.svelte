<script lang="ts">
    import { ChevronDownIcon, CheckIcon, FileIcon, FileTextIcon, HardDriveIcon, SendIcon } from "svelte-feather-icons";
    import { onDestroy, onMount } from 'svelte';
    import "katex/dist/katex.min.css";
    import { initialize, startPolling, stopPolling, desktopIntegration } from '$lib/stores/desktopIntegration';
    import { processReasoningMessage, processOutputMessage, toggleReasoning, processSource } from '$lib/utils/chat';
    import { renderLatex, renderContent } from '$lib/utils/katex';
	  import type { BotMessage, ChatState, Source } from "$lib/types/chat";
    import type { DesktopIntegration } from "$lib/types/desktopIntegration";

  let userQuery = '';
  let requestId: string | null = null;
  let conversationId: string | null = null;
  let isLoading = false;
  const truncateLength = 80;

  let eventSource: EventSource | null = null;
  let messages: BotMessage[] = [];
  let isFullscreen = false;
  let isReasoning = false;
  let conversationContainer: HTMLElement | null = null;

  export let data: { integrations: string[], desktopInfo: DesktopIntegration };

  const isIntegrated = (provider: string): boolean => {
    return data.integrations.includes(provider.toUpperCase());
  };

  onMount(() => {
    initialize(data.desktopInfo);
    if (data.desktopInfo.isCrawling) {
      startPolling();
    }
  });
  
  onDestroy(() => {
    stopPolling();
  });

  async function query() {
    try {
      isLoading = true;

      const res = await fetch('/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ query: userQuery, conversation_id: conversationId ?? '' })
      });

      if (!res.ok) {
        const msg = await res.text();
        console.error('Error from /chat POST:', msg);
        isLoading = false;
        return;
      }


        messages = [...messages, { text: userQuery, sender: "user", reasoning: [], reasoningSectionCollapsed: false, sources: [] }];

      const data = await res.json();
      requestId = data.request_id;
      conversationId = data.conversation_id;

        messages = [...messages, { text: "", sender: "bot", reasoning: [], reasoningSectionCollapsed: false, sources: [] }];
        streamResponse();
        } catch (err) {
        console.error('sendMessage error:', err);
        isLoading = false;
        }

    userQuery = '';
    
    // Reset textarea height for both textareas
    setTimeout(() => {
      const textareas = document.querySelectorAll('textarea');
      textareas.forEach(textarea => {
        textarea.style.height = 'auto';
        textarea.rows = 1;
      });
    }, 0);
  }

  function streamResponse() {
    if (!requestId) {
      console.error('No requestId to stream');
      isLoading = false;
      return;
    }

    // Close any existing connection
    eventSource?.close();

        isFullscreen = true;
        isReasoning = false;

        const url = `/chat?requestId=${encodeURIComponent(requestId)}`;
        eventSource = new EventSource(url);
        let botMessage : BotMessage = { 
            text: "", 
            sender: "bot", 
            reasoning: [] as {text: string; collapsed: boolean}[],
            reasoningSectionCollapsed: false,
            sources: [] as Source[],
            sourcesScrollAtEnd: false,
            isScrollable: false 
        };

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
                setTimeout(() => {
                    const scrollContainer = document.querySelector(`.scroll-container:last-child`) as HTMLElement;
                    if (scrollContainer) {
                        const isScrollable = scrollContainer.scrollWidth > scrollContainer.clientWidth;
                        messages = messages.map((msg, idx) => 
                            idx === messages.length - 1
                                ? {...msg, isScrollable}
                                : msg
                        );
                    }
                }, 0);
                return;
            case "end":
                if (eventSource) {
                    eventSource.close();
                    isLoading = false;
                }
                return;
            case "token":
                // state object to pass to the processing functions
                const state : ChatState = {
                    messages,
                    isReasoning
                };
                
                if (isReasoning) {
                    processReasoningMessage(payload.token, botMessage, state);
                    messages = state.messages;
                    isReasoning = state.isReasoning;
                } else {
                    processOutputMessage(payload.token, botMessage, state);
                    messages = state.messages;
                }
          }
        });

    eventSource.addEventListener('error', (err) => {
      console.error('SSE error:', err);
      eventSource?.close();
      isLoading = false;
    });
  }

    onDestroy(() => {
        eventSource?.close();
    });

    function handleScroll(e: Event, messageIndex: number) {
        const container = e.target as HTMLElement;
        const atEnd = container.scrollLeft + container.clientWidth >= container.scrollWidth - 10;
        const isScrollable = container.scrollWidth > container.clientWidth;
        
        // Hide tooltips - more thoroughly by targeting both aria attribute and style properties
        const tooltips = document.querySelectorAll('.tooltip');
        tooltips.forEach(tooltip => {
            tooltip.setAttribute('aria-hidden', 'true');
            (tooltip as HTMLElement).style.opacity = '0';
            (tooltip as HTMLElement).style.visibility = 'hidden';
        });
        
        messages = messages.map((msg, idx) => 
            idx === messageIndex 
                ? {...msg, sourcesScrollAtEnd: atEnd, isScrollable}
                : msg
        );
    }

    function checkScrollable(container: HTMLElement, messageIndex: number) {
        const isScrollable = container.scrollWidth > container.clientWidth;
        messages = messages.map((msg, idx) => 
            idx === messageIndex 
                ? {...msg, isScrollable}
                : msg
        );
    }

    function scrollToPosition(element: HTMLElement, messageIndex: number) {
        const message = messages[messageIndex];
        if (message.sourcesScrollAtEnd) {
            element.scrollTo({ left: 0, behavior: 'smooth' });
            // Wait for scroll animation to complete before updating state
            element.addEventListener('scrollend', () => {
                messages = messages.map((msg, idx) => 
                    idx === messageIndex 
                        ? {...msg, sourcesScrollAtEnd: false}
                        : msg
                );
            }, { once: true });
        } else {
            element.scrollTo({ left: element.scrollWidth, behavior: 'smooth' }); // Add smooth scrolling
            // Wait for scroll animation to complete before updating state
            element.addEventListener('scrollend', () => {
                messages = messages.map((msg, idx) => 
                    idx === messageIndex 
                        ? {...msg, sourcesScrollAtEnd: true}
                        : msg
                );
            }, { once: true });
        }
    }

    // Add the action to check scrollability on mount
    function initScrollCheck(node: HTMLElement, messageIndex: number) {
        checkScrollable(node, messageIndex);
        
        const resizeObserver = new ResizeObserver(() => {
            checkScrollable(node, messageIndex);
        });
        
        resizeObserver.observe(node);
        
        return {
            destroy() {
                resizeObserver.disconnect();
            }
        };
    }

    // Function to position tooltips
    function positionTooltip(event: MouseEvent | FocusEvent) {
        const container = event.currentTarget as HTMLElement;
        const tooltipId = container.getAttribute('data-tooltip-id');
        if (!tooltipId) return;
        const tooltip = document.getElementById(tooltipId);
        if (!tooltip) return;
        
        const rect = container.getBoundingClientRect();
        const viewportHeight = window.innerHeight;
        
        // Check if tooltip would go off the bottom of the viewport
        const tooltipHeight = 80; // Approximate height - you may want to calculate this dynamically
        const spaceBelow = viewportHeight - rect.bottom;
        
        if (spaceBelow < tooltipHeight + 10) {
            // If not enough space below, show tooltip above
            tooltip.style.left = `${rect.left}px`;
            tooltip.style.top = `${rect.top - tooltipHeight - 8}px`;
            tooltip.style.transformOrigin = 'bottom center';
        } else {
            // Show tooltip below as usual
            tooltip.style.left = `${rect.left}px`;
            tooltip.style.top = `${rect.bottom + 8}px`;
            tooltip.style.transformOrigin = 'top center';
        }
        
        tooltip.setAttribute('aria-hidden', 'false');
        (tooltip as HTMLElement).style.opacity = '1';
        (tooltip as HTMLElement).style.visibility = 'visible';
    }
    
    // Function to hide tooltip
    function hideTooltip(event: MouseEvent | FocusEvent) {
        const container = event.currentTarget as HTMLElement;
        const tooltipId = container.getAttribute('data-tooltip-id');
        if (!tooltipId) return;
        
        const tooltip = document.getElementById(tooltipId);
        if (!tooltip) return;
        tooltip.setAttribute('aria-hidden', 'true');
        (tooltip as HTMLElement).style.opacity = '0';
        (tooltip as HTMLElement).style.visibility = 'hidden';
    }

</script>

<svelte:head>
  <title>Indeq</title>
  <meta name="description" content="Chat with Indeq" />
</svelte:head>

<main class="min-h-screen flex flex-col items-center justify-center p-6">
  {#if !isFullscreen}
    <!-- Centered Search Box -->
    <div class="w-full max-w-3xl p-8 text-center">
      <h1 class="text-4xl text-gray-900 mb-3">Indeq</h1>
      <p class="text-gray-600 mb-6">
        Crawl your content in seconds, so you can spend more time on what matters.
      </p>

      <!-- Search Input -->
      <div class="flex flex-col gap-3 p-3 bg-white rounded-lg relative">
        <textarea
          bind:value={userQuery}
          placeholder="Ask me anything..."
          class="w-full p-2 bg-transparent focus:outline-none prose prose-lg resize-none overflow-y-auto textarea-scrollbar pr-12"
          rows="1"
          on:input={(e) => {
            const target = e.target as HTMLTextAreaElement;
            target.style.height = 'auto';
            const newHeight = target.scrollHeight;
            const maxHeight = 150;
            target.style.height = Math.min(newHeight, maxHeight) + 'px';
            target.style.overflowY = newHeight > maxHeight ? 'auto' : 'hidden';
            
            const sendButton = target.parentElement?.querySelector('.send-button-dynamic') as HTMLElement;
            if (sendButton) {
              const singleLineHeight = 36;
              // If height is close to a single line, center it
              if (newHeight <= singleLineHeight * 1.5) {
                sendButton.classList.remove('bottom-3');
                sendButton.classList.add('top-1/2', '-translate-y-1/2');
              } else {
                sendButton.classList.remove('top-1/2', '-translate-y-1/2');
                sendButton.classList.add('bottom-3');
              }
            }
          }}
          on:keydown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault();
              query();
            }
          }}
        ></textarea>
        <button
          class="p-2 rounded-lg bg-primary text-white hover:bg-blue-600 transition-colors absolute right-3 z-10 top-1/2 -translate-y-1/2 send-button-dynamic flex items-center justify-center"
          on:click={query}
          disabled={isLoading}
        >
            <SendIcon size="20" />
        </button>
      </div>

      <!-- Integration Badges -->
      <div class="flex gap-4 mt-4 justify-center">
        <!-- Desktop Integration -->
        <div class="flex items-center gap-2 bg-gray-50 px-3 py-1.5 rounded-full">
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
          <span class="text-sm text-gray-600">Desktop {$desktopIntegration && $desktopIntegration.isCrawling && $desktopIntegration.crawledFiles != $desktopIntegration.totalFiles ? $desktopIntegration.crawledFiles + ' / ' + $desktopIntegration.totalFiles + ' files' : ""}</span>
        </div>
        <!-- Google -->
        <div class="flex items-center gap-2 bg-gray-50 px-3 py-1.5 rounded-full">
          <div class="relative">
            <div
              class="w-2 h-2 rounded-full"
              style="background-color: {isIntegrated('GOOGLE') ? 'green' : 'red'}"
            ></div>
            <div
              class="w-2 h-2 rounded-full absolute top-0 animate-ping"
              style="background-color: {isIntegrated('GOOGLE') ? 'green' : 'red'}"
            ></div>
          </div>
          <span class="text-sm text-gray-600">Google</span>
        </div>
        <!-- Microsoft -->
        <div class="flex items-center gap-2 bg-gray-50 px-3 py-1.5 rounded-full">
          <div class="relative">
            <div
              class="w-2 h-2 rounded-full"
              style="background-color: {isIntegrated('MICROSOFT') ? 'green' : 'red'}"
            ></div>
            <div
              class="w-2 h-2 rounded-full absolute top-0 animate-ping"
              style="background-color: {isIntegrated('MICROSOFT') ? 'green' : 'red'}"
            ></div>
          </div>
          <span class="text-sm text-gray-600">Microsoft</span>
        </div>
        <!-- Notion -->
        <div class="flex items-center gap-2 bg-gray-50 px-3 py-1.5 rounded-full">
          <div class="relative">
            <div
              class="w-2 h-2 rounded-full"
              style="background-color: {isIntegrated('NOTION') ? 'green' : 'red'}"
            ></div>
            <div
              class="w-2 h-2 rounded-full absolute top-0 animate-ping"
              style="background-color: {isIntegrated('NOTION') ? 'green' : 'red'}"
            ></div>
          </div>
          <span class="text-sm text-gray-600">Notion</span>
        </div>
      </div>
    </div>
  {:else}
    <div class="flex-1 flex flex-col w-full max-w-3xl">
      <div
        class="conversation-container flex-1 overflow-y-auto p-4 space-y-6 pb-32"
        bind:this={conversationContainer}
      >
        {#each messages as message, messageIndex}
          <div class="space-y-4">
            <div class="prose max-w-3xl mx-auto prose-lg">
              {#if message.sender === 'user'}
                <div class="font-bold prose-xl break-words whitespace-normal overflow-hidden w-full">{message.text}</div>
              {:else}
                {#if message.sources.length > 0}
                  <div class="mb-6">
                      <div class="flex justify-between items-center mb-2">
                          <h3 class="text-sm font-semibold text-gray-600">Sources</h3>
                          {#if message.isScrollable}
                          <button 
                              class="text-xs text-gray-400 hover:text-gray-600 transition-colors cursor-pointer"
                              on:click={(e) => {
                                  const target = e.target as HTMLElement;
                                  const scrollContainer = target.closest('.mb-6')?.querySelector('.scroll-container');
                                  if (scrollContainer) {
                                      scrollToPosition(scrollContainer as HTMLElement, messageIndex);
                                  }
                              }}
                          >
                              {message.sourcesScrollAtEnd ? '← Scroll to start' : 'Scroll to end →'}
                          </button>
                          {/if}
                      </div>

                      <!-- Scroll container -->
                      <div class="relative">
                          <div 
                              class="flex overflow-x-auto overflow-y-hidden pb-4 gap-3 scrollbar-thin scroll-container"
                              on:scroll={(e) => handleScroll(e, messageIndex)}
                              use:initScrollCheck={messageIndex}
                          >
                              {#each message.sources as source, sourceIndex}
                                  <div class="flex-none w-[325px]">
                                      <div 
                                          class="bg-gray-50 rounded-md p-3 hover:bg-gray-100 transition-colors duration-200 shadow-sm border border-gray-100 relative tooltip-container"
                                          on:mouseenter={positionTooltip}
                                          on:mouseleave={hideTooltip}
                                          data-tooltip-id={`tooltip-${messageIndex}-${sourceIndex}`}
                                          role="button"
                                          tabindex="0"
                                          aria-describedby={`tooltip-${messageIndex}-${sourceIndex}`}
                                      >
                                          <div class="flex items-center gap-1 text-gray-400 mb-1">
                                              <HardDriveIcon size="14" />
                                              <span class="text-gray-300 mx-1">|</span>
                                              {#if source.extension === 'pdf'}
                                                  <FileTextIcon size="14" />
                                              {:else}
                                                  <FileIcon size="14" />
                                              {/if}
                                              <span class="text-xs uppercase tracking-wider font-medium">
                                                  {source.extension}
                                              </span>
                                          </div>
                                          <div>
                                              <div class="text-sm font-medium text-gray-900 truncate mb-1">{source.title}</div>
                                              <div class="text-xs text-gray-500 truncate font-light">{source.filePath}</div>
                                          </div>
                                          
                                          <!-- Source tooltip that appears on hover -->
                                          <div class="tooltip fixed opacity-0 pointer-events-none text-gray-800 p-3 rounded shadow-md text-sm z-20 max-w-full whitespace-normal border border-gray-100" 
                                               id={`tooltip-${messageIndex}-${sourceIndex}`}
                                               role="tooltip"
                                               aria-hidden="true">

                                              <div class="font-semibold mb-1 break-words">{source.title}</div>
                                              <div class="text-xs text-gray-600 break-words">{source.filePath}</div>
                                          </div>
                                      </div>
                                  </div>
                              {/each}
                          </div>
                      </div>
                  </div>
                  
                {/if}
                {#if message.reasoning.length > 0}
                  <div class="max-w-3xl mx-auto">
                    <div class="flex justify-between items-center">
                      <h3 class="text-sm font-semibold text-gray-600">Reasoning</h3>
                      <button
                        class="text-gray-600 cursor-pointer transition-transform duration-200 mt-3"
                        class:rotate-180={!message.reasoningSectionCollapsed}
                        on:click={() => {
                          if (message.sender !== 'user') {
                            messages = messages.map((msg, idx) =>
                              idx === messageIndex
                                ? {
                                    ...msg,
                                    reasoningSectionCollapsed: !msg.reasoningSectionCollapsed
                                  }
                                : msg
                            );
                          }
                        }}
                      >
                        <ChevronDownIcon size="16" />
                      </button>
                    </div>

                    {#if !message.reasoningSectionCollapsed}
                      {#each message.reasoning as thought, reasoningIndex}
                        <div class="rounded-lg pl-3 py-2 mb-3 w-full">
                          <div class="flex items-start w-full">
                            <div class="flex justify-between items-start gap-2 w-full">
                              <div class="flex items-start gap-2 flex-1 min-w-0">
                                <div class="shrink-0">
                                  {#if isReasoning && reasoningIndex === message.reasoning.length - 1}
                                    <div class="relative mt-3">
                                      <div class="w-2 h-2 bg-green-400 rounded-full"></div>
                                      <div
                                        class="w-2 h-2 bg-green-400 rounded-full absolute top-0 animate-ping"
                                      ></div>
                                    </div>
                                  {:else if isReasoning}
                                    <div class="w-2 h-2 bg-gray-400 rounded-full mt-3"></div>
                                  {:else}
                                    <CheckIcon size="16" class="text-gray-500 mt-2" />
                                  {/if}
                                </div>
                                <div class="text-gray-600 reasoning-container">
                                  <div
                                    class={`reasoning-content ${thought.collapsed ? 'collapsed' : 'expanded'}`}
                                  >
                                    {@html renderLatex(thought.text)}
                                  </div>
                                </div>
                              </div>
                              {#if thought.text.length > truncateLength}
                                <button
                                  class="text-gray-600 shrink-0 cursor-pointer transition-transform duration-200 mt-2"
                                  class:rotate-180={!thought.collapsed}
                                  on:click={() => {
                                    const state = {
                                      messages,
                                      isReasoning
                                    };
                                    toggleReasoning(messageIndex, reasoningIndex, state);
                                    messages = state.messages;
                                  }}
                                >
                                  <ChevronDownIcon size="16" />
                                </button>
                              {/if}
                            </div>
                          </div>
                        </div>
                      {/each}
                    {/if}
                  </div>
                {/if}
                {#if message.text !== ''}
                  <h3 class="text-sm font-semibold text-gray-600">Answer</h3>
                  <div class="mt-4 prose max-w-3xl mx-auto prose-lg">
                    {@html renderContent(message.text)}
                  </div>
                {:else}
                  <div class="animate-pulse mt-4">Thinking...</div>
                {/if}
              {/if}
            </div>
          </div>
        {/each}
      </div>
      <!-- Chat Input -->
      <div class="bottom-0 left-0 right-0 flex justify-center z-10 opacity-95">
        <div class="w-full max-w-3xl p-4 pt-0">
          <div class="relative bg-white rounded-lg shadow-sm border border-gray-200 overflow-hidden">
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
                  query();
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
                      style="background-color: {isIntegrated('GOOGLE') ? 'green' : 'red'}"
                    ></div>
                    <div
                      class="w-2 h-2 rounded-full absolute top-0 animate-ping"
                      style="background-color: {isIntegrated('GOOGLE') ? 'green' : 'red'}"
                    ></div>
                  </div>
                  <span class="text-xs text-gray-600 ml-1">Google</span>
                </div>
                <!-- Microsoft -->
                <div class="flex items-center gap-1 bg-gray-50 px-2 py-1 rounded-full">
                  <div class="relative">
                    <div
                      class="w-2 h-2 rounded-full"
                      style="background-color: {isIntegrated('MICROSOFT') ? 'green' : 'red'}"
                    ></div>
                    <div
                      class="w-2 h-2 rounded-full absolute top-0 animate-ping"
                      style="background-color: {isIntegrated('MICROSOFT') ? 'green' : 'red'}"
                    ></div>
                  </div>
                  <span class="text-xs text-gray-600 ml-1">Microsoft</span>
                </div>
                <!-- Notion -->
                <div class="flex items-center gap-1 bg-gray-50 px-2 py-1 rounded-full">
                  <div class="relative">
                    <div
                      class="w-2 h-2 rounded-full"
                      style="background-color: {isIntegrated('NOTION') ? 'green' : 'red'}"
                    ></div>
                    <div
                      class="w-2 h-2 rounded-full absolute top-0 animate-ping"
                      style="background-color: {isIntegrated('NOTION') ? 'green' : 'red'}"
                    ></div>
                  </div>
                  <span class="text-xs text-gray-600 ml-1">Notion</span>
                </div>
              </div>
              
              <!-- Send Button -->
              <button
                class="p-1.5 rounded-lg bg-primary text-white hover:bg-blue-600 transition-colors flex items-center justify-center"
                style="width: 32px; height: 32px;"
                on:click={query}
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
  {/if}
</main>

<style>
  .reasoning-container {
    position: relative;
    width: 100%;
    overflow: hidden;
  }

  .reasoning-content {
    transition: all 0.3s ease;
  }

  .reasoning-content.collapsed {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-height: 1.5em;
  }

  .reasoning-content.expanded {
    white-space: normal;
    max-height: 500px;
  }

  .scrollbar-thin {
      scrollbar-width: thin;
      -ms-overflow-style: none;
      scroll-behavior: smooth;
  }

  .scrollbar-thin::-webkit-scrollbar {
      height: 6px;
  }

  .scrollbar-thin::-webkit-scrollbar-track {
      background: #f1f1f1;
      border-radius: 3px;
      margin: 0;
  }

  .scrollbar-thin::-webkit-scrollbar-thumb {
      background: #888;
      border-radius: 3px;
  }

  .scrollbar-thin::-webkit-scrollbar-thumb:hover {
      background: #666;
  }

  .group {
      position: relative;
  }

  /* Prevent tooltip from being cut off */
  .scroll-container {
      margin-top: 0;
      padding-top: 0;
  }

  /* Remove previous tooltip styles and add these */
  .pointer-events-none {
      pointer-events: none;
  }
  
  .pointer-events-auto {
      pointer-events: auto;
  }

  /* Add tooltip styles */
  .tooltip-container {
      position: relative;
  }
  
  .tooltip {
      width: 325px;
      box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06);
      transition: opacity 0.2s, visibility 0.2s, transform 0.2s;
      transition-delay: 300ms;
      visibility: hidden;
      position: fixed;
      z-index: 50;
      transform: scaleY(0.98);
      transform-origin: top center;
  }
  
  .tooltip-container:hover .tooltip {
      opacity: 1;
      visibility: visible;
      transform: scaleY(1);
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

  .textarea-container {
    border-radius: 0.5rem;
    overflow: hidden;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .animate-spin {
    animation: spin 1s linear infinite;
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
</style>
