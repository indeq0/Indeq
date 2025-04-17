<script lang="ts">
	import NavHistory from "./nav-history.svelte";
	import { sidebarExpanded } from '../../stores/sidbarStore';
	import { Button } from "$lib/components/ui/button";
	import { GitBranchIcon, MessageCircleIcon, UserIcon } from 'svelte-feather-icons';
	import * as Tooltip from "$lib/components/ui/tooltip";
	import { page } from '$app/stores';
	import { fade } from 'svelte/transition';
	//TODO: Temporary static history data (replace with a historyStore)
	const tempHistory = [
	  { id: "chat-1", title: "This is a temporary search history test to see how it would look"},
	  { id: "chat-2", title: "This is just a visual to see what it will look like once chat history is up"},
	  { id: "chat-3", title: "I dont know what this typing is doing but did you see the double the the" },
	];
</script>

<nav class={`flex flex-col gap-1 pt-2  ${$sidebarExpanded ? "px-3" : ""}`}>
	{#if $sidebarExpanded}
	
	<Button 
		href="/chat" 
		variant="outline"
		class="w-full justify-center gap-2 mt-1 rounded-lg bg-primary text-white transition-all duration-300 ease-in-out"
	>
		<MessageCircleIcon class="size-5" />
		{#if $sidebarExpanded}
			<span class="transition-all duration-300 ease-in-out" in:fade={{ delay: 150 }}>New Chat</span>
		{/if}
	</Button>
	<h2 class="text-sm font-medium text-gray-700 mr-2 mt-2">
		Shortcuts	
	</h2>
	<div class="flex my-1 gap-1 transition-all duration-300 ease-in-out">
		<!-- Chat -->
		<Tooltip.Root>
			<Tooltip.Trigger asChild let:builder>
				<Button 
					href="/chat"
					variant="ghost" 
					size="icon" 
					class="rounded-lg hover:bg-[#e6e4e3] {$page.url.pathname === '/chat' ? 'bg-[#e6e4e3]' : ''}"
					builders={[builder]}
				>
					<MessageCircleIcon class="size-5 stroke-1.5 {$page.url.pathname === '/chat' ? 'stroke-gray-900' : 'stroke-gray-700'}" />
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
					class="rounded-lg hover:bg-[#e6e4e3] {$page.url.pathname === '/profile/integration' ? 'bg-[#e6e4e3]' : ''}"
					builders={[builder]}
				>
					<GitBranchIcon class="size-5 stroke-1.5 {$page.url.pathname === '/profile/integration' ? 'stroke-gray-900' : 'stroke-gray-700'}" />
				</Button>
			</Tooltip.Trigger>
			<Tooltip.Content side="bottom" class="bg-gray-800 text-white" sideOffset={5}>Integrations</Tooltip.Content>
		</Tooltip.Root>
		<!-- Profile -->
		<Tooltip.Root>
			<Tooltip.Trigger asChild let:builder>
				<Button 
					href="/profile/account"
					variant="ghost" 
					size="icon" 
					class="rounded-lg hover:bg-[#e6e4e3] {$page.url.pathname === '/profile/account' ? 'bg-[#e6e4e3]' : ''}"
					builders={[builder]}
				>
					<UserIcon class="size-5 stroke-1.5 {$page.url.pathname === '/profile/account' ? 'stroke-gray-900' : 'stroke-gray-700'}" />
				</Button>
			</Tooltip.Trigger>
			<Tooltip.Content side="bottom" class="bg-gray-800 text-white" sideOffset={5}>Profile</Tooltip.Content>
		</Tooltip.Root>
	</div>
	{:else}
	<div class="flex flex-col gap-1 items-center mx-auto my-1 transition-all duration-300 ease-in-out">
		<Tooltip.Root>
			<Tooltip.Trigger asChild let:builder>
				<Button 
					href="/chat"
					variant="ghost" 
					size="icon" 
					class="rounded-lg hover:bg-[#e6e4e3] {$page.url.pathname === '/chat' ? 'bg-[#e6e4e3]' : ''}"
					builders={[builder]}
				>
					<MessageCircleIcon class="size-5 {$page.url.pathname === '/chat' ? 'stroke-gray-900' : 'stroke-gray-700'}" />
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
					class="rounded-lg hover:bg-[#e6e4e3] {$page.url.pathname === '/profile/integration' ? 'bg-[#e6e4e3]' : ''}"
					builders={[builder]}
				>
					<GitBranchIcon class="size-5 {$page.url.pathname === '/profile/integration' ? 'stroke-gray-900' : 'stroke-gray-700'}" />
				</Button>
			</Tooltip.Trigger>
			<Tooltip.Content side="right" class="bg-gray-800 text-white" sideOffset={5}>Integrations</Tooltip.Content>
		</Tooltip.Root>
	</div>
	{/if}
	{#if $sidebarExpanded}
		<div class="flex items-center py-1 mt-1 mb-1 transition-all duration-300 ease-in-out">
			<h2 class="text-sm font-medium text-gray-700 mr-2">
				History	
			</h2>
		</div>
	{/if}
</nav>
<nav class="grid gap-0 pl-6 pr-2 p-0 transition-all duration-300 ease-in-out">
    {#each tempHistory as item}
        <NavHistory {item} expanded={$sidebarExpanded} />
    {/each}
</nav>