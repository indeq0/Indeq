<script lang="ts">
	import NavHistory from "./nav-history.svelte";
	import { sidebarExpanded, toggleSidebar } from '../../stores/sidbarStore';
	import { Button } from "$lib/components/ui/button";
	import { GitBranchIcon, MessageCircleIcon, SettingsIcon, UserIcon } from 'svelte-feather-icons';
	import * as Tooltip from "$lib/components/ui/tooltip";
	import { page } from '$app/stores';
	import { fade } from 'svelte/transition';
	import { conversationStore } from '../../stores/conversationStore';
	import { onMount } from 'svelte';

	let loading = true;
	let isMac = false;

	onMount(() => {
		// Add keyboard shortcut listener
		const handleKeyDown = (e: KeyboardEvent) => {
			if ((e.metaKey || e.ctrlKey) && e.key === '\\') {
				e.preventDefault();
				toggleSidebar();
			}
		};

		window.addEventListener('keydown', handleKeyDown);
		
		// Initialize async data
		(async () => {
			await conversationStore.fetchConversations();
			loading = false;
			isMac = navigator.platform.toUpperCase().indexOf('MAC') >= 0;
		})();

		return () => {
			window.removeEventListener('keydown', handleKeyDown);
		};
	});

	$: conversations = $conversationStore.headers;
	$: error = $conversationStore.error;
	$: loading = $conversationStore.loading;
	
	function handleWheel(event: WheelEvent) {
		event.stopPropagation();
		
		const element = event.currentTarget as HTMLElement;
		const { scrollTop, scrollHeight, clientHeight } = element;
		
		// If scrolling up and not at the top, or scrolling down and not at the bottom
		// we need to manually handle the scroll to prevent default
		if ((event.deltaY < 0 && scrollTop > 0) || 
			(event.deltaY > 0 && scrollTop + clientHeight < scrollHeight)) {
			event.preventDefault();
			element.scrollTop += event.deltaY;
		}
	}
</script>

