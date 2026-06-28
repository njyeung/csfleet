<script lang="ts">
	import { Copy, Check } from '@lucide/svelte';

	// A labelled, copyable `connect <ip>:<port>` box. ip/port may be unresolved
	// (null) while the orchestrator info is still loading, rendered as an em dash.
	let { label, ip, port }: { label: string; ip: string | null; port: number | null } = $props();

	const value = $derived(`connect ${ip ?? '—'}:${port ?? '—'}`);

	let copied = $state(false);
	let timer: ReturnType<typeof setTimeout>;
	async function copy() {
		try {
			await navigator.clipboard.writeText(value);
			copied = true;
			clearTimeout(timer);
			timer = setTimeout(() => (copied = false), 1500);
		} catch {
			// Clipboard API unavailable (e.g. insecure context) — silently no-op.
		}
	}
</script>

<div
	class="flex items-center justify-between gap-2 rounded border border-neutral-700 bg-neutral-900/60 px-3 py-2"
>
	<div class="min-w-0">
		<div class="text-[10px] font-medium uppercase tracking-wide text-neutral-500">{label}</div>
		<code class="block truncate font-mono text-sm text-neutral-200">{value}</code>
	</div>
	<button
		type="button"
		onclick={copy}
		title="Copy connect string"
		aria-label="Copy {label} connect string"
		class="shrink-0 rounded border border-neutral-600 p-1.5 text-neutral-400 transition-colors hover:bg-neutral-700/50 hover:text-neutral-200"
	>
		{#if copied}
			<Check size={15} class="text-green-400" />
		{:else}
			<Copy size={15} />
		{/if}
	</button>
</div>
