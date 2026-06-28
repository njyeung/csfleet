<script lang="ts">
	import type { AssignmentSet } from '$lib/api/types';

	// Read-only renderer for a plugin/config assignment set (GET .../plugins|configs).
	// `overridden` means this scope defines the set; otherwise it's inherited from a
	// parent (cluster for a member, else global). These are create-only, hence read-only.
	let {
		title,
		set,
		loading = false
	}: { title: string; set: AssignmentSet | null; loading?: boolean } = $props();

	// `items` is null for an inherited/empty set (nil slice over the wire) — treat
	// it the same as []. Without this, set.items.length throws and takes the panel down.
	const items = $derived(set?.items ?? []);
</script>

<section>
	<h3 class="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-neutral-500">
		{title}
		{#if set}
			<span class="rounded bg-neutral-700 px-1.5 py-0.5 text-[10px] font-normal normal-case text-neutral-300">
				{set.overridden ? 'overridden' : 'inherited'}
			</span>
		{/if}
	</h3>

	{#if loading}
		<p class="text-sm text-neutral-500">Loading…</p>
	{:else if !set}
		<p class="text-sm text-neutral-500">—</p>
	{:else if items.length === 0}
		<p class="text-sm text-neutral-500">none</p>
	{:else}
		<ul class="flex flex-wrap gap-1.5">
			{#each items as item (item)}
				<li class="rounded border border-neutral-700 bg-neutral-900 px-2 py-0.5 font-mono text-xs text-neutral-300">
					{item}
				</li>
			{/each}
		</ul>
	{/if}
</section>
