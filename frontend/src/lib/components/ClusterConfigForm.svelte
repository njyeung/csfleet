<script lang="ts">
	import { untrack } from 'svelte';
	import { clusters as clustersApi } from '$lib/api/services';
	import type { Cluster, LBPolicy, UpdateClusterRequest } from '$lib/api/types';
	import Switch from './ui/Switch.svelte';
	import NumberField from './ui/NumberField.svelte';
	import HoursField from './ui/HoursField.svelte';

	// Keyed by cluster name in the parent, so it re-seeds on navigation.
	let { cluster }: { cluster: Cluster } = $props();

	const POLICIES: { value: LBPolicy; label: string }[] = [
		{ value: 'round_robin', label: 'Round robin' },
		{ value: 'packing', label: 'Packing' },
		{ value: 'sparse', label: 'Sparse' }
	];
	const isPolicy = (v: string): v is LBPolicy => POLICIES.some((p) => p.value === v);

	// Staged edits, seeded once (untrack: deliberate one-time read).
	let port = $state<number | null>(untrack(() => cluster.port));
	let policy = $state<LBPolicy>(untrack(() => (isPolicy(cluster.lb_policy) ? cluster.lb_policy : 'round_robin')));
	let autoToken = $state(untrack(() => cluster.auto_token));
	let accepting = $state(untrack(() => cluster.accepting_connections));
	let restart = $state<number | null>(untrack(() => cluster.restart_after_hrs ?? null));
	let stop = $state<number | null>(untrack(() => cluster.stop_after_hrs ?? null));

	function snapshot() {
		return { port, policy, autoToken, accepting, restart, stop };
	}
	let baseline = $state(untrack(snapshot));

	let saving = $state(false);
	let error = $state<string | null>(null);

	// Clusters never inherit: null and -1 both mean "no limit".
	function hrsKey(v: number | null): string {
		return v == null || v < 0 ? 'nolimit' : `v${v}`;
	}

	const portInvalid = $derived(port == null || port <= 0);
	const portChanged = $derived(port !== baseline.port);
	const dirty = $derived(
		portChanged ||
			policy !== baseline.policy ||
			autoToken !== baseline.autoToken ||
			accepting !== baseline.accepting ||
			hrsKey(restart) !== hrsKey(baseline.restart) ||
			hrsKey(stop) !== hrsKey(baseline.stop)
	);
	const canSave = $derived(dirty && !saving && !portInvalid);

	function reset() {
		port = baseline.port;
		policy = baseline.policy;
		autoToken = baseline.autoToken;
		accepting = baseline.accepting;
		restart = baseline.restart;
		stop = baseline.stop;
		error = null;
	}

	async function save() {
		if (!canSave || port == null) return;
		saving = true;
		error = null;
		const body: UpdateClusterRequest = {
			port,
			lb_policy: policy,
			auto_token: autoToken,
			accepting_connections: accepting,
			restart_after_hrs: restart,
			stop_after_hrs: stop
		};
		const res = await clustersApi.update(cluster.name, body);
		if (res.ok) {
			baseline = snapshot();
			// The SSE state frame reflects the new port/policy in the sidebar/detail.
		} else {
			error = `Save failed (HTTP ${res.status})`;
		}
		saving = false;
	}
</script>

<div class="rounded border border-neutral-700 bg-neutral-800">
	<div class="space-y-1 p-4">
		<h2 class="mb-2 text-xs font-semibold uppercase tracking-wide text-neutral-500">Configuration</h2>

		<div class="flex items-start justify-between gap-4 py-2">
			<label for="cl-port" class="pt-1.5 text-sm text-neutral-400">Port</label>
			<div class="text-right">
				<NumberField id="cl-port" bind:value={port} min={1} max={65535} invalid={portInvalid} />
				{#if portChanged}
					<p class="mt-1 text-xs text-amber-400">Changing the port rebinds every member.</p>
				{/if}
			</div>
		</div>

		<div class="flex items-center justify-between gap-4 py-2">
			<label for="cl-policy" class="text-sm text-neutral-400">LB policy</label>
			<select
				id="cl-policy"
				bind:value={policy}
				class="rounded border border-neutral-700 bg-neutral-900 px-2 py-1 text-sm text-neutral-200 outline-none focus:border-neutral-500"
			>
				{#each POLICIES as p (p.value)}
					<option value={p.value}>{p.label}</option>
				{/each}
			</select>
		</div>

		<div class="flex items-center justify-between gap-4 py-2">
			<span class="text-sm text-neutral-400">Auto token</span>
			<Switch bind:checked={autoToken} label="Auto token" />
		</div>

		<div class="flex items-center justify-between gap-4 py-2">
			<span class="text-sm text-neutral-400">Accepting connections</span>
			<Switch bind:checked={accepting} label="Accepting connections" />
		</div>

		<div class="flex items-start justify-between gap-4 py-2">
			<span class="pt-1.5 text-sm text-neutral-400">Restart after</span>
			<HoursField bind:value={restart} />
		</div>

		<div class="flex items-start justify-between gap-4 py-2">
			<span class="pt-1.5 text-sm text-neutral-400">Stop after</span>
			<HoursField bind:value={stop} />
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
