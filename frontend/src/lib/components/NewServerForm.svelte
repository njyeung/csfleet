<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { sse } from '$lib/api/sse.svelte';
	import { servers, clusters, plugins, configs, globals, env } from '$lib/api/services';
	import type { CreateServerRequest, EnvVar } from '$lib/api/types';
	import { nameError, isValidEnvKey } from '$lib/validation';
	import NumberField from './ui/NumberField.svelte';
	import HoursField from './ui/HoursField.svelte';
	import Switch from './ui/Switch.svelte';
	import AssignmentPicker from './AssignmentPicker.svelte';
	import EnvEditor from './EnvEditor.svelte';

	// When opened from a cluster's [+], membership is preselected and locked.
	let { lockedCluster = null, onclose }: { lockedCluster?: string | null; onclose: () => void } =
		$props();

	const availableClusters = $derived(sse.clusters.map((c) => c.name).sort());

	// --- form state ---
	let name = $state('');
	// Seeded once from the (stable, create-time) lockedCluster prop.
	let placement = $state<'standalone' | 'member'>(
		untrack(() => (lockedCluster ? 'member' : 'standalone'))
	);
	let port = $state<number | null>(null);
	let cluster = $state(untrack(() => lockedCluster ?? ''));
	let accepting = $state(true);
	let restart = $state<number | null>(-1); // -1 = no limit (default)
	let stop = $state<number | null>(-1);
	let startImmediately = $state(true);
	let pluginSel = $state<string[] | null>(null); // null = inherit (default)
	let configSel = $state<string[] | null>(null);
	let envOverlay = $state<Record<string, string>>({});

	const isMember = $derived(placement === 'member');

	// --- catalog + inherited-preview data ---
	let pluginCatalog = $state<string[]>([]);
	let configCatalog = $state<string[]>([]);
	let globalPlugins = $state<string[]>([]);
	let globalConfigs = $state<string[]>([]);
	let globalEnv = $state<EnvVar[]>([]);
	let clusterPlugins = $state<string[]>([]);
	let clusterConfigs = $state<string[]>([]);
	let clusterEnv = $state<EnvVar[]>([]);
	let previewLoading = $state(false);

	const pluginPreview = $derived(isMember ? clusterPlugins : globalPlugins);
	const configPreview = $derived(isMember ? clusterConfigs : globalConfigs);
	const envInherited = $derived(isMember ? [...globalEnv, ...clusterEnv] : globalEnv);

	onMount(async () => {
		const [pl, cf, gp, gc, ge] = await Promise.all([
			plugins.list(),
			configs.list(),
			globals.getPlugins(),
			globals.getConfigs(),
			env.list('global')
		]);
		if (pl.ok) pluginCatalog = pl.data.map((m) => m.name).sort();
		if (cf.ok) configCatalog = cf.data.map((c) => c.name).sort();
		if (gp.ok) globalPlugins = gp.data.items ?? [];
		if (gc.ok) globalConfigs = gc.data.items ?? [];
		if (ge.ok) globalEnv = ge.data;
		if (lockedCluster) await loadCluster(lockedCluster);
	});

	// Inherited preview for a member comes from its cluster's resolved sets/env.
	async function loadCluster(cl: string) {
		if (!cl) {
			clusterPlugins = [];
			clusterConfigs = [];
			clusterEnv = [];
			return;
		}
		previewLoading = true;
		const [cp, cc, ce] = await Promise.all([
			clusters.plugins(cl),
			clusters.configs(cl),
			clusters.env(cl)
		]);
		clusterPlugins = cp.ok ? (cp.data.items ?? []) : [];
		clusterConfigs = cc.ok ? (cc.data.items ?? []) : [];
		clusterEnv = ce.ok ? ce.data : [];
		previewLoading = false;
	}

	function onPlacement(next: 'standalone' | 'member') {
		placement = next;
		if (next === 'member' && cluster) loadCluster(cluster);
	}
	function onCluster(e: Event & { currentTarget: HTMLSelectElement }) {
		cluster = e.currentTarget.value;
		loadCluster(cluster);
	}

	// --- submit ---
	let saving = $state(false);
	let error = $state<string | null>(null);

	const trimmedName = $derived(name.trim());
	const nameErr = $derived(nameError(trimmedName));
	// An invalid overlay key (space / '=') blocks submit; the row is flagged too.
	const envInvalid = $derived(Object.keys(envOverlay).some((k) => !isValidEnvKey(k)));
	const portInvalid = $derived(placement === 'standalone' && (port == null || port <= 0 || port > 65535));
	const canSave = $derived(
		!saving &&
			trimmedName !== '' &&
			nameErr === null &&
			!envInvalid &&
			(placement === 'standalone' ? !portInvalid : cluster !== '')
	);

	async function submit() {
		if (!canSave) return;
		saving = true;
		error = null;

		const body: CreateServerRequest = { name: trimmedName };
		if (placement === 'standalone') body.port = port;
		else body.cluster = cluster;
		body.accepting_connections = accepting;
		body.restart_after_hrs = restart;
		body.stop_after_hrs = stop;
		body.desired_state = startImmediately ? 'running' : 'stopped';
		// Tri-state: omit = inherit; an explicit (possibly empty) set is sent as-is.
		if (pluginSel !== null) body.plugins = pluginSel;
		if (configSel !== null) body.configs = configSel;
		if (Object.keys(envOverlay).length) body.env = envOverlay;

		const res = await servers.create(body);
		saving = false;
		if (res.ok) {
			onclose();
			goto(`/servers/${encodeURIComponent(trimmedName)}`);
		} else {
			error = `Create failed (HTTP ${res.status})`;
		}
	}
