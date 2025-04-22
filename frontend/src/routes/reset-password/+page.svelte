<script lang="ts">
  import * as Card from '$lib/components/ui/card/index.js';
  import { Input } from '$lib/components/ui/input/index.js';
  import { Label } from '$lib/components/ui/label/index.js';
  import { Button } from '$lib/components/ui/button/index.js';
  import { enhance } from '$app/forms';
  import { toast } from 'svelte-sonner';
  import { goto } from '$app/navigation';

  export let form;
  let submitting = false;

  function handleEnhance() {
    return async ({ result, update }: { result: any; update: any }) => {
      submitting = false;

      if (result.type === 'success') {
        toast.success('Password successfully reset! Please log in with your new password.');
        goto('/login');
        return;
      }

      // Always update the form unless we've redirected
      await update();
    };
  }

  $: if (form?.error) {
    toast.error(form.error);
  }
</script>

<svelte:head>
  <title>Reset Password | Indeq</title>
  <meta name="description" content="Choose a new password" />
</svelte:head>

<div class="min-h-screen flex items-center justify-center">
  <div class="flex flex-col gap-4 min-w-96">
    <Card.Root class="w-full max-w-sm mx-auto">
      <Card.Header class="space-y-1">
        <Card.Title class="text-2xl">Reset your password</Card.Title>
        <Card.Description>Enter your new password to finish the reset process.</Card.Description>
      </Card.Header>
      <form method="POST" use:enhance={handleEnhance}>
        <Card.Content class="grid gap-4">
          <div class="grid gap-2">
            <Label for="password">New Password</Label>
            <Input id="password" name="password" type="password" required />
          </div>
        </Card.Content>
        <Card.Footer class="flex flex-col gap-4">
          <Button type="submit" class="w-full">Save Password</Button>
        </Card.Footer>
      </form>
    </Card.Root>
  </div>
</div>
