<script lang="ts">
	import { untrack } from 'svelte';
	import { servers } from '$lib/api/services';
	import type { Server, UpdateServerRequest } from '$lib/api/types';
	import BoolField from './ui/BoolField.svelte';
	import NumberField from './ui/NumberField.svelte';
	import HoursField from './ui/HoursField.svelte';

	// The parent renders this keyed by server name, so it remounts (and re-seeds)
	// on navigation but survives live SSE ticks while editing.
	let { server }: { server: Server } = $props();

	// Membership is create-only, so it's stable for this instance.
	const isMember = $derived(!!server.cluster);

	// Staged edits, seeded once from the live row (untrack: deliberate one-time read).
	let port = $state(untrack(() => server.port));
	// Keep null (= inherit from cluster for a member; backend default otherwise) —
	// don't coerce to false, or saving a member would clobber its inheritance.
	let autoToken = $state<boolean | null>(untrack(() => server.auto_token));
	let accepting = $state<boolean | null>(untrack(() => server.accepting_connections));
	let restart = $state<number | null>(untrack(() => server.restart_after_hrs ?? null));
	let stop = $state<number | null>(untrack(() => server.stop_after_hrs ?? null));

	function snapshot() {
		return { port, autoToken, accepting, restart, stop };
	}
	// Baseline for dirty-tracking; advances on a successful save (independent of SSE).
	let baseline = $state(untrack(snapshot));

	let saving = $state(false);
	let error = $state<string | null>(null);

	// null and -1 both mean "no limit" for a standalone server, so canonicalize
	// before comparing to avoid a spurious dirty state.
	function hrsKey(v: number | null): string {
		if (v == null) return isMember ? 'inherit' : 'nolimit';
		if (v < 0) return 'nolimit';
		return `v${v}`;
	}

	// Tri-state for members (null=inherit); for a standalone, null and true both
	// resolve to the on default, so canonicalize them to avoid a spurious dirty.
	function boolKey(v: boolean | null): string {
		if (v == null) return isMember ? 'inherit' : 'on';
		return v ? 'on' : 'off';
	}

	const portInvalid = $derived(!isMember && (port == null || port <= 0));
	const dirty = $derived(
		port !== baseline.port ||
			boolKey(autoToken) !== boolKey(baseline.autoToken) ||
			boolKey(accepting) !== boolKey(baseline.accepting) ||
			hrsKey(restart) !== hrsKey(baseline.restart) ||
			hrsKey(stop) !== hrsKey(baseline.stop)
	);
	const canSave = $derived(dirty && !saving && !portInvalid);

	function reset() {
		port = baseline.port;
		autoToken = baseline.autoToken;
		accepting = baseline.accepting;
		restart = baseline.restart;
		stop = baseline.stop;
		error = null;
	}

	async function save() {
		if (!canSave) return;
		saving = true;
		error = null;
		const body: UpdateServerRequest = {
			auto_token: autoToken,
			accepting_connections: accepting,
			restart_after_hrs: restart,
			stop_after_hrs: stop
		};
		// A member's port is owned by the cluster — never send it.
		if (!isMember) body.port = port;

		const res = await servers.update(server.name, body);
		saving = false;
		if (res.ok) baseline = snapshot();
		else error = `Save failed (HTTP ${res.status})`;
	}
</script>

<div class="rounded border border-neutral-700 bg-neutral-800">
	<div class="space-y-1 p-4">
		<h2 class="mb-2 text-xs font-semibold uppercase tracking-wide text-neutral-500">Configuration</h2>

		<div class="flex items-center justify-between gap-4 py-2">
			<label for="srv-port" class="text-sm text-neutral-400">Port</label>
			{#if isMember}
				<span class="text-sm text-neutral-500">managed by cluster</span>
			{:else}
				<NumberField id="srv-port" bind:value={port} min={1} max={65535} invalid={portInvalid} />
			{/if}
		</div>

		<div class="flex items-start justify-between gap-4 py-2">
			<span class="pt-1 text-sm text-neutral-400">Auto token</span>
			<BoolField bind:value={autoToken} allowInherit={isMember} label="Auto token" />
		</div>

		<div class="flex items-start justify-between gap-4 py-2">
			<span class="pt-1 text-sm text-neutral-400">Accepting connections</span>
			<BoolField bind:value={accepting} allowInherit={isMember} label="Accepting connections" />
		</div>

		<div class="flex items-start justify-between gap-4 py-2">
			<span class="pt-1.5 text-sm text-neutral-400">Restart after</span>
			<HoursField bind:value={restart} allowInherit={isMember} />
		</div>

		<div class="flex items-start justify-between gap-4 py-2">
			<span class="pt-1.5 text-sm text-neutral-400">Stop after</span>
			<HoursField bind:value={stop} allowInherit={isMember} />
		</div>
	</div>

	<div class="sticky bottom-0 rounded-b bg-neutral-800/95 backdrop-blur">
		<div class="mx-4 border-t border-neutral-700"></div>
		<div class="flex items-center gap-2 px-4 py-3">
			{#if error}
				<span class="mr-auto text-xs text-red-400">{error}</span>
			{:else if portInvalid}
				<span class="mr-auto text-xs text-amber-400">Port must be 1–65535.</span>
			{:else}
				<span class="mr-auto text-xs text-neutral-600">
					{dirty ? 'Unsaved changes' : ''}
				</span>
			{/if}
			<button
				type="button"
				disabled={!dirty || saving}
				onclick={reset}
				class="rounded border border-neutral-600 px-3 py-1 text-sm text-neutral-300 hover:bg-neutral-700/50 disabled:cursor-not-allowed disabled:opacity-40"
			>
				Reset
			</button>
			<button
				type="button"
				disabled={!canSave}
				onclick={save}
				class="rounded bg-neutral-200 px-3 py-1 text-sm font-medium text-neutral-900 hover:bg-white disabled:cursor-not-allowed disabled:bg-neutral-700 disabled:text-neutral-500"
			>
				{saving ? 'Saving…' : 'Save changes'}
			</button>
		</div>
	</div>
</div>
