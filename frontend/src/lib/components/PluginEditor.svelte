<script lang="ts">
	import { untrack } from 'svelte';
	import { plugins } from '$lib/api/services';
	import type { Manifest } from '$lib/api/types';
	import { pluginNameError } from '$lib/validation';
	import Modal from './ui/Modal.svelte';

	// Add / edit a plugin manifest. In edit mode `editing` carries the
	// existing manifest; its name is the PK and stays read-only. In add mode the
	// name is free text (the PUT route key).
	let {
		open = false,
		editing = null,
		onclose,
		onsaved
	}: {
		open?: boolean;
		editing?: Manifest | null;
		onclose: () => void;
		onsaved: () => void;
	} = $props();

	const isEdit = $derived(editing !== null);

	// Seeded once from the (stable, per-open) editing prop; the parent remounts
	// this via {#key} so each open re-seeds.
	let name = $state(untrack(() => editing?.name ?? ''));
	let manifest = $state(untrack(() => editing?.manifest ?? ''));

	let saving = $state(false);
	let error = $state<string | null>(null);

	const trimmedName = $derived(name.trim());
	// Validate the name only when adding — on edit it's the existing (read-only) PK.
	const nameErr = $derived(isEdit ? null : pluginNameError(trimmedName));
	const canSave = $derived(!saving && trimmedName !== '' && nameErr === null && manifest.trim() !== '');

	async function submit() {
		if (!canSave) return;
		saving = true;
		error = null;
		const res = await plugins.put(trimmedName, manifest);
		saving = false;
		if (res.ok) {
			onsaved();
			onclose();
		} else {
			error = `Save failed (HTTP ${res.status})`;
		}
	}
</script>

<Modal {open} title={isEdit ? `Edit plugin · ${editing?.name}` : 'Add plugin'} size="lg" busy={saving} {onclose}>
	<div class="space-y-4">
		<div class="flex items-start justify-between gap-4">
			<label for="pl-name" class="pt-1 text-sm text-neutral-400">Name</label>
			<div class="flex flex-col items-end gap-1">
				<input
					id="pl-name"
					type="text"
					bind:value={name}
					disabled={isEdit}
					placeholder="unique plugin name"
					spellcheck="false"
					class={[
						'w-72 rounded border bg-neutral-900 px-2 py-1 text-sm text-neutral-200 outline-none disabled:opacity-50',
						nameErr ? 'border-red-500/60' : 'border-neutral-700 focus:border-neutral-500'
					]}
				/>
				{#if nameErr}
					<p class="text-xs text-red-400">{nameErr}</p>
				{/if}
			</div>
		</div>

		<div class="space-y-1">
			<label for="pl-manifest" class="text-sm text-neutral-400">Manifest (TOML)</label>
			<textarea
				id="pl-manifest"
				bind:value={manifest}
				rows="16"
				spellcheck="false"
				placeholder="# plugin manifest TOML"
				class="w-full resize-y rounded border border-neutral-700 bg-neutral-900 px-3 py-2 font-mono text-sm text-neutral-200 outline-none focus:border-neutral-500"
			></textarea>
		</div>

		<div class="flex items-center gap-2 border-t border-neutral-700 pt-3">
			{#if error}
				<span class="mr-auto text-xs text-red-400">{error}</span>
			{:else}
				<span class="mr-auto"></span>
			{/if}
			<button
				type="button"
				onclick={onclose}
				class="rounded border border-neutral-600 px-3 py-1 text-sm text-neutral-300 hover:bg-neutral-700/50"
			>
				Cancel
			</button>
			<button
				type="button"
				disabled={!canSave}
				onclick={submit}
				class="rounded bg-neutral-200 px-3 py-1 text-sm font-medium text-neutral-900 hover:bg-white disabled:cursor-not-allowed disabled:bg-neutral-700 disabled:text-neutral-500"
			>
				{saving ? 'Saving…' : isEdit ? 'Save changes' : 'Add plugin'}
			</button>
		</div>
	</div>
</Modal>
