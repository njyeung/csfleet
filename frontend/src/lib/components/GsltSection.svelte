<script lang="ts">
	import { Trash2, Plus } from '@lucide/svelte';
	import { gslt } from '$lib/api/services';

	// GSLT token pool. Tokens are claimed by servers with auto_token on.
	// Add/delete are immediate (no staged Save); the parent reloads the list.
	let { tokens, onchanged }: { tokens: string[]; onchanged: () => void } = $props();

	let draft = $state('');
	let busy = $state(false);
	let error = $state<string | null>(null);

	const trimmed = $derived(draft.trim());

	async function add() {
		if (!trimmed || busy) return;
		busy = true;
		error = null;
		const res = await gslt.add(trimmed);
		busy = false;
		if (res.ok) {
			draft = '';
			onchanged();
		} else {
			error = `Add failed (HTTP ${res.status})`;
		}
	}

	async function remove(token: string) {
		busy = true;
		error = null;
		const res = await gslt.remove(token);
		busy = false;
		if (res.ok) onchanged();
		else error = `Delete failed (HTTP ${res.status})`;
	}
</script>

<section class="rounded-lg border border-neutral-700">
	<header class="border-b border-neutral-700 px-4 py-3">
		<h2 class="text-sm font-semibold text-neutral-100">GSLT token pool</h2>
		<p class="mt-0.5 text-xs text-neutral-500">
			Game Server Login Tokens. A server with auto-token on claims a free one from this pool at start.
		</p>
	</header>

	<div class="space-y-2 px-4 py-3">
		{#if tokens.length === 0}
			<p class="py-1 text-sm text-neutral-500">Pool is empty.</p>
		{:else}
			<ul class="space-y-1">
				{#each tokens as token (token)}
					<li class="flex items-center gap-2">
						<span class="flex-1 truncate rounded border border-neutral-800 bg-neutral-900 px-2 py-1 font-mono text-sm text-neutral-300">
							{token}
						</span>
						<button
							type="button"
							title="Delete token"
							aria-label="Delete token"
							disabled={busy}
							onclick={() => remove(token)}
							class="rounded p-1.5 text-neutral-500 hover:bg-red-500/10 hover:text-red-400 disabled:opacity-40"
						>
							<Trash2 size={15} />
						</button>
					</li>
				{/each}
			</ul>
		{/if}

		<div class="flex items-center gap-2 pt-1">
			<input
				type="text"
				bind:value={draft}
				placeholder="add a token…"
				spellcheck="false"
				autocomplete="off"
				onkeydown={(e) => e.key === 'Enter' && add()}
				class="flex-1 rounded border border-neutral-700 bg-neutral-900 px-2 py-1 font-mono text-sm text-neutral-200 outline-none focus:border-neutral-500"
			/>
			<button
				type="button"
				disabled={!trimmed || busy}
				onclick={add}
				class="flex items-center gap-1.5 rounded border border-neutral-600 px-3 py-1 text-sm text-neutral-200 hover:bg-neutral-700/50 disabled:opacity-40"
			>
				<Plus size={15} /> Add
			</button>
		</div>
		{#if error}
			<p class="text-xs text-red-400">{error}</p>
		{/if}
	</div>
</section>