<nav class={`flex flex-col gap-1 pt-2 h-full ${$sidebarExpanded ? "px-3" : ""}`}>
	{#if $sidebarExpanded}
	
	<Button 
		href="/chat" 
		variant="outline"
		class="w-full justify-between gap-2 mt-1 rounded-lg bg-primary text-white transition-all duration-300 ease-in-out shrink-0"
	>
		<div class="flex items-center gap-2">
			<MessageCircleIcon class="size-5" />
			{#if $sidebarExpanded}
				<span class="transition-all duration-300 ease-in-out" in:fade={{ delay: 150 }}>New Chat</span>
			{/if}
		</div>
		{#if $sidebarExpanded}
			<span class="transition-all duration-300 ease-in-out" in:fade={{ delay: 150 }}>{isMac ? '⌘K' : 'Ctrl+K'}</span>
		{/if}
	</Button>
	<h2 class="text-sm font-medium text-gray-700 mr-2 mt-2 shrink-0">
		Shortcuts	
	</h2>
	<div class="flex my-1 mb-2 gap-1 transition-all duration-300 ease-in-out shrink-0">
		<!-- Chat -->
		<Tooltip.Root>
			<Tooltip.Trigger asChild let:builder>
				<Button 
					href="/chat"
					variant="ghost" 
					size="icon" 
					class="rounded-xl hover:bg-[#e6e4e3] {$page.url.pathname === '/chat' ? 'bg-[#e6e4e3]' : ''}"
					builders={[builder]}
				>
					<MessageCircleIcon class="size-5 stroke-1.5 {$page.url.pathname === '/chat' ? 'stroke-gray-700' : 'stroke-gray-500'}" />
				</Button>
			</Tooltip.Trigger>
			<Tooltip.Content side="bottom" class="bg-gray-800 text-white" sideOffset={5}>Chat</Tooltip.Content>
		</Tooltip.Root>
		<!-- Integration -->
		<Tooltip.Root>
			<Tooltip.Trigger asChild let:builder>
				<Button 
					href="/profile/integration"
					variant="ghost" 
					size="icon" 
					class="rounded-xl hover:bg-[#e6e4e3] {$page.url.pathname === '/profile/integration' ? 'bg-[#e6e4e3]' : ''}"
					builders={[builder]}
				>
					<GitBranchIcon class="size-5 stroke-1.5 {$page.url.pathname === '/profile/integration' ? 'stroke-gray-700' : 'stroke-gray-500'}" />
				</Button>
			</Tooltip.Trigger>
			<Tooltip.Content side="bottom" class="bg-gray-800 text-white" sideOffset={5}>Integrations</Tooltip.Content>
		</Tooltip.Root>
		<!-- Settings -->
		<Tooltip.Root>
			<Tooltip.Trigger asChild let:builder>
				<Button 
					href="/profile/settings"
					variant="ghost" 
					size="icon" 
					class="rounded-xl hover:bg-[#e6e4e3] {$page.url.pathname === '/profile/settings' ? 'bg-[#e6e4e3]' : ''}"
					builders={[builder]}
				>
					<SettingsIcon class="size-5 stroke-1.5 {$page.url.pathname === '/profile/settings' ? 'stroke-gray-700' : 'stroke-gray-500'}" />
				</Button>
			</Tooltip.Trigger>
			<Tooltip.Content side="bottom" class="bg-gray-800 text-white" sideOffset={5}>Settings</Tooltip.Content>
		</Tooltip.Root>
	</div>
	{:else}
	<div class="flex flex-col gap-1 items-center mx-auto my-1 transition-all duration-300 ease-in-out shrink-0">
		<Tooltip.Root>
			<Tooltip.Trigger asChild let:builder>
				<Button 
					href="/chat"
					variant="ghost" 
					size="icon" 
					class="rounded-xl hover:bg-[#e6e4e3] {$page.url.pathname === '/chat' ? 'bg-[#e6e4e3]' : ''}"
					builders={[builder]}
				>
					<MessageCircleIcon class="size-5 stroke-1.5 {$page.url.pathname === '/chat' ? 'stroke-gray-700' : 'stroke-gray-500'}" />
				</Button>
			</Tooltip.Trigger>
			<Tooltip.Content side="right" class="bg-gray-800 text-white" sideOffset={5}>Chat</Tooltip.Content>
		</Tooltip.Root>
		<Tooltip.Root>
			<Tooltip.Trigger asChild let:builder>
				<Button 
					href="/profile/integration"
					variant="ghost" 
					size="icon" 
					class="rounded-xl hover:bg-[#e6e4e3] {$page.url.pathname === '/profile/integration' ? 'bg-[#e6e4e3]' : ''}"
					builders={[builder]}
				>
					<GitBranchIcon class="size-5 stroke-1.5 {$page.url.pathname === '/profile/integration' ? 'stroke-gray-700' : 'stroke-gray-500'}" />
				</Button>
			</Tooltip.Trigger>
			<Tooltip.Content side="right" class="bg-gray-800 text-white" sideOffset={5}>Integrations</Tooltip.Content>
		</Tooltip.Root>
		<!-- Settings -->
		<Tooltip.Root>
			<Tooltip.Trigger asChild let:builder>
				<Button 
					href="/profile/settings"
					variant="ghost" 
					size="icon" 
					class="rounded-xl hover:bg-[#e6e4e3] {$page.url.pathname === '/profile/settings' ? 'bg-[#e6e4e3]' : ''}"
					builders={[builder]}
				>
					<SettingsIcon class="size-5 {$page.url.pathname === '/profile/settings' ? 'stroke-gray-900' : 'stroke-gray-700'}" />
				</Button>
			</Tooltip.Trigger>
			<Tooltip.Content side="right" class="bg-gray-800 text-white" sideOffset={5}>Settings</Tooltip.Content>
		</Tooltip.Root>
	</div>
	{/if}
	{#if $sidebarExpanded}
		<div class="flex items-center py-1 mt-1 mb-1 transition-all duration-300 ease-in-out shrink-0">
			<h2 class="text-sm font-medium text-gray-700 mr-2">
				History	
			</h2>
		</div>
		<div 
			class="overflow-y-auto flex-1 sidebar-scroll mr-[-12px] pr-[8px] w-[calc(100%+12px)] overscroll-contain"
			on:wheel={handleWheel}
		>
			{#if loading}
				<div class=""></div>
			{:else if error}
				<div class="text-center py-2 text-sm text-red-500">Failed to load history</div>
			{:else if conversations.length === 0}
				<div class="text-center py-2 text-sm text-gray-500">No conversations yet</div>
			{:else}
				{#each conversations as conversation}
					<div class="w-full">
						<NavHistory item={{ id: conversation.conversation_id, title: conversation.title }} expanded={$sidebarExpanded} />
					</div>
				{/each}
			{/if}
		</div>
	{/if}
</nav>