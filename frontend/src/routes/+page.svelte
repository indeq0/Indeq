<script lang="ts">
  import { BoxIcon, MailIcon, LoaderIcon } from 'svelte-feather-icons';
  import { onMount } from 'svelte';
  import { toast } from 'svelte-sonner';

  let email = '';
  let submitStatus: 'idle' | 'loading' | 'success' | 'error' = 'idle';

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    submitStatus = 'loading';

    try {
      const response = await fetch('/api/waitlist', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({ email })
      });

      const result = await response.json();

      if (!response.ok || !result.success) {
        throw new Error(result.message || 'Submission failed');
      }

      submitStatus = 'success';
      email = '';
      toast.success(result.message || 'Successfully added to the waitlist! 🎉');
    } catch (error) {
      submitStatus = 'error';
      let errorMessage = 'Failed to submit email. Please try again';
      if (error instanceof Error) {
        errorMessage = error.message;
      }
      toast.error(errorMessage);
    }
  }

  // Blob positions and sizes
  let blobs = [
    { x: 30, y: 50, size: 200, color: '#a78bfa' }, // Purple
    { x: 50, y: 20, size: 250, color: '#93c5fd' }, // Blue
    { x: 70, y: 60, size: 300, color: '#fbcfe8' } // Pink
  ];

  // Animate blobs
  onMount(() => {
    const animate = () => {
      blobs = blobs.map((blob) => ({
        ...blob,
        x: blob.x + (Math.random() - 0.5) * 0.5, // Random movement
        y: blob.y + (Math.random() - 0.5) * 0.5
      }));
      requestAnimationFrame(animate);
    };
    animate();
  });
</script>

<main
  class="min-h-screen flex flex-col items-center justify-center bg-gradient-to-br from-white to-gray-50 p-6 relative overflow-hidden"
>
  <!-- Blurred Gradient Animation -->
  {#each blobs as blob}
    <div
      class="absolute rounded-full opacity-50 blur-3xl"
      style={`width: ${blob.size}px; height: ${blob.size}px; background: ${blob.color}; left: ${blob.x}%; top: ${blob.y}%;`}
    ></div>
  {/each}

  <div class="w-full max-w-md text-center relative z-10">
    <!-- App Name -->
    <h1 class="text-4xl text-gray-900 mb-3">Indeq</h1>
    <p class="text-gray-600 text-lg">Find What Matters, Across Every Platform.</p>

    <!-- Email Signup Form -->
    <form class="flex items-center gap-3 mt-8" on:submit={handleSubmit}>
      <div class="relative flex-1">
        <MailIcon
          size="20"
          class="absolute left-3 top-1/2 transform -translate-y-1/2 text-gray-400"
        />
        <input
          bind:value={email}
          placeholder="Enter your email..."
          class="w-full pl-10 p-2 bg-white border border-gray-200 rounded-lg text-gray-900 focus:outline-none focus:ring-2 focus:ring-primary placeholder-gray-400 disabled:bg-gray-100 disabled:text-gray-500"
          disabled={submitStatus === 'loading'}
        />
      </div>
      <button
        type="submit"
        class="p-2 rounded-lg bg-primary text-white hover:bg-blue-600 transition-colors flex items-center justify-center min-w-[90px]"
        disabled={submitStatus === 'loading'}
      >
        {#if submitStatus === 'loading'}
          <div class="animate-spin">
            <LoaderIcon size="18" />
          </div>
        {:else}
          Notify Me
        {/if}
      </button>
    </form>

    <!-- Coming Soon Message -->
    <div class="flex justify-center mt-6 px-2">
      <span class="inline-flex items-center text-center">
        <BoxIcon
          size="18"
          class="hidden sm:block text-primary transition-transform duration-300 hover:scale-125 hover:rotate-12 flex-shrink-0"
        />
        <p class="text-lg font-medium text-gray-800 sm:ml-2">Get notified the moment we launch!</p>
      </span>
    </div>

    <hr class="border-t border-gray-300 my-8" />

    <div>
      <p class="text-lg pt-1 font-medium text-gray-600 mb-3">Built by engineers from</p>
      <div class="flex items-center justify-center gap-6 flex-wrap">
        <img
          src="/meta.svg"
          alt="Meta"
          class="h-8 w-8 opacity-50 hover:opacity-100 transition-opacity"
        />
        <img
          src="/roblox.svg"
          alt="Roblox"
          class="h-8 w-16 opacity-50 hover:opacity-100 transition-opacity"
        />
        <img
          src="/google.svg"
          alt="Google"
          class="h-8 w-8 opacity-50 hover:opacity-100 transition-opacity"
        />
        <img
          src="/coinbase.svg"
          alt="Coinbase"
          class="h-8 w-16 opacity-50 hover:opacity-100 transition-opacity"
        />
        <img
          src="/oracle.svg"
          alt="Oracle"
          class="h-6 w-9 opacity-50 hover:opacity-100 transition-opacity"
        />
        <img
          src="/aws.svg"
          alt="Amazon"
          class="mt-1 h-8 w-8 opacity-50 hover:opacity-100 transition-opacity"
        />
        <img
          src="/atlassian.svg"
          alt="Atlassian"
          class="-mt-2 h-8 w-8 opacity-50 hover:opacity-100 transition-opacity"
        />
      </div>
    </div>
  </div>
</main>

<style>
  /* Smooth transitions for blobs */
  .absolute {
    transition: all 0.5s ease-out;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .animate-spin {
    animation: spin 1s linear infinite;
  }
</style>
