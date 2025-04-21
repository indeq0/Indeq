<script lang="ts">
    import { modelStore } from "$lib/stores/modelStore";
    import { fade } from 'svelte/transition';
    import { ChevronDownIcon } from "svelte-feather-icons";
    import { onMount } from "svelte";

    let selectedModel: string;
    let isDropdownOpen = false;
    let dropdownContainer: HTMLDivElement;
    
    // Subscribe to the model store
    modelStore.subscribe(value => {
        selectedModel = value;
    });

    function handleModelChange(modelId: string) {
        modelStore.setModel(modelId);
        isDropdownOpen = false;
    }

    function toggleDropdown() {
        isDropdownOpen = !isDropdownOpen;
    }
    
    onMount(() => {
        const handleClickOutside = (event: MouseEvent) => {
            if (dropdownContainer && !dropdownContainer.contains(event.target as Node)) {
                isDropdownOpen = false;
            }
        };
        
        document.addEventListener('click', handleClickOutside);
        
        return () => {
            document.removeEventListener('click', handleClickOutside);
        };
    });

    // Get model logo based on model ID
    function getModelLogo(modelId: string): string {
        const logos: Record<string, string> = {
            'gemini-2.0-flash': '/gemini.svg',
            'llama-4.0-maverick': '/meta.svg', // Using microsoft logo for llama
            'qwq-32b': '/qwen.png',
            'gpt-4o-mini': '/openai.svg', 
            'deepseek-r1-distill-qwen-32b': '/deepseek.svg',
        };
        
        return logos[modelId] || '/microsoft.png';
    }
</script>

<div class="relative p-2" bind:this={dropdownContainer}>
    <div class="transition-all duration-300 ease-in-out" in:fade={{ delay: 100 }}>
        <div class="relative">
            <button 
                type="button"
                on:click|stopPropagation={toggleDropdown}
                class="flex items-center h-9 text-sm px-2 cursor-pointer hover:text-gray-800 text-gray-800"
            >
                <img 
                    src={getModelLogo(selectedModel)} 
                    alt="Model logo" 
                    class={`mr-2 h-5 w-5`} 
                />
                <span class="flex-1 text-left truncate">
                    {modelStore.getModelName(selectedModel)}
                </span>
                <ChevronDownIcon class="h-4 w-4 text-gray-700 font-light ml-1" />
            </button>
            
            {#if isDropdownOpen}
                <div class="absolute z-50 mt-1 min-w-[180px] bg-white border border-gray-200 rounded-lg shadow-lg overflow-hidden">
                    <ul>
                        {#each modelStore.availableModels as model}
                            <li>
                                <button
                                    type="button"
                                    on:click={() => handleModelChange(model.id)}
                                    class="flex items-center w-full px-3 py-2 text-sm hover:bg-gray-50 focus:bg-gray-50 focus:outline-none {selectedModel === model.id ? 'bg-gray-50' : ''}"
                                >
                                    <img 
                                        src={getModelLogo(model.id)} 
                                        alt="Model logo" 
                                        class={`mr-2 h-5 w-5`} 
                                    />
                                    <span>{model.name}</span>
                                </button>
                            </li>
                        {/each}
                    </ul>
                </div>
            {/if}
        </div>
    </div>
</div> 