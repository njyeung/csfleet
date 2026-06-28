<script lang="ts">
	import { untrack } from 'svelte';
	import { SvelteMap, SvelteSet } from 'svelte/reactivity';
	import { Eye, EyeOff, Trash2, Plus } from '@lucide/svelte';
	import { env } from '$lib/api/services';
	import type { EnvVar } from '$lib/api/types';
	import { isValidEnvKey } from '$lib/validation';

	// Global env CRUD — the only editable env tier. Edits are staged and
	// committed by Save: the final key→value map is diffed against the loaded
	// baseline, issuing a PUT per added/changed key and a DELETE per removed key.
	let { initial }: { initial: EnvVar[] } = $props();

	// db.* env is seeded from the orchestrator's .env on first boot. The API would
	// technically accept an overwrite (INSERT IGNORE seeds once; PUT is unguarded),
	// but changing it is unsupported and breaks the control-plane DB connection, so
	// the editor locks these rows display-only and the save diff skips them.
	const MANAGED_KEYS = new Set(['db.host', 'db.port', 'db.name', 'db.user', 'db.pass', 'db.rootpass']);
	const isManaged = (key: string) => MANAGED_KEYS.has(key.trim());

	// Credentials are masked with reveal-on-click.
	const SECRET_KEYS = new Set(['db.pass', 'db.rootpass']);
	const isSecret = (key: string) => SECRET_KEYS.has(key.trim());

	type Row = { id: number; key: string; value: string };
	let nextId = 0;
	const toRows = (vars: EnvVar[]): Row[] =>
		vars
			.slice()
			.sort((a, b) => a.key.localeCompare(b.key))
			.map((v) => ({ id: nextId++, key: v.key, value: v.value }));

	let rows = $state<Row[]>(untrack(() => toRows(initial)));
	// key→value as last loaded/saved; Save diffs against this.
	const baseline = new SvelteMap<string, string>(untrack(() => initial.map((v) => [v.key, v.value])));
	const revealed = new SvelteSet<number>();

	let saving = $state(false);
	let error = $state<string | null>(null);
	let saved = $state(false);

	// Final committed map: trimmed non-empty keys, last one wins on collision.
	const liveMap = $derived.by(() => {
		const m = new Map<string, string>();
		for (const r of rows) {
			const k = r.key.trim();
			if (k) m.set(k, r.value);
		}
		return m;
	});

	const dirty = $derived.by(() => {
		if (liveMap.size !== baseline.size) return true;
		for (const [k, v] of liveMap) if (!baseline.has(k) || baseline.get(k) !== v) return true;
		return false;
	});

	// A non-empty user key with a space or '=' blocks Save (managed db.* rows are
	// locked and exempt). Friendly hint, mirrors the EnvEditor overlay check.
	const anyInvalid = $derived(
		rows.some((r) => !isManaged(r.key) && r.key.trim() !== '' && !isValidEnvKey(r.key.trim()))
	);

	function addRow() {
		rows = [...rows, { id: nextId++, key: '', value: '' }];
	}
	function removeRow(id: number) {
		rows = rows.filter((r) => r.id !== id);
	}
	function toggleReveal(id: number) {
		if (revealed.has(id)) revealed.delete(id);
		else revealed.add(id);
	}

	async function save() {
		if (!dirty || saving || anyInvalid) return;
		saving = true;
		error = null;
		saved = false;

		const live = liveMap;
		// Upserts (added or changed). Managed db.* rows are never written.
		for (const [key, value] of live) {
			if (isManaged(key) || baseline.get(key) === value) continue;
			const res = await env.set({ key, value, scope: 'global', scope_name: '' });
			if (!res.ok) {
				error = `Saving "${key}" failed (HTTP ${res.status})`;
				saving = false;
				return;
			}
		}
		// Deletions (in baseline, gone from live). Managed db.* rows are never removed.
		for (const key of baseline.keys()) {
			if (live.has(key) || isManaged(key)) continue;
			const res = await env.remove(key, 'global', '');
			if (!res.ok) {
				error = `Deleting "${key}" failed (HTTP ${res.status})`;
				saving = false;
				return;
			}
		}

		baseline.clear();
		for (const [k, v] of live) baseline.set(k, v);
		saving = false;
		saved = true;
	}

	function reset() {
		// Rebuild rows from the current baseline (advances on each successful save).
		rows = [...baseline].map(([key, value]) => ({ id: nextId++, key, value }));
		revealed.clear();
		error = null;
		saved = false;
	}
