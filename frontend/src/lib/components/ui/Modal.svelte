<script lang="ts">
	import type { Snippet } from 'svelte';

	// Generic modal shell: a dimmed backdrop + a neutral panel. The parent owns
	// `open` and renders the body via the `children` snippet; `onclose` fires on
	// Escape or backdrop click (suppressed while `busy`, e.g. mid-submit).
	let {
		open = false,
		title,
		busy = false,
		size = 'md',
		onclose,
		children
	}: {
		open?: boolean;
		title: string;
		busy?: boolean;
		size?: 'md' | 'lg';
		onclose: () => void;
		children: Snippet;
	} = $props();

	const maxW = $derived(size === 'lg' ? 'max-w-2xl' : 'max-w-lg');
</script>

<svelte:window
	onkeydown={(e) => {
		if (open && !busy && e.key === 'Escape') onclose();
	}}
/>

{#if open}
	<div class="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto p-4 sm:p-8">
		<button
			type="button"
			aria-label="Close"
			class="fixed inset-0 bg-black/60"
			onclick={() => !busy && onclose()}
		></button>
		<div
			role="dialog"
			aria-modal="true"
			aria-label={title}
			class="relative my-auto w-full {maxW} rounded-lg border border-neutral-700 bg-neutral-800 shadow-xl"
		>
			<header class="flex items-center justify-between border-b border-neutral-700 px-5 py-3">
				<h2 class="text-sm font-semibold text-neutral-100">{title}</h2>
				<button
					type="button"
					aria-label="Close"
					disabled={busy}
					onclick={onclose}
					class="rounded p-1 text-neutral-500 hover:bg-neutral-700/50 hover:text-neutral-200 disabled:opacity-40"
				>
					✕
				</button>
			</header>
			<div class="px-5 py-4">
				{@render children()}
			</div>
		</div>
	</div>
{/if}
