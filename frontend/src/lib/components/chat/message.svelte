<script lang="ts">
  import type { ChatMessage } from "$lib/types/chat";
  import { renderContent, renderLatex } from "$lib/utils/katex";
  import { scrollToPosition, handleScroll, initScrollCheck, positionTooltip, hideTooltip } from "$lib/utils/sources";
  import { toggleReasoning } from "$lib/utils/chat";
  import { CheckIcon, ChevronDownIcon, FileIcon, FileTextIcon, HardDriveIcon } from "svelte-feather-icons";
  
  export let message: ChatMessage;
  export let messageIndex: number;
  export let messages: ChatMessage[] = [];
  export let isReasoning: boolean = false;
  export let truncateLength: number = 80;
  export let updateMessages: (msgs: ChatMessage[]) => void;
</script>

<div class="space-y-4">
  <div class="prose max-w-3xl mx-auto prose-lg w-full overflow-x-hidden">
    {#if message.sender === 'user'}
      <div class="font-bold prose-xl whitespace-normal overflow-hidden w-full">{message.text}</div>
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
                            scrollToPosition(messages, scrollContainer as HTMLElement, messageIndex, (msgs) => {
                                messages = msgs;
                            });
                        }
                    }}
                >
                    {message.sourcesScrollAtEnd ? '← Scroll to start' : 'Scroll to end →'}
                </button>
                {/if}
            </div>

          <!-- Sources -->
          <div class="relative !overflow-hidden w-full">
            <div 
                class="flex overflow-x-auto overflow-y-hidden pb-4 gap-3 scrollbar-thin scroll-container max-w-full"
                on:scroll={(e) => {
                    messages = handleScroll(messages, e, messageIndex);
                    updateMessages(messages);
                }}
                use:initScrollCheck={{
                  node: document.querySelector('.conversation-container'), 
                  messages, 
                  messageIndex,
                  setMessages: (msgs) => {
                      messages = msgs;
                      updateMessages(messages);
                  }
                }}
            >
                {#each message.sources as source, sourceIndex}
                    <div class="flex-none w-[325px]">
                        <div 
                            class="bg-white rounded-xl p-3 hover:bg-gray-100 transition-colors duration-200 shadow-sm border border-gray-100 relative tooltip-container"
                            on:mouseenter={positionTooltip}
                            on:mouseleave={hideTooltip}
                            on:click={() => window.open(source.fileUrl, '_blank')}
                            on:keydown={(e) => e.key === 'Enter' && window.open(source.fileUrl, '_blank')}
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
                            <div class="tooltip fixed bg-white opacity-0 pointer-events-none text-gray-800 p-3 rounded-xl shadow-md text-sm z-20 max-w-full whitespace-normal border border-gray-100" 
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
                  updateMessages(messages.map((msg, idx) =>
                    idx === messageIndex
                      ? {
                          ...msg,
                          reasoningSectionCollapsed: !msg.reasoningSectionCollapsed
                        }
                      : msg
                  ));
                }
              }}
            >
              <ChevronDownIcon size="16" />
            </button>
          </div>

          {#if !message.reasoningSectionCollapsed}
            {#each message.reasoning as thought, reasoningIndex}
              <div class="pl-3 py-2 mb-3 w-full">
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
                          updateMessages(state.messages);
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

<style>
  .reasoning-container {
    position: relative;
    width: 100%;
    overflow: hidden;
  }

  .reasoning-content {  
    overflow: hidden;
  }

  .reasoning-content.collapsed {
    max-height: 1.5em;
    text-overflow: ellipsis;
    white-space: nowrap;
    overflow: hidden;
  }

  .reasoning-content.expanded {
    max-height: 500px;
    white-space: normal;
    overflow: hidden;
  }
  
  /* Tooltip styles */
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
</style> 