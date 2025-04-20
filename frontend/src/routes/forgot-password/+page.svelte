<script lang="ts">
    import * as Card from '$lib/components/ui/card/index.js';
    import { Input } from '$lib/components/ui/input/index.js';
    import { Label } from '$lib/components/ui/label/index.js';
    import { Button } from '$lib/components/ui/button/index.js';
    import { enhance } from '$app/forms';
    import { toast } from 'svelte-sonner';
  
    export let form;
  
    $: if (form?.error) {
      toast.error(form.error);
    }
  </script>
  
  <svelte:head>
    <title>Forgot Password | Indeq</title>
    <meta name="description" content="Reset your Indeq password" />
  </svelte:head>
  
  <div class="min-h-screen flex items-center justify-center">
    <div class="flex flex-col gap-4 min-w-96">
      <Card.Root class="w-full max-w-sm mx-auto">
        <Card.Header class="space-y-1">
          <Card.Title class="text-2xl">Forgot your password?</Card.Title>
          <Card.Description>
            No worries, we'll send you reset instructions.
          </Card.Description>
        </Card.Header>
        <form method="POST" use:enhance>
          <Card.Content class="grid gap-4">
            <div class="grid gap-2">
              <Label for="email">Email</Label>
              <Input id="email" name="email" type="email" placeholder="Enter your email" required />
            </div>
  
            {#if form?.error}
              <p class="text-destructive text-sm">{form.error}</p>
            {/if}
          </Card.Content>
          <Card.Footer class="flex flex-col gap-4">
            <Button type="submit" class="w-full">Reset Password</Button>
            <Card.Description class="relative text-center">
              Know your password?
              <a class="underline hover:text-primary" href="/login">Login</a>
            </Card.Description>
          </Card.Footer>
        </form>
      </Card.Root>
    </div>
  </div>
  