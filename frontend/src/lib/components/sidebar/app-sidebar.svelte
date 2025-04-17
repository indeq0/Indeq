<script lang="ts">
    import { Button } from "$lib/components/ui/button";
    import * as Tooltip from "$lib/components/ui/tooltip";
    import {ChevronsLeftIcon, ChevronsRightIcon, SidebarIcon} from 'svelte-feather-icons';
    import SidebarMain from "./sidebar-main.svelte";
    import SidebarSecondary from "./sidebar-secondary.svelte";
    import SidebarFooter from "$lib/components/sidebar/sidebar-footer.svelte";
    import MenubarNav from "$lib/components/sidebar/sidebar-menu.svelte";
    import { sidebarExpanded, toggleSidebar } from '../../stores/sidbarStore';
    import { fade } from 'svelte/transition';
</script>

<div class="grid h-screen w-full">
    <!-- Sidebar -->
    <aside class="fixed shadow-md inset-y-0 left-0 z-10 hidden md:flex h-[calc(100%-1rem)] flex-col bg-[#eeefec] backdrop-blur supports-[backdrop-filter]:bg-[#eeefec]/60 mx-2 my-2 rounded-xl transition-all duration-300 ease-in-out"
        class:w-72={$sidebarExpanded}
        class:w-[70px]={!$sidebarExpanded}>
        <!-- Header -->
        <div class="flex items-center justify-between">
            <div class="flex items-center gap-2 w-full h-full">
                <a href="/chat" 
                   class="w-full h-full rounded-md flex items-center"
                   aria-label="Home"
                >
                    <div class="flex items-center gap-2 py-2 pl-4">
                        <img src="/logo-transparent-large.svg" 
                             alt="Indeq Logo" 
                             class={"h-9 w-9"}
                        />
                        {#if $sidebarExpanded}
                            <span class="text-lg font-medium">Indeq</span>
                        {/if}
                    </div>
                </a>
            </div>
            {#if $sidebarExpanded}
            <div class="pr-3 transition-all duration-300 ease-in-out" in:fade={{ delay: 150 }}>
                <Tooltip.Root>
                    <Tooltip.Trigger asChild let:builder>
                        <Button
                            variant="ghost" 
                            size="icon"
                            on:click={toggleSidebar}
                            class="rounded-lg" 
                            aria-label={$sidebarExpanded ? "Collapse sidebar" : "Expand sidebar"}
                            builders={[builder]}
                        >
                            <SidebarIcon class="size-4"/>
                        </Button>
                    </Tooltip.Trigger>
                    <Tooltip.Content side="right" sideOffset={5}>
                        {$sidebarExpanded ? "Collapse" : "Expand"}
                    </Tooltip.Content>
                </Tooltip.Root>
            </div>
            {/if}
        </div>
        <hr class="border-t border-gray-300 mx-3"/>
        <!-- Main navigation -->
        <SidebarMain />
        <nav class="absolute right-0 top-0 h-full translate-x-1/2">
            <div class="flex h-full items-center">
                <Tooltip.Root>
                    <Tooltip.Trigger asChild let:builder>
                        <Button
                            variant="ghost" 
                            size="icon"
                            on:click={toggleSidebar}
                            class="rounded-lg bg-[#eeefec] border shadow-sm" 
                            aria-label={$sidebarExpanded ? "Collapse sidebar" : "Expand sidebar"}
                            builders={[builder]}
                        >
                            {#if $sidebarExpanded}
                                <ChevronsLeftIcon class="size-5"/>
                            {:else}
                                <ChevronsRightIcon class="size-5"/>
                            {/if}
                        </Button>
                    </Tooltip.Trigger>
                    <Tooltip.Content side="right" sideOffset={5}>
                        {$sidebarExpanded ? "Collapse" : "Expand"}
                    </Tooltip.Content>
                </Tooltip.Root>
            </div>
        </nav>
        <!-- Secondary navigation -->
        <SidebarSecondary/>
        <hr class="border-t border-gray-300 mx-3 mt-2"/>
        <SidebarFooter/>
    </aside>
    <!--Menubar-->
    <MenubarNav/>
    <!-- Main content -->
    <div class="flex flex-col transition-all duration-300 bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60"
        class:md:pl-56={$sidebarExpanded} 
        class:md:pl-14={!$sidebarExpanded}>
        <div class="flex flex-col transition-all duration-300 pb-16 md:pb-0">
            <slot/>
        </div>
    </div>
</div>
