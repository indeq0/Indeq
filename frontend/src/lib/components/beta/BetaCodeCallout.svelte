<script lang="ts">
  import { ArrowRightIcon } from 'svelte-feather-icons';
  import { goto } from '$app/navigation';
</script>

<button
  type="button"
  on:click={() => goto('/beta-code')}
  class="hidden md:flex fixed top-6 right-6 z-50
	       items-center gap-2 px-4 py-2 rounded-full
	       bg-primary text-white shadow-lg cursor-pointer select-none
	       transition hover:bg-primary/90 focus-within:ring-2 focus-within:ring-primary/50"
>
  <span class="text-sm font-medium">Enter&nbsp;Beta&nbsp;Code</span>
  <ArrowRightIcon class="w-4 h-4" />
</button>
