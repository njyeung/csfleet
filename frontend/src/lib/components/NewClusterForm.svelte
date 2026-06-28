<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { clusters, plugins, configs, env } from '$lib/api/services';
	import type { CreateClusterRequest, LBPolicy, EnvVar } from '$lib/api/types';
	import { nameError, isValidEnvKey } from '$lib/validation';
	import NumberField from './ui/NumberField.svelte';
	import HoursField from './ui/HoursField.svelte';
	import Switch from './ui/Switch.svelte';
	import AssignmentPicker from './AssignmentPicker.svelte';
	import EnvEditor from './EnvEditor.svelte';

	let { onclose }: { onclose: () => void } = $props();

	const POLICIES: { value: LBPolicy; label: string }[] = [
		{ value: 'round_robin', label: 'Round robin' },
		{ value: 'packing', label: 'Packing' },
		{ value: 'sparse', label: 'Sparse' }
	];

	// --- form state ---
	let name = $state('');
	let port = $state<number | null>(null);
	let policy = $state<LBPolicy>('round_robin');
	let autoToken = $state(true); // New Cluster defaults auto_token on
	let accepting = $state(true);
	let restart = $state<number | null>(-1);
	let stop = $state<number | null>(-1);
	// Clusters never inherit: explicit sets, default None.
	let pluginSel = $state<string[]>([]);
	let configSel = $state<string[]>([]);
	let envOverlay = $state<Record<string, string>>({});

	// --- catalog + inherited (global env) ---
	let pluginCatalog = $state<string[]>([]);
	let configCatalog = $state<string[]>([]);
	let globalEnv = $state<EnvVar[]>([]);

	onMount(async () => {
		const [pl, cf, ge] = await Promise.all([plugins.list(), configs.list(), env.list('global')]);
		if (pl.ok) pluginCatalog = pl.data.map((m) => m.name).sort();
		if (cf.ok) configCatalog = cf.data.map((c) => c.name).sort();
		if (ge.ok) globalEnv = ge.data;
	});

	// --- submit ---
	let saving = $state(false);
	let error = $state<string | null>(null);

	const trimmedName = $derived(name.trim());
	const nameErr = $derived(nameError(trimmedName));
	// An invalid overlay key (space / '=') blocks submit; the row is flagged too.
	const envInvalid = $derived(Object.keys(envOverlay).some((k) => !isValidEnvKey(k)));
	const portInvalid = $derived(port == null || port <= 0 || port > 65535);
	const canSave = $derived(!saving && trimmedName !== '' && nameErr === null && !envInvalid && !portInvalid);

	async function submit() {
		if (!canSave || port == null) return;
		saving = true;
		error = null;

		const body: CreateClusterRequest = {
			name: trimmedName,
			port,
			lb_policy: policy,
			auto_token: autoToken,
			accepting_connections: accepting,
			restart_after_hrs: restart,
			stop_after_hrs: stop,
			// Always explicit for clusters — never omitted.
			plugins: pluginSel,
			configs: configSel
		};
		if (Object.keys(envOverlay).length) body.env = envOverlay;

		const res = await clusters.create(body);
		saving = false;
		if (res.ok) {
			onclose();
			goto(`/clusters/${encodeURIComponent(trimmedName)}`);
		} else {
			error = `Create failed (HTTP ${res.status})`;
		}
	}
</script>

<div class="space-y-4">
	<div class="flex items-start justify-between gap-4">
		<label for="nc-name" class="pt-1 text-sm text-neutral-400">Name</label>
		<div class="flex flex-col items-end gap-1">
			<input
				id="nc-name"
				type="text"
				bind:value={name}
				placeholder="Unique Name"
				spellcheck="false"
				class={[
					'w-56 rounded border bg-neutral-900 px-2 py-1 text-sm text-neutral-200 outline-none',
					nameErr ? 'border-red-500/60' : 'border-neutral-700 focus:border-neutral-500'
				]}
			/>
			{#if nameErr}
				<p class="text-xs text-red-400">{nameErr}</p>
			{/if}
		</div>
	</div>

	<div class="flex items-center justify-between gap-4">
		<label for="nc-port" class="text-sm text-neutral-400">Port</label>
		<NumberField id="nc-port" bind:value={port} min={1} max={65535} placeholder="ingress port" invalid={portInvalid} />
	</div>

	<div class="flex items-center justify-between gap-4">
		<label for="nc-policy" class="text-sm text-neutral-400">LB policy</label>
		<select
			id="nc-policy"
			bind:value={policy}
			class="rounded border border-neutral-700 bg-neutral-900 px-2 py-1 text-sm text-neutral-200 outline-none focus:border-neutral-500"
		>
			{#each POLICIES as p (p.value)}
				<option value={p.value}>{p.label}</option>
			{/each}
		</select>
	</div>

	<div class="flex items-center justify-between gap-4">
		<span class="text-sm text-neutral-400">Auto token</span>
		<Switch bind:checked={autoToken} label="Auto token" />
	</div>
	<div class="flex items-center justify-between gap-4">
		<span class="text-sm text-neutral-400">Accepting connections</span>
		<Switch bind:checked={accepting} label="Accepting connections" />
	</div>

	<div class="flex items-start justify-between gap-4">
		<span class="pt-1.5 text-sm text-neutral-400">Restart after</span>
		<HoursField bind:value={restart} />
	</div>
	<div class="flex items-start justify-between gap-4">
		<span class="pt-1.5 text-sm text-neutral-400">Stop after</span>
		<HoursField bind:value={stop} />
	</div>

	<hr class="border-neutral-700" />

	<AssignmentPicker label="Plugins" catalog={pluginCatalog} bind:value={pluginSel} />
	<AssignmentPicker label="Configs" catalog={configCatalog} bind:value={configSel} />

	<hr class="border-neutral-700" />

	<!-- Cluster env overlay. -->
	<EnvEditor bind:value={envOverlay} inherited={globalEnv} />

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
			{saving ? 'Creating…' : 'Create cluster'}
		</button>
	</div>
</div>
