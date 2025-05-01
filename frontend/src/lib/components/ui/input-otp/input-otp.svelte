<script lang="ts">
	import { createEventDispatcher, onMount } from 'svelte';

	/** Number of cells in the OTP. */
	export let length: number = 6;

	/** Two‑way bound value (`bind:value`). */
	export let value: string = '';

	/** Autofocus the first cell on mount? */
	export let autoFocus: boolean = true;

	/** Disable the whole control. */
	export let disabled = false;

	/** Mode of the input. */
	export let mode: 'numeric' | 'alpha' = 'numeric';

	const dispatch = createEventDispatcher<{ change: string; complete: string }>();

	let inputs: HTMLInputElement[] = [];

	/** Focus a cell and select its content. */
	function focusCell(i: number) {
		inputs[i]?.focus();
		inputs[i]?.select();
	}

	/** Handle normal typing. */
	function handleInput(e: Event, i: number) {
		const el = e.target as HTMLInputElement;
        let chars = el.value.replace(/\s+/g, '');
        
		const isValidChar = mode === 'numeric' ? /^\d$/ : /^[a-zA-Z0-9]$/;
		if (!isValidChar.test(chars)) {
			el.value = '';
			return;
		}

        const arr = value.split('');
        for (let j = 0; j < chars.length && i + j < length; j++) {
            arr[i + j] = chars[j];
            inputs[i + j].value = chars[j];
        }
        value = arr.join('').padEnd(length, '');
        dispatch('change', value);

        const next = Math.min(i + chars.length, length - 1);
        focusCell(next);

        if (value.trim().length === length) dispatch('complete', value.trim());
	}

	/** Handle backspace / arrow nav. */
	function handleKey(e: KeyboardEvent, i: number) {
		const el = e.target as HTMLInputElement;

		if (e.key === 'Backspace') {
			if (el.value) {
				el.value = '';
				const chars = value.split('');
				chars[i] = '';
				value = chars.join('');
				dispatch('change', value);
			} else if (i > 0) {
				focusCell(i - 1);
			}
		} else if (e.key === 'ArrowLeft' && i > 0) {
			focusCell(i - 1);
		} else if (e.key === 'ArrowRight' && i < length - 1) {
			focusCell(i + 1);
		}
	}

	/** Smart paste (full code in clipboard). */
	function handlePaste(e: ClipboardEvent) {
		const txt = (e.clipboardData?.getData('text') || '').replace(/\s+/g, '');

        if (mode === 'numeric' ? /^\d+$/.test(txt) : /^[a-zA-Z0-9]+$/.test(txt)) {
            const digits = txt.slice(0, length).padEnd(length, '');
            value = digits;
            digits.split('').forEach((d, idx) => (inputs[idx].value = d));

            dispatch('change', value);
            if (digits.trim().length === length) dispatch('complete', digits.trim());

            focusCell(Math.min(digits.trim().length, length - 1));
        }

        e.preventDefault();                                   
	}

	onMount(() => {
		if (autoFocus) focusCell(0);
	});
</script>

<div
	class="flex w-full justify-between gap-2"
	role="group"
	aria-label="One-time password input"
	on:paste={handlePaste}
>
	{#each Array(length) as _, i}
		<input
			bind:this={inputs[i]}
			class="w-12 h-12 text-center text-lg border rounded focus:outline-none
			focus:ring focus:ring-primary disabled:opacity-50"
			type="text"
			inputmode={mode === 'numeric' ? 'numeric' : 'text'}
			pattern={mode === 'numeric' ? '\\d*' : '[a-zA-Z0-9]*'}
			maxlength="1"
			disabled={disabled}
			value={value[i] ?? ''}
			aria-label={`Digit ${i + 1} of ${length}`}
			on:input={(e) => handleInput(e, i)}
			on:keydown={(e) => handleKey(e, i)}
		/>
	{/each}
</div>
