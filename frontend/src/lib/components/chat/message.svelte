<script lang="ts">
  import type { ChatMessage } from "$lib/types/chat";
  import { renderContent, renderLatex, initCodeCopyButtons } from "$lib/utils/katex";
  import { scrollToPosition, handleScroll, initScrollCheck, positionTooltip, hideTooltip } from "$lib/utils/sources";
  import { toggleReasoning } from "$lib/utils/chat";
  import { CheckIcon, ChevronDownIcon, FileIcon, FileTextIcon, HardDriveIcon, CopyIcon, RefreshCwIcon } from "svelte-feather-icons";
  import { Button } from "$lib/components/ui/button";
  import * as Tooltip from "$lib/components/ui/tooltip";
  import { toast } from "svelte-sonner";
  import { onMount } from "svelte";
  import 'prismjs/themes/prism.css';

  export let message: ChatMessage;
  export let messageIndex: number;
  export let messages: ChatMessage[] = [];
  export let isReasoning: boolean = false;
  export let isStreaming: boolean = false;
  export let truncateLength: number = 80;
  export let updateMessages: (msgs: ChatMessage[]) => void;
  export let retryMessage: (query: string) => void;

  function copyMessage() {
    if (message.text) {
      navigator.clipboard.writeText(message.text);
      toast.success('Copied to clipboard', { duration: 750});
    }
  }

  // Initialize code copy buttons after the component is mounted
  onMount(() => {
    if (message.sender !== 'user') {
      initCodeCopyButtons();
    }
  });

  // Watch for changes in message text to re-initialize copy buttons
  $: if (message.text && message.sender !== 'user') {
    // Use setTimeout to ensure the DOM has been updated
    setTimeout(initCodeCopyButtons, 0);
  }
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
        <div class="message-wrapper">
          <div class="prose max-w-3xl mx-auto prose-lg">
            {@html renderContent(message.text)}
          </div>
          {#if !isStreaming}
            <!-- Message toolbar -->
            <div class="message-toolbar">
              <Tooltip.Root>
                <Tooltip.Trigger asChild let:builder>
                  <Button 
                    variant="ghost" 
                    size="icon" 
                    class="rounded-xl hover:bg-[#e6e4e3]"
                    builders={[builder]}
                    on:click={copyMessage}
                  >
                    <CopyIcon size="15" />
                  </Button>
                </Tooltip.Trigger>
                <Tooltip.Content side="bottom" class="bg-gray-800 text-white" sideOffset={5}>Copy</Tooltip.Content>
              </Tooltip.Root>
              <Tooltip.Root>
                <Tooltip.Trigger asChild let:builder>
                  <Button
                    variant="ghost" 
                    size="icon" 
                    class="rounded-xl hover:bg-[#e6e4e3]"
                    builders={[builder]}
                    on:click={() => {
                      const previousIndex = messageIndex - 1;
                      if (previousIndex >= 0) {
                        const previousMessage = messages[previousIndex];
                        if (previousMessage.sender === 'user') {
                          retryMessage(previousMessage.text);
                        } else {
                          for (let i = previousIndex; i >= 0; i--) {
                            if (messages[i].sender === 'user') {
                              retryMessage(messages[i].text);
                              break;
                            }
                          }
                        }
                      }
                    }}
                  >
                    <RefreshCwIcon size="15" />
                  </Button>
                </Tooltip.Trigger>
                <Tooltip.Content side="bottom" class="bg-gray-800 text-white" sideOffset={5}>Retry Message</Tooltip.Content>
              </Tooltip.Root>
              <div class="text-sm text-gray-500 ml-2 flex items-center">
                Generated with {message.model}
              </div>
            </div>
          {/if}
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

  /* Message toolbar styles */
  .message-wrapper {
    position: relative;
    margin-top: 1rem;
  }
  
  .message-toolbar {
    display: flex;
    padding: 4px 0;
    gap: 8px;
    opacity: 0;
    transition: opacity 0.2s ease;
    margin-top: 0.5rem;
  }

  .message-wrapper:hover .message-toolbar {
    opacity: 1;
  }

  .toolbar-icon {
    color: #6b7280;
    background: transparent;
    border: none;
    cursor: pointer;
    padding: 4px;
    border-radius: 4px;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .toolbar-icon:hover {
    background-color: #f3f4f6;
    color: #374151;
  }

  /* Code block styles */
  :global(.code-block-wrapper) {
    margin: 1rem 0;
    border-radius: 0.5rem;
    overflow: hidden;
    background-color: #fcfcfc;
    border: 1px solid #eaecef;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05);
  }

  :global(.code-block-header) {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.5rem 1rem;
    background-color: #f6f8fa;
    border-bottom: 1px solid #eaecef;
    font-family: 'Monospace', monospace;
    font-size: 0.875rem;
  }

  :global(.code-language) {
    font-weight: 500;
    color: #57606a;
    text-transform: none;
    font-size: 0.8rem;
    letter-spacing: 0.01em;
    font-family: 'Work Sans', sans-serif;
  }

  :global(.copy-code-button) {
    background-color: transparent;
    color: #57606a;
    border: none;
    border-radius: 0.25rem;
    padding: 0.5rem;
    cursor: pointer;
    transition: all 0.2s;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  :global(.copy-code-button:hover) {
    background-color: #e1e4e8;
    color: #24292e;
  }

  :global(.copy-code-button svg) {
    width: 16px;
    height: 16px;
  }

  :global(.copied-icon) {
    color: #22863a;
  }

  :global(.code-block-wrapper pre) {
    margin: 0;
    padding: 1rem;
    overflow-x: auto;
    background-color: #fcfcfc;
  }

  :global(.code-block-wrapper code) {
    font-family: 'Monospace', monospace;
    font-size: 0.875rem;
    line-height: 1.5;
  }
  
  /* Inline code block styles */
  :global(.inline-code) {
    font-family: 'Monospace', monospace;
    font-size: 0.85em;
    background-color: #f6f8fa;
    padding: 0.3em 0.6em;
    border-radius: 4px;
    border: 1px solid #eaecef;
    white-space: nowrap;
    color: #24292e;
    margin: 0 0.2em;
  }
  
  /* Override Prism.js token colors for a lighter theme */
  :global(.token.comment),
  :global(.token.prolog),
  :global(.token.doctype),
  :global(.token.cdata) {
    color: #6e7781;
  }
  
  :global(.token.punctuation) {
    color: #57606a;
  }
  
  :global(.token.property),
  :global(.token.tag),
  :global(.token.boolean),
  :global(.token.number),
  :global(.token.constant),
  :global(.token.symbol) {
    color: #0550ae;
  }
  
  :global(.token.selector),
  :global(.token.attr-name),
  :global(.token.string),
  :global(.token.char),
  :global(.token.builtin) {
    color: #0a3069;
  }
  
  :global(.token.operator),
  :global(.token.entity),
  :global(.token.url),
  :global(.language-css .token.string),
  :global(.style .token.string) {
    color: #57606a;
  }
  
  :global(.token.atrule),
  :global(.token.attr-value),
  :global(.token.keyword) {
    color: #cf222e;
  }
  
  :global(.token.function),
  :global(.token.class-name) {
    color: #8250df;
  }
  
  :global(.token.regex),
  :global(.token.important) {
    color: #d4a72c;
  }
</style> 