<script lang="ts">
	import { Button } from "$lib/components/ui/button";
	import * as Tooltip from "$lib/components/ui/tooltip";
	import { navigation } from '$lib/config/navigation';
</script>

<nav class="fixed bottom-0 left-0 right-0 z-20 md:hidden bg-background border-t">
	<div class="flex justify-center items-center p-2 gap-12">
		{#each [...navigation.main, ...navigation.secondary] as item}
			<div class="flex flex-col items-center">
				<Tooltip.Root>
					<Tooltip.Trigger asChild let:builder>
						<Button
							href={item.url}
							variant="ghost"
							size="icon"
							class="rounded-lg"
							aria-label={item.label}
							builders={[builder]}
						>
							<svelte:component this={item.icon} class="size-5"/>
						</Button>
					</Tooltip.Trigger>
					<Tooltip.Content side="top" sideOffset={5}>{item.label}</Tooltip.Content>
				</Tooltip.Root>
				<span class="text-xs mt-1">{item.label}</span>
			</div>
		{/each}
	</div>
</nav>