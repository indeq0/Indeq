<script lang="ts">
  import * as Card from '$lib/components/ui/card/index.js';
  import { InputOTP } from '$lib/components/ui/input-otp/index.js';
  import { Button } from '$lib/components/ui/button/index.js';
  import { Label } from '$lib/components/ui/label/index.js';
  import { enhance } from '$app/forms';
  import { toast } from 'svelte-sonner';
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { fetchAndStoreUserData } from '$lib/utils/user';

  export let data: { context: 'register' | 'forgot'; expired?: boolean };
  export let form:
    | { expired: true; context: 'register' | 'forgot' }
    | { message?: string; success?: boolean; error?: string; verifiedType?: string; user?: any }
    | undefined;

  let otp = '';
  $: hiddenCode = otp;

  let countdown = 0;
  let countdownInterval: NodeJS.Timeout;
  function startCountdown() {
    countdown = 30;
    countdownInterval = setInterval(() => {
      countdown--;
      if (countdown <= 0) clearInterval(countdownInterval);
    }, 1000);
  }
  onMount(() => () => countdownInterval && clearInterval(countdownInterval));

  onMount(() => {
    if (data.expired) {
      toast.error('Your verification session expired. Please start again.');
      const target = data.context === 'register' ? '/register' : '/login';
      setTimeout(() => goto(target), 10);
    }
  });

  $: if (form && 'expired' in form && form.expired) {
    toast.error('Your verification session expired. Please start again.');
    const target = form.context === 'register' ? '/register' : '/login';
    setTimeout(() => goto(target), 10);
  }

  $: if (form && 'message' in form && form.message) {
    toast.success(form.message);
    startCountdown();
  }

  $: if (form && 'success' in form && form.success && 'verifiedType' in form) {
    if (form.verifiedType === 'register') {
      fetchAndStoreUserData();
      toast.success('Welcome aboard! 🎉');
      goto('/chat');
    } else {
      goto('/reset-password');
    }
  }

  $: if (form && 'error' in form && form.error) {
    toast.error(form.error);
  }
</script>


<svelte:head>
  <title>Enter Code | Indeq</title>
  <meta name="description" content="Enter the verification code sent to your email" />
</svelte:head>

<div class="min-h-screen flex items-center justify-center">
  <div class="flex flex-col gap-4 min-w-96">
    <Card.Root class="w-full max-w-sm mx-auto">
      <Card.Header class="space-y-1">
        <Card.Title class="text-2xl">Enter your code</Card.Title>
        <Card.Description>
          We've sent a 6‑digit code to your email. Enter it below to
          {data.context === 'register' ? ' complete your registration.' : ' reset your password.'}
        </Card.Description>
      </Card.Header>

      <form method="POST" use:enhance>
        <Card.Content class="grid gap-4">
          <input type="hidden" name="type" value={data.context} />

          <div class="grid gap-2">
            <Label for="otp">Verification Code</Label>

            <input id="otp" name="code" type="hidden" value={hiddenCode} />

            <InputOTP
              bind:value={otp}
              length={6}
              autoFocus
            />
          </div>

          {#if form && 'error' in form && form.error}
            <p class="text-destructive text-sm">{form.error}</p>
          {/if}
        </Card.Content>

        <Card.Footer class="flex flex-col gap-4 pb-2">
          <Button type="submit" class="w-full">Submit Code</Button>
        </Card.Footer>
      </form>

      <!-- Resend form is separate to avoid nested forms -->
      <form method="POST" use:enhance class="px-6 pt-0 pb-6 flex flex-col gap-4">
        <input type="hidden" name="resend" value="true" />
        <input type="hidden" name="type" value={data.context} />
        <Button type="submit" variant="outline" class="w-full" disabled={countdown > 0}>
          {#if countdown > 0}
            Resend Code ({countdown}s)
          {:else}
            Resend Code
          {/if}
        </Button>
        <Card.Description class="text-center text-sm text-muted-foreground">
          Didn't receive it? Check your spam folder.
        </Card.Description>
      </form>
    </Card.Root>
  </div>
</div>
