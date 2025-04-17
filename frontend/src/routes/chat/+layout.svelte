<script>
  import ModelSelector from '$lib/components/sidebar/model-selector.svelte';
  import { onNavigate } from '$app/navigation';

onNavigate((navigation) => {
	if (!document.startViewTransition) return;

	return new Promise((resolve) => {
		document.startViewTransition(async () => {
			resolve();
			await navigation.complete;
		});
	});
});
</script>

<div>
    <div class="sticky top-0 pt-2 ml-3" style="view-transition-name: none;">
        <ModelSelector />
    </div>
      
    <slot />
</div>

<style>
    @keyframes move-up {
        from {
            transform: translateY(33vh);
        }
        to {
            transform: translateY(0);
        }
    }

    @keyframes fly-in-from-top {
        from {
            transform: translateY(-250px);
        }
        to {
            transform: translateY(0);
        }
    }

    /* Target the chat input specifically */
    :global(.chat-input-container) {
        view-transition-name: chat-input;
    }

    :global(.chat-input-container)::view-transition-old(chat-input) {
        animation: 500ms cubic-bezier(0.4, 0, 0.2, 1) both move-up;
    }

    :global(.chat-input-container)::view-transition-new(chat-input) {
        animation: 500ms cubic-bezier(0.4, 0, 0.2, 1) both move-up;
    }

    /* Welcome text transition */
    :global(.welcome-text) {
        view-transition-name: welcome-text;
    }

    :global(.welcome-text)::view-transition-old(welcome-text) {
        animation: 300ms cubic-bezier(0.4, 0, 0.2, 1) both fly-in-from-top;
    }

    :global(.welcome-text)::view-transition-new(welcome-text) {
        animation: 300ms cubic-bezier(0, 0, 0.2, 1) both fly-in-from-top;
    }
</style>
