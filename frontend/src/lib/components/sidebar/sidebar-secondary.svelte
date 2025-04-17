<script lang="ts">
	import NavItem from "./nav-item.svelte";
	import { sidebarExpanded } from '../../stores/sidbarStore';
	import { navigation } from '$lib/config/navigation';
</script>

<nav class={`flex flex-col gap-1 mt-auto transition-all duration-300 ease-in-out ${$sidebarExpanded ? "px-3" : ""}`}>
	{#each navigation.secondary as item}
		<NavItem {item} expanded={$sidebarExpanded} />
	{/each}
</nav>