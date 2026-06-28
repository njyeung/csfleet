<script lang="ts">
	// Controlled confirm modal. The parent owns `open` and the pending action;
	// `busy` disables the buttons while the action runs (e.g. a delete fan-out).
	let {
		open = false,
		title,
		message,
		confirmLabel = 'Confirm',
		tone = 'red',
		busy = false,
		onconfirm,
		oncancel
	}: {
		open?: boolean;
		title: string;
		message: string;
		confirmLabel?: string;
		tone?: 'red' | 'amber' | 'neutral';
		busy?: boolean;
		onconfirm: () => void;
		oncancel: () => void;
	} = $props();

	const confirmClass = $derived(
		tone === 'red'
			? 'border-red-500/40 text-red-300 hover:bg-red-500/10'
			: tone === 'amber'
				? 'border-amber-500/40 text-amber-300 hover:bg-amber-500/10'
				: 'border-neutral-600 text-neutral-200 hover:bg-neutral-700/50'
	);
</script>

<svelte:window
	onkeydown={(e) => {
		if (open && !busy && e.key === 'Escape') oncancel();
	}}
/>

{#if open}
	<div class="fixed inset-0 z-50 flex items-center justify-center p-4">
		<button
			type="button"
			aria-label="Cancel"
			class="absolute inset-0 bg-black/60"
			onclick={() => !busy && oncancel()}
		></button>
		<div
			role="dialog"
			aria-modal="true"
			aria-label={title}
			class="relative w-full max-w-sm rounded-lg border border-neutral-700 bg-neutral-800 p-5 shadow-xl"
		>
			<h2 class="text-sm font-semibold text-neutral-100">{title}</h2>
			<p class="mt-2 text-sm text-neutral-400">{message}</p>
			<div class="mt-5 flex justify-end gap-2">
				<button
					type="button"
					disabled={busy}
					onclick={oncancel}
					class="rounded border border-neutral-600 px-3 py-1 text-sm text-neutral-300 hover:bg-neutral-700/50 disabled:opacity-50"
				>
					Cancel
				</button>
				<button
					type="button"
					disabled={busy}
					onclick={onconfirm}
					class={['rounded border px-3 py-1 text-sm disabled:opacity-50', confirmClass]}
				>
					{busy ? 'Working…' : confirmLabel}
				</button>
			</div>
		</div>
	</div>
{/if}
