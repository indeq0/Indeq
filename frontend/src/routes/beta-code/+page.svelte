<script lang="ts">
	import Button from '$lib/components/ui/button/button.svelte';
  import { InputOTP } from '$lib/components/ui/input-otp/index.js';
  import { enhance } from '$app/forms';
  import { goto } from '$app/navigation';
  import { toast } from 'svelte-sonner';
  export let form;

  let betaCode = '';
  let email = '';

  $: if (form?.success) {
    goto('/register');
  }

  $: if (form?.error) {
    toast.error(form.error);
  }
</script>

<form method="POST" use:enhance>
  <div class="min-h-screen w-full grid lg:grid-cols-2">
    <div class="bg-muted relative hidden h-full flex-col p-10 text-white lg:flex dark:border-r">
      <div
        class="absolute inset-0 bg-cover"
        style="background-image: url('/beta.png');"
      />
    </div>

    <div class="relative flex items-center justify-center">
      <div class="mx-auto w-full max-w-[350px] space-y-6 px-4">
        <div class="flex flex-col space-y-2 text-center">
          <h1 class="text-2xl font-semibold tracking-tight">Beta Access</h1>
          <p class="text-muted-foreground text-sm">
            Enter your beta code and email to check access
          </p>
        </div>

        <div class="space-y-4">
          <InputOTP bind:value={betaCode} length={6} autoFocus mode="alpha" />
          <div class="relative flex items-center">
            <div class="flex-grow border-t border-gray-300"></div>
            <span class="mx-4 text-gray-500 text-sm">and</span>
            <div class="flex-grow border-t border-gray-300"></div>
          </div>
          <input
            type="email"
            placeholder="m@example.com"
            bind:value={email}
            class="w-full rounded-md border px-4 py-2 text-black dark:text-white"
          />
          <Button type="submit" class="w-full">Check Access</Button>
          {#if form && 'error' in form && form.error}
            <p class="text-sm text-red-500 text-center">{form.error}</p>
          {/if}
        </div>

        <p class="text-muted-foreground px-8 text-center text-sm">
          Questions?
          <a
            href="mailto:support@indeq.app"
            class="hover:text-primary underline underline-offset-4"
          >
            Contact us
          </a>.
        </p>
      </div>
    </div>
  </div>
</form>