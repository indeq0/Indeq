<script lang="ts">
  import * as Card from '$lib/components/ui/card/index.js';
  import { Label } from '$lib/components/ui/label/index.js';
  import { Input } from '$lib/components/ui/input/index.js';
  import { Button } from '$lib/components/ui/button/index.js';
  import { enhance } from '$app/forms';
  import { toast } from 'svelte-sonner';
  import { goto } from '$app/navigation';

  // Check for success or error in the server response
  $: if (form?.success) {
    goto('/chat');
  }

  $: if (form?.error) {
    toast.error(form.error);
  }

  export let form;
  
  let isGoogleLoading = false;

  async function handleGoogleLogin() {
    try {
      isGoogleLoading = true;
      
      // Redirect to Google's consent screen
      await goto('/sso/GOOGLE');

      
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Failed to initiate Google login');
      isGoogleLoading = false;
    }
  }
</script>

<svelte:head>
  <title>Login | Indeq</title>
  <meta name="description" content="Login to Indeq" />
</svelte:head>

<div class="min-h-screen flex items-center justify-center">
  <div class="flex flex-col gap-4 min-w-96">
    <Card.Root class="w-full max-w-sm mx-auto">
      <Card.Header class="space-y-1">
        <Card.Title class="text-2xl">Welcome back</Card.Title>
        <Card.Description>Enter your email below to login to your account</Card.Description>
      </Card.Header>
      <form method="POST" use:enhance>
        <Card.Content class="grid gap-4">
          <Button
            variant="outline"
            type="button"
            on:click={handleGoogleLogin}
            disabled={isGoogleLoading}
          >
            {#if isGoogleLoading}
              <span class="mr-2 h-6 w-6 animate-spin">⌛</span>
              Connecting...
            {:else}
              <img src="/google.svg" alt="Google logo" class="mr-2 h-6 w-6" />
              Login with Google
            {/if}
          </Button>
          <div class="relative">
            <div class="absolute inset-0 flex items-center">
              <span class="w-full border-t"></span>
            </div>
            <div class="relative flex justify-center text-xs">
              <span class="bg-card text-muted-foreground px-2"> Or continue with </span>
            </div>
          </div>
          <div class="grid gap-2">
            <Label for="email">Email</Label>
            <Input id="email" name="email" type="email" placeholder="m@example.com" />
          </div>
          <div class="grid gap-2">
            <Label for="password">Password</Label>
            <Input id="password" name="password" type="password" />
          </div>
          {#if form?.error}
            <div class="text-destructive text-sm">{form.error}</div>
          {/if}
        </Card.Content>
        <Card.Footer class="flex flex-col gap-4">
          <Button type="submit" class="w-full">Login</Button>
          <Card.Description class="relative text-center">
            Don't have an account? <a class="underline hover:text-primary" href="/register"
              >Sign up</a
            >
          </Card.Description>
        </Card.Footer>
      </form>
    </Card.Root>
    <Card.Description class="text-center text-sm text-muted-foreground">&nbsp;</Card.Description>
  </div>
</div>
