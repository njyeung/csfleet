<script lang="ts">
	// Tri-state plugin/config picker for the New forms.
	//   value === null  → Inherit (servers only): field omitted on the wire.
	//   value is string[] → Explicit: [] = run none, [..] = these. (Always for clusters.)
	// Clusters don't inherit, so `allowInherit` is false there and the Inherit
	// toggle + parent preview are hidden.
	let {
		label,
		catalog,
		value = $bindable(),
		allowInherit = false,
		inheritPreview = null,
		previewLoading = false
	}: {
		label: string;
		catalog: string[];
		value: string[] | null;
		allowInherit?: boolean;
		// Resolved parent set shown read-only while inheriting (cluster set for a
		// member, else the global set).
		inheritPreview?: string[] | null;
		previewLoading?: boolean;
	} = $props();

	const inheriting = $derived(allowInherit && value === null);

	function setExplicit(on: boolean) {
		// Inherit → Explicit starts from an empty "run none"; Explicit → Inherit drops it.
		value = on ? [] : null;
	}

	function toggleItem(name: string, checked: boolean) {
		const cur = value ?? [];
		value = checked ? [...cur, name] : cur.filter((n) => n !== name);
	}

	const selected = $derived(new Set(value ?? []));
</script>

<div class="space-y-2">
	<div class="flex items-center justify-between gap-4">
		<span class="text-sm font-medium text-neutral-300">{label}</span>
		{#if allowInherit}
			<div class="flex items-center gap-2 text-xs">
				<button
					type="button"
					onclick={() => setExplicit(false)}
					class={[
						'rounded px-2 py-0.5',
						inheriting ? 'bg-neutral-700 text-neutral-100' : 'text-neutral-500 hover:bg-neutral-700/50'
					]}
				>
					Inherit
				</button>
				<button
					type="button"
					onclick={() => setExplicit(true)}
					class={[
						'rounded px-2 py-0.5',
						!inheriting ? 'bg-neutral-700 text-neutral-100' : 'text-neutral-500 hover:bg-neutral-700/50'
					]}
				>
					Explicit
				</button>
			</div>
		{/if}
	</div>

	{#if inheriting}
		<div class="rounded border border-neutral-700 bg-neutral-900/50 p-2 text-xs">
			{#if previewLoading}
				<p class="text-neutral-500">Loading inherited set…</p>
			{:else if inheritPreview && inheritPreview.length > 0}
				<p class="mb-1 text-neutral-500">Inherits from parent:</p>
				<ul class="flex flex-wrap gap-1">
					{#each inheritPreview as name (name)}
						<li class="rounded bg-neutral-700/60 px-1.5 py-0.5 text-neutral-300">{name}</li>
					{/each}
				</ul>
			{:else}
				<p class="text-neutral-500">Inherits an empty set (runs none).</p>
			{/if}
		</div>
	{:else}
		<div class="max-h-40 overflow-y-auto rounded border border-neutral-700 bg-neutral-900/50 p-2">
			{#if catalog.length === 0}
				<p class="text-xs text-neutral-500">Catalog is empty.</p>
			{:else}
				<ul class="space-y-0.5">
					{#each catalog as name (name)}
						<li>
							<label class="flex items-center gap-2 rounded px-1 py-0.5 text-sm text-neutral-300 hover:bg-neutral-700/40">
								<input
									type="checkbox"
									checked={selected.has(name)}
									onchange={(e) => toggleItem(name, e.currentTarget.checked)}
									class="h-3.5 w-3.5 rounded border-neutral-600 bg-neutral-900 text-neutral-400 focus:ring-0 focus:ring-offset-0"
								/>
								<span class="truncate">{name}</span>
							</label>
						</li>
					{/each}
				</ul>
			{/if}
			<p class="mt-1 px-1 text-xs text-neutral-600">
				{selected.size === 0 ? 'None selected — runs none.' : `${selected.size} selected.`}
			</p>
		</div>
	{/if}
</div>
