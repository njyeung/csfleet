<script lang="ts">
	import { untrack } from 'svelte';
	import { configs } from '$lib/api/services';
	import type { ConfigFile } from '$lib/api/types';
	import { configNameError, filenameError } from '$lib/validation';
	import Modal from './ui/Modal.svelte';

	// Add / edit a config file. A config is a (name, filename, content)
	// tuple: `name` is the catalog PK assignments reference (read-only on edit),
	// `filename` is the path under game/csgo/cfg/ the content is written to.
	let {
		open = false,
		editing = null,
		onclose,
		onsaved
	}: {
		open?: boolean;
		editing?: ConfigFile | null;
		onclose: () => void;
		onsaved: () => void;
	} = $props();

	const isEdit = $derived(editing !== null);

	let name = $state(untrack(() => editing?.name ?? ''));
	let filename = $state(untrack(() => editing?.filename ?? ''));
	let content = $state(untrack(() => editing?.content ?? ''));

	let saving = $state(false);
	let error = $state<string | null>(null);

	const trimmedName = $derived(name.trim());
	const trimmedFile = $derived(filename.trim());
	// Name is the read-only PK on edit, so only validate it when adding; filename
	// is editable in both modes.
	const nameErr = $derived(isEdit ? null : configNameError(trimmedName));
	const fileErr = $derived(filenameError(trimmedFile));
	const canSave = $derived(
		!saving && trimmedName !== '' && trimmedFile !== '' && nameErr === null && fileErr === null
	);

	async function submit() {
		if (!canSave) return;
		saving = true;
		error = null;
		const res = await configs.put(trimmedName, trimmedFile, content);
		saving = false;
		if (res.ok) {
			onsaved();
			onclose();
		} else {
			error = `Save failed (HTTP ${res.status})`;
		}
	}
</script>

<Modal {open} title={isEdit ? `Edit config · ${editing?.name}` : 'Add config'} size="lg" busy={saving} {onclose}>
	<div class="space-y-4">
		<div class="flex items-start justify-between gap-4">
			<label for="cf-name" class="pt-1 text-sm text-neutral-400">Name</label>
			<div class="flex flex-col items-end gap-1">
				<input
					id="cf-name"
					type="text"
					bind:value={name}
					disabled={isEdit}
					placeholder="catalog identifier (assignments reference this)"
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

		<div class="flex items-start justify-between gap-4">
			<label for="cf-filename" class="pt-1 text-sm text-neutral-400">Filename</label>
			<div class="flex flex-col items-end gap-1">
				<input
					id="cf-filename"
					type="text"
					bind:value={filename}
					placeholder="e.g. gamemode_competitive_server.cfg"
					spellcheck="false"
					class={[
						'w-72 rounded border bg-neutral-900 px-2 py-1 font-mono text-sm text-neutral-200 outline-none',
						fileErr ? 'border-red-500/60' : 'border-neutral-700 focus:border-neutral-500'
					]}
				/>
				{#if fileErr}
					<p class="text-xs text-red-400">{fileErr}</p>
				{/if}
			</div>
		</div>
		<p class="text-xs text-neutral-600">
			Path under <span class="font-mono">game/csgo/cfg/</span>. Keep filenames distinct. Two configs
			on the same server with the same filename overwrite with an undefined winner.
		</p>

		<div class="space-y-1">
			<label for="cf-content" class="text-sm text-neutral-400">Content</label>
			<textarea
				id="cf-content"
				bind:value={content}
				rows="14"
				spellcheck="false"
				placeholder="// config file body"
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
				{saving ? 'Saving…' : isEdit ? 'Save changes' : 'Add config'}
			</button>
		</div>
	</div>
</Modal>