</script>

<div class="space-y-4">
	<!-- Name -->
	<div class="flex items-start justify-between gap-4">
		<label for="ns-name" class="pt-1 text-sm text-neutral-400">Name</label>
		<div class="flex flex-col items-end gap-1">
			<input
				id="ns-name"
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

	<!-- Placement -->
	<div class="flex items-start justify-between gap-4">
		<span class="pt-1 text-sm text-neutral-400">Placement</span>
		<div class="flex flex-col items-end gap-2">
			<div class="flex items-center gap-3 text-sm">
				<label class="flex items-center gap-1.5 {lockedCluster ? 'opacity-40' : ''}">
					<input
						type="radio"
						name="placement"
						checked={placement === 'standalone'}
						disabled={!!lockedCluster}
						onchange={() => onPlacement('standalone')}
					/>
					Standalone
				</label>
				<label class="flex items-center gap-1.5">
					<input
						type="radio"
						name="placement"
						checked={placement === 'member'}
						disabled={!!lockedCluster}
						onchange={() => onPlacement('member')}
					/>
					Cluster member
				</label>
			</div>
			{#if placement === 'standalone'}
				<NumberField bind:value={port} min={1} max={65535} placeholder="port" invalid={portInvalid} />
			{:else if lockedCluster}
				<span class="text-sm text-neutral-300">{lockedCluster}</span>
			{:else}
				<select
					value={cluster}
					onchange={onCluster}
					class="w-56 rounded border border-neutral-700 bg-neutral-900 px-2 py-1 text-sm text-neutral-200 outline-none focus:border-neutral-500"
				>
					<option value="">Select a cluster…</option>
					{#each availableClusters as c (c)}
						<option value={c}>{c}</option>
					{/each}
				</select>
			{/if}
		</div>
	</div>

	<!-- Toggles -->
	<div class="flex items-center justify-between gap-4">
		<span class="text-sm text-neutral-400">Accepting connections</span>
		<Switch bind:checked={accepting} label="Accepting connections" />
	</div>
	<div class="flex items-center justify-between gap-4">
		<span class="text-sm text-neutral-400">Start immediately</span>
		<Switch bind:checked={startImmediately} label="Start immediately" />
	</div>

	<!-- Hour limits -->
	<div class="flex items-start justify-between gap-4">
		<span class="pt-1.5 text-sm text-neutral-400">Restart after</span>
		<HoursField bind:value={restart} allowInherit={isMember} />
	</div>
	<div class="flex items-start justify-between gap-4">
		<span class="pt-1.5 text-sm text-neutral-400">Stop after</span>
		<HoursField bind:value={stop} allowInherit={isMember} />
	</div>

	<hr class="border-neutral-700" />

	<!-- Assignments -->
	<AssignmentPicker
		label="Plugins"
		catalog={pluginCatalog}
		bind:value={pluginSel}
		allowInherit
		inheritPreview={pluginPreview}
		{previewLoading}
	/>
	<AssignmentPicker
		label="Configs"
		catalog={configCatalog}
		bind:value={configSel}
		allowInherit
		inheritPreview={configPreview}
		{previewLoading}
	/>

	<hr class="border-neutral-700" />

	<!-- Env overlay -->
	<EnvEditor
		bind:value={envOverlay}
		inherited={envInherited}
		inheritedLoading={previewLoading}
		serverName={trimmedName}
	/>

	<!-- Footer -->
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
			{saving ? 'Creating…' : 'Create server'}
		</button>
	</div>
</div>