</script>

<section class="rounded-lg border border-neutral-700">
	<header class="flex items-center justify-between border-b border-neutral-700 px-4 py-3">
		<div>
			<h2 class="text-sm font-semibold text-neutral-100">Global environment</h2>
			<p class="mt-0.5 text-xs text-neutral-500">
				Base env for every server (global ◂ cluster ◂ server). Applied at container start.
			</p>
		</div>
	</header>

	<div class="space-y-2 px-4 py-3">
		{#if rows.length === 0}
			<p class="py-2 text-sm text-neutral-500">No global variables.</p>
		{/if}
		{#each rows as row (row.id)}
			{@const secret = isSecret(row.key)}
			{@const managed = isManaged(row.key)}
			{@const keyInvalid = !managed && row.key.trim() !== '' && !isValidEnvKey(row.key.trim())}
			<div class="flex items-center gap-2">
				<input
					type="text"
					bind:value={row.key}
					disabled={managed}
					placeholder="KEY"
					spellcheck="false"
					class={[
						'w-56 rounded border bg-neutral-900 px-2 py-1 font-mono text-sm text-neutral-200 outline-none disabled:text-neutral-400 disabled:opacity-70',
						keyInvalid ? 'border-red-500/60' : 'border-neutral-700 focus:border-neutral-500'
					]}
				/>
				<input
					type={secret && !revealed.has(row.id) ? 'password' : 'text'}
					bind:value={row.value}
					disabled={managed}
					placeholder="value"
					spellcheck="false"
					autocomplete="off"
					class="flex-1 rounded border border-neutral-700 bg-neutral-900 px-2 py-1 font-mono text-sm text-neutral-200 outline-none focus:border-neutral-500 disabled:text-neutral-400 disabled:opacity-70"
				/>
				{#if keyInvalid}
					<span class="shrink-0 text-xs text-red-400" title="Keys can't contain spaces or '='">invalid</span>
				{/if}
				{#if managed}
					<span
						title="Seeded from the orchestrator's .env — managed by CSFleet"
						class="shrink-0 rounded border border-neutral-700 px-1.5 py-0.5 text-xs text-neutral-500"
					>
						managed
					</span>
				{/if}
				{#if secret}
					<button
						type="button"
						title={revealed.has(row.id) ? 'Hide' : 'Reveal'}
						aria-label={revealed.has(row.id) ? 'Hide value' : 'Reveal value'}
						onclick={() => toggleReveal(row.id)}
						class="rounded p-1.5 text-neutral-500 hover:bg-neutral-700/50 hover:text-neutral-200"
					>
						{#if revealed.has(row.id)}<EyeOff size={15} />{:else}<Eye size={15} />{/if}
					</button>
				{/if}
				{#if !managed}
					<button
						type="button"
						title="Remove"
						aria-label="Remove {row.key}"
						onclick={() => removeRow(row.id)}
						class="rounded p-1.5 text-neutral-500 hover:bg-red-500/10 hover:text-red-400"
					>
						<Trash2 size={15} />
					</button>
				{/if}
			</div>
		{/each}

		<button
			type="button"
			onclick={addRow}
			class="flex items-center gap-1.5 rounded border border-neutral-700 px-2 py-1 text-xs text-neutral-300 hover:bg-neutral-700/50"
		>
			<Plus size={13} /> Add variable
		</button>
	</div>

	<footer class="flex items-center gap-2 border-t border-neutral-700 px-4 py-3">
		{#if error}
			<span class="mr-auto text-xs text-red-400">{error}</span>
		{:else if saved && !dirty}
			<span class="mr-auto text-xs text-neutral-500">Saved.</span>
		{:else}
			<span class="mr-auto"></span>
		{/if}
		<button
			type="button"
			disabled={!dirty || saving}
			onclick={reset}
			class="rounded border border-neutral-600 px-3 py-1 text-sm text-neutral-300 hover:bg-neutral-700/50 disabled:opacity-40"
		>
			Reset
		</button>
		<button
			type="button"
			disabled={!dirty || saving || anyInvalid}
			onclick={save}
			class="rounded bg-neutral-200 px-3 py-1 text-sm font-medium text-neutral-900 hover:bg-white disabled:cursor-not-allowed disabled:bg-neutral-700 disabled:text-neutral-500"
		>
			{saving ? 'Saving…' : 'Save changes'}
		</button>
	</footer>
</section>
