<script lang="ts">
	import { onMount } from 'svelte';
	import { Pencil, Trash2, Plus } from '@lucide/svelte';
	import { configs } from '$lib/api/services';
	import type { ConfigFile } from '$lib/api/types';
	import ConfigEditor from '$lib/components/ConfigEditor.svelte';
	import ConfirmDialog from '$lib/components/ui/ConfirmDialog.svelte';

	// The config-file catalog: list, add/edit (name + filename +
	// content), delete. Parallel to the Plugins page.
	let list = $state<ConfigFile[]>([]);
	let loading = $state(true);
	let loadError = $state<string | null>(null);

	const fmtDate = (s: string) => new Date(s).toLocaleString();

	async function reload() {
		const res = await configs.list();
		if (res.ok) {
			list = (res.data ?? []).slice().sort((a, b) => a.name.localeCompare(b.name));
			loadError = null;
		} else {
			loadError = `Couldn't load configs (HTTP ${res.status})`;
		}
		loading = false;
	}

	onMount(reload);

	// --- editor modal ---
	let editorOpen = $state(false);
	let editing = $state<ConfigFile | null>(null);
	let editorSeed = $state(0);

	function openAdd() {
		editing = null;
		editorSeed++;
		editorOpen = true;
	}
	function openEdit(c: ConfigFile) {
		editing = c;
		editorSeed++;
		editorOpen = true;
	}

	// --- delete ---
	let toDelete = $state<ConfigFile | null>(null);
	let deleting = $state(false);
	let deleteError = $state<string | null>(null);

	function askDelete(c: ConfigFile) {
		toDelete = c;
		deleteError = null;
	}
	async function confirmDelete() {
		if (!toDelete) return;
		deleting = true;
		const res = await configs.remove(toDelete.name);
		deleting = false;
		if (res.ok) {
			toDelete = null;
			await reload();
		} else {
			deleteError =
				res.status === 409
					? 'In use — this config is assigned to a scope. Unassign it first.'
					: `Delete failed (HTTP ${res.status})`;
		}
	}
</script>

<div class="mx-auto max-w-3xl p-6">
	<div class="flex items-center justify-between">
		<div>
			<h1 class="text-lg font-semibold text-neutral-100">Configs</h1>
			<p class="mt-1 text-sm text-neutral-500">
				The config file catalog.
			</p>
		</div>
		<button
			type="button"
			onclick={openAdd}
			class="flex items-center gap-1.5 rounded border border-neutral-600 px-3 py-1.5 text-sm text-neutral-200 hover:bg-neutral-700/50"
		>
			<Plus size={15} /> Add config
		</button>
	</div>

	<div class="mt-5 overflow-hidden rounded-lg border border-neutral-700">
		{#if loading}
			<p class="px-4 py-6 text-sm text-neutral-500">Loading…</p>
		{:else if loadError}
			<p class="px-4 py-6 text-sm text-red-400">{loadError}</p>
		{:else if list.length === 0}
			<p class="px-4 py-6 text-sm text-neutral-500">No configs yet. Add one to get started.</p>
		{:else}
			<table class="w-full text-sm">
				<thead class="border-b border-neutral-700 bg-neutral-800/50 text-left text-xs uppercase tracking-wide text-neutral-500">
					<tr>
						<th class="px-4 py-2 font-medium">Name</th>
						<th class="px-4 py-2 font-medium">Filename</th>
						<th class="px-4 py-2 font-medium">Updated</th>
						<th class="w-24 px-4 py-2"></th>
					</tr>
				</thead>
				<tbody>
					{#each list as c (c.name)}
						<tr class="border-b border-neutral-800 last:border-0 hover:bg-neutral-800/40">
							<td class="px-4 py-2 font-mono text-neutral-200">{c.name}</td>
							<td class="px-4 py-2 font-mono text-neutral-400">{c.filename}</td>
							<td class="px-4 py-2 text-neutral-500">{fmtDate(c.updated_at)}</td>
							<td class="px-4 py-2">
								<div class="flex items-center justify-end gap-1">
									<button
										type="button"
										title="Edit"
										aria-label="Edit {c.name}"
										onclick={() => openEdit(c)}
										class="rounded p-1.5 text-neutral-400 hover:bg-neutral-700/50 hover:text-neutral-200"
									>
										<Pencil size={15} />
									</button>
									<button
										type="button"
										title="Delete"
										aria-label="Delete {c.name}"
										onclick={() => askDelete(c)}
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
	<ConfigEditor open={editorOpen} {editing} onclose={() => (editorOpen = false)} onsaved={reload} />
{/key}

<ConfirmDialog
	open={toDelete !== null}
	title="Delete config"
	message={deleteError ?? `Delete "${toDelete?.name}" from the catalog? This can't be undone.`}
	confirmLabel="Delete"
	busy={deleting}
	onconfirm={confirmDelete}
	oncancel={() => (toDelete = null)}
/>
