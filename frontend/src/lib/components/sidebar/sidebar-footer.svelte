<script lang="ts">
    import { MenuIcon, LogOutIcon, CodesandboxIcon } from 'svelte-feather-icons';
    import * as Avatar from "$lib/components/ui/avatar";
    import * as Popover from "$lib/components/ui/popover";
    import { Button } from "$lib/components/ui/button";
    import { userStore } from '../../stores/userStore';
    import { Routes } from '$lib/config/sidebar-routes';
    import { sidebarExpanded } from '../../stores/sidbarStore';
    import { goto } from '$app/navigation';

    // Create a user object with either the store data or a fallback
    $: user = $userStore.user || {
        name: "Guest",
        email: "guest@example.com",
        avatar: 3,
        alias: "Guest"
    };

    async function handleLogout() {
        try {
            await fetch('/api/logout', { method: 'POST' });
            userStore.clearUser();
            goto('/login');
        } catch (error) {
            console.error('Error during logout:', error);
            userStore.clearUser();
            goto('/login');
        }
    }
</script>
  
<div class="py-2 shrink-0" class:px-3={$sidebarExpanded} class:px-2={!$sidebarExpanded}>
    <Popover.Root>
        <Popover.Trigger asChild let:builder>
            <Button
                variant="ghost"
                size="sm"
                class="w-full justify-center lg:justify-start gap-2 py-6"
                builders={[builder]}
            >
                <Avatar.Root class="h-8 w-8 rounded-full mr-1">
                    <Avatar.Image src={`/gradients/gradient-${user.avatar}.png`} alt={user.name} />
                </Avatar.Root>
                <div class="hidden lg:grid flex-1 text-left text-sm leading-tight">
                    <span class="truncate font-sm">{user.name}</span>
                </div>
                <MenuIcon class="hidden lg:block ml-auto size-4"/>
            </Button>
        </Popover.Trigger>
        <Popover.Content
            class="w-[var(--radix-popover-trigger-width)] {$sidebarExpanded ? 'min-w-72' : 'min-w-48'} rounded-lg p-2"
            side={"top"}
            sideOffset={0}
        >
            <Button
                href={Routes.profileAccount}
                variant="ghost"
                class="flex items-center justify-start px-0 py-1.5 text-sm space-x-2"         
            >
                <Avatar.Root class="h-8 w-8 rounded-full">
                    <Avatar.Image src={`/gradients/gradient-${user.avatar}.png`} alt={user.name} />
                </Avatar.Root>
                <div class="grid flex-1 text-left text-sm leading-tight">
                    <span class="truncate font-sm">{user.name}</span>
                    <span class="truncate text-xs font-sm">{user.email}</span>
                </div>
            </Button>
            <hr class="my-2" />     
            <Button
                variant="ghost" 
                class="flex items-center justify-start px-0 py-1.5 text-sm space-x-2 w-full"
                on:click={handleLogout}
            >
                <LogOutIcon class="h-4 w-4 ml-2" />
                <span class="truncate text-xs font-sm">Log Out</span>
            </Button>
        </Popover.Content>
    </Popover.Root>
</div>