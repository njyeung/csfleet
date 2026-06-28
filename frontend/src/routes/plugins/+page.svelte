<script lang="ts">
	import { onMount } from 'svelte';
	import { Pencil, Trash2, Plus } from '@lucide/svelte';
	import { plugins } from '$lib/api/services';
	import type { Manifest } from '$lib/api/types';
	import PluginEditor from '$lib/components/PluginEditor.svelte';
	import ConfirmDialog from '$lib/components/ui/ConfirmDialog.svelte';

	// The plugin manifest catalog: list, add/edit TOML, delete. This is
	// the catalog of available plugins, distinct from per-scope assignment.
	let list = $state<Manifest[]>([]);
	let loading = $state(true);
	let loadError = $state<string | null>(null);

	const fmtDate = (s: string) => new Date(s).toLocaleString();

	async function reload() {
		const res = await plugins.list();
		if (res.ok) {
			list = (res.data ?? []).slice().sort((a, b) => a.name.localeCompare(b.name));
			loadError = null;
		} else {
			loadError = `Couldn't load plugins (HTTP ${res.status})`;
		}
		loading = false;
	}

	onMount(reload);

	// --- editor modal ---
	let editorOpen = $state(false);
	let editing = $state<Manifest | null>(null);
	let editorSeed = $state(0); // bump to remount the editor so it re-seeds

	function openAdd() {
		editing = null;
		editorSeed++;
		editorOpen = true;
	}
	// Edit needs the full manifest body; the list rows already carry it.
	function openEdit(m: Manifest) {
		editing = m;
		editorSeed++;
		editorOpen = true;
	}

	// --- delete ---
	let toDelete = $state<Manifest | null>(null);
	let deleting = $state(false);
	let deleteError = $state<string | null>(null);

	function askDelete(m: Manifest) {
		toDelete = m;
		deleteError = null;
	}
	async function confirmDelete() {
		if (!toDelete) return;
		deleting = true;
		const res = await plugins.remove(toDelete.name);
		deleting = false;
		if (res.ok) {
			toDelete = null;
			await reload();
		} else {
			// The backend blocks deletion while the plugin is assigned somewhere.
			deleteError =
				res.status === 409
					? 'In use — this plugin is assigned to a scope. Unassign it first.'
					: `Delete failed (HTTP ${res.status})`;
		}
	}
</script>

<div class="mx-auto max-w-3xl p-6">
	<div class="flex items-center justify-between">
		<div>
			<h1 class="text-lg font-semibold text-neutral-100">Plugins</h1>
			<p class="mt-1 text-sm text-neutral-500">
				The plugin manifest catalog.
			</p>
		</div>
		<button
			type="button"
			onclick={openAdd}
			class="flex items-center gap-1.5 rounded border border-neutral-600 px-3 py-1.5 text-sm text-neutral-200 hover:bg-neutral-700/50"
		>
			<Plus size={15} /> Add plugin
		</button>
	</div>

	<div class="mt-5 overflow-hidden rounded-lg border border-neutral-700">
		{#if loading}
			<p class="px-4 py-6 text-sm text-neutral-500">Loading…</p>
		{:else if loadError}
			<p class="px-4 py-6 text-sm text-red-400">{loadError}</p>
		{:else if list.length === 0}
			<p class="px-4 py-6 text-sm text-neutral-500">No plugins yet. Add one to get started.</p>
		{:else}
			<table class="w-full text-sm">
				<thead class="border-b border-neutral-700 bg-neutral-800/50 text-left text-xs uppercase tracking-wide text-neutral-500">
					<tr>
						<th class="px-4 py-2 font-medium">Name</th>
						<th class="px-4 py-2 font-medium">Updated</th>
						<th class="w-24 px-4 py-2"></th>
					</tr>
				</thead>
				<tbody>
					{#each list as m (m.name)}
						<tr class="border-b border-neutral-800 last:border-0 hover:bg-neutral-800/40">
							<td class="px-4 py-2 font-mono text-neutral-200">{m.name}</td>
							<td class="px-4 py-2 text-neutral-500">{fmtDate(m.updated_at)}</td>
							<td class="px-4 py-2">
								<div class="flex items-center justify-end gap-1">
									<button
										type="button"
										title="Edit"
										aria-label="Edit {m.name}"
										onclick={() => openEdit(m)}
										class="rounded p-1.5 text-neutral-400 hover:bg-neutral-700/50 hover:text-neutral-200"
									>
										<Pencil size={15} />
									</button>
									<button
										type="button"
										title="Delete"
										aria-label="Delete {m.name}"
										onclick={() => askDelete(m)}
										class="rounded p-1.5 text-neutral-400 hover:bg-red-500/10 hover:text-red-400"
									>
										<Trash2 size={15} />
									</button>
								</div>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</div>
</div>

{#key editorSeed}
	<PluginEditor open={editorOpen} {editing} onclose={() => (editorOpen = false)} onsaved={reload} />
{/key}

<ConfirmDialog
	open={toDelete !== null}
	title="Delete plugin"
	message={deleteError ?? `Delete "${toDelete?.name}" from the catalog? This can't be undone.`}
	confirmLabel="Delete"
	busy={deleting}
	onconfirm={confirmDelete}
	oncancel={() => (toDelete = null)}
/>
