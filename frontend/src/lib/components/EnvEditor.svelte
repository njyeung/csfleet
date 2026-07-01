<script lang="ts">
	import type { EnvVar } from '$lib/api/types';
	import { CS2_CATALOG, isReserved } from '$lib/cs2/catalog';
	import { isValidEnvKey } from '$lib/validation';

	// Create-only env overlay editor. Two regions:
	//   • Inherited (read-only) — what the instance resolves from its parents
	//     (global, plus the cluster tier for a member), each badged with its scope.
	//     "Override" copies a key into the editable list to shadow it.
	//   • This instance — the editable overlay that becomes the `env` create field.
	// Reserved vars are silently dropped on the wire.
	let {
		value = $bindable({}),
		inherited = [],
		inheritedLoading = false,
		serverName = ''
	}: {
		value?: Record<string, string>;
		inherited?: EnvVar[];
		inheritedLoading?: boolean;
		serverName?: string;
	} = $props();

	type Row = { id: number; key: string; value: string };
	let nextId = 0;
	let rows = $state<Row[]>([]);
	let catalogPick = $state('');

	// Rebuild the wire record from the rows: trim keys, drop blanks, drop reserved
	// (a no-op), last value wins on a duplicate key.
	function sync() {
		const out: Record<string, string> = {};
		for (const r of rows) {
			const k = r.key.trim();
			if (k === '' || isReserved(k)) continue;
			out[k] = r.value;
		}
		value = out;
	}

	function addRow(key = '', val = '') {
		rows.push({ id: nextId++, key, value: val });
		sync();
	}
	function removeRow(id: number) {
		rows = rows.filter((r) => r.id !== id);
		sync();
	}
	function override(v: EnvVar) {
		// Don't stack duplicate override rows for the same key.
		if (!rows.some((r) => r.key.trim() === v.key)) addRow(v.key, v.value);
	}
	function addFromCatalog(key: string) {
		catalogPick = '';
		if (key === '') return;
		// CS2_SERVERNAME defaults to the instance name.
		const def =
			key === 'CS2_SERVERNAME'
				? serverName
				: (CS2_CATALOG.flatMap((g) => g.vars).find((c) => c.key === key)?.default ?? '');
		if (!rows.some((r) => r.key.trim() === key)) addRow(key, def);
	}

	const SCOPE_BADGE: Record<string, string> = {
		global: 'bg-neutral-700 text-neutral-300',
		cluster: 'bg-neutral-700 text-neutral-300',
		server: 'bg-neutral-700 text-neutral-300'
	};

	const overrideKeys = $derived(new Set(rows.map((r) => r.key.trim())));
</script>

<div class="space-y-3">
	<!-- Inherited (read-only) -->
	<div>
		<h4 class="mb-1 text-xs font-semibold uppercase tracking-wide text-neutral-500">Inherited</h4>
		<div class="rounded border border-neutral-700 bg-neutral-900/50 p-2">
			{#if inheritedLoading}
				<p class="text-xs text-neutral-500">Loading…</p>
			{:else if inherited.length === 0}
				<p class="text-xs text-neutral-500">Nothing inherited.</p>
			{:else}
				<ul class="space-y-0.5">
					{#each inherited as v (v.scope + ':' + v.key)}
						<li class="flex items-center gap-2 px-1 py-0.5 text-xs">
							<span class="rounded px-1.5 py-0.5 text-[10px] uppercase {SCOPE_BADGE[v.scope] ?? SCOPE_BADGE.global}">
								{v.scope}
							</span>
							<span class="font-mono text-neutral-300">{v.key}</span>
							<span class="truncate font-mono text-neutral-500">{v.value}</span>
							<button
								type="button"
								onclick={() => override(v)}
								disabled={overrideKeys.has(v.key)}
								class="ml-auto shrink-0 rounded border border-neutral-600 px-1.5 py-0.5 text-neutral-400 hover:bg-neutral-700/50 disabled:opacity-40"
							>
								{overrideKeys.has(v.key) ? 'overriding' : 'Override'}
							</button>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	</div>

	<!-- This instance (editable overlay) -->
	<div>
		<h4 class="mb-1 text-xs font-semibold uppercase tracking-wide text-neutral-500">This instance</h4>
		<div class="space-y-1.5">
			{#each rows as row (row.id)}
				{@const collides = inherited.some((v) => v.key === row.key.trim())}
				{@const keyInvalid = row.key.trim() !== '' && !isValidEnvKey(row.key.trim())}
				<div class="flex items-center gap-1.5">
					<input
						type="text"
						bind:value={row.key}
						oninput={sync}
						placeholder="KEY"
						spellcheck="false"
						class={[
							'w-44 rounded border bg-neutral-900 px-2 py-1 font-mono text-sm text-neutral-200 outline-none',
							keyInvalid ? 'border-red-500/60' : 'border-neutral-700 focus:border-neutral-500'
						]}
					/>
					<input
						type="text"
						bind:value={row.value}
						oninput={sync}
						placeholder="value"
						spellcheck="false"
						class="min-w-0 flex-1 rounded border border-neutral-700 bg-neutral-900 px-2 py-1 font-mono text-sm text-neutral-200 outline-none focus:border-neutral-500"
					/>
					{#if isReserved(row.key)}
						<span class="shrink-0 text-xs text-amber-400" title="Reserved by the orchestrator — ignored">reserved</span>
					{:else if keyInvalid}
						<span class="shrink-0 text-xs text-red-400" title="Keys can't contain spaces or '='">invalid</span>
					{:else if collides}
						<span class="shrink-0 text-[10px] uppercase text-neutral-500">overrides</span>
					{/if}
					<button
						type="button"
						aria-label="Remove variable"
						onclick={() => removeRow(row.id)}
						class="shrink-0 rounded border border-neutral-700 px-2 py-1 text-neutral-500 hover:bg-neutral-700/50 hover:text-neutral-300"
					>
						✕
					</button>
				</div>
			{/each}

			<div class="flex items-center gap-2 pt-1">
				<button
					type="button"
					onclick={() => addRow()}
					class="rounded border border-neutral-600 px-2 py-1 text-xs text-neutral-300 hover:bg-neutral-700/50"
				>
					+ Add variable
				</button>
				<select
					bind:value={catalogPick}
					onchange={(e) => addFromCatalog(e.currentTarget.value)}
					class="rounded border border-neutral-700 bg-neutral-900 px-2 py-1 text-xs text-neutral-300 outline-none focus:border-neutral-500"
				>
					<option value="">+ Add from CS2 catalog…</option>
					{#each CS2_CATALOG as group (group.label)}
						<optgroup label={group.label}>
							{#each group.vars as v (v.key)}
								<option value={v.key} title={v.description}>{v.key}</option>
							{/each}
						</optgroup>
					{/each}
				</select>
			</div>
		</div>
	</div>
</div>
