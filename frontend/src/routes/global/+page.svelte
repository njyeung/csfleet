<script lang="ts">
	import { onMount } from 'svelte';
	import { plugins, configs, globals, env, gslt } from '$lib/api/services';
	import type { EnvVar } from '$lib/api/types';
	import GlobalEnvSection from '$lib/components/GlobalEnvSection.svelte';
	import GlobalAssignmentSection from '$lib/components/GlobalAssignmentSection.svelte';
	import GsltSection from '$lib/components/GsltSection.svelte';

	// Global settings: the only editable env/assignment tier, plus the
	// GSLT pool. Each section owns its own staged Save; this page just loads the
	// initial data and gates rendering on it so the sections seed cleanly.
	let loaded = $state(false);
	let loadError = $state<string | null>(null);

	let globalEnv = $state<EnvVar[]>([]);
	let pluginCatalog = $state<string[]>([]);
	let configCatalog = $state<string[]>([]);
	let globalPlugins = $state<string[]>([]);
	let globalConfigs = $state<string[]>([]);
	let tokens = $state<string[]>([]);

	// A seed bumped after a (re)load to remount the staged-save sections so they
	// re-seed their baselines from fresh data.
	let seed = $state(0);

	async function load() {
		const [ge, pl, cf, gp, gc, gt] = await Promise.all([
			env.list('global'),
			plugins.list(),
			configs.list(),
			globals.getPlugins(),
			globals.getConfigs(),
			gslt.list()
		]);
		if (!ge.ok || !pl.ok || !cf.ok || !gp.ok || !gc.ok || !gt.ok) {
			loadError = "Couldn't load global settings — is the orchestrator reachable?";
			loaded = true;
			return;
		}
		// Go nil slices marshal as null, so coalesce every list response.
		globalEnv = ge.data ?? [];
		pluginCatalog = (pl.data ?? []).map((m) => m.name).sort();
		configCatalog = (cf.data ?? []).map((c) => c.name).sort();
		globalPlugins = gp.data.items ?? [];
		globalConfigs = gc.data.items ?? [];
		tokens = (gt.data ?? []).slice().sort();
		loadError = null;
		loaded = true;
		seed++;
	}

	onMount(load);

	// GSLT add/delete are immediate; reload just the pool after a change.
	async function reloadTokens() {
		const res = await gslt.list();
		if (res.ok) tokens = (res.data ?? []).slice().sort();
	}
</script>

<div class="mx-auto max-w-3xl space-y-6 p-6">
	<div>
		<h1 class="text-lg font-semibold text-neutral-100">Global settings</h1>
		<p class="mt-1 text-sm text-neutral-500">
			The base env and plugin/config assignment every server inherits, plus the GSLT token pool.
		</p>
	</div>

	{#if !loaded}
		<p class="text-sm text-neutral-500">Loading…</p>
	{:else if loadError}
		<p class="text-sm text-red-400">{loadError}</p>
	{:else}
		{#key seed}
			<GlobalEnvSection initial={globalEnv} />
			<GlobalAssignmentSection
				label="Global plugins"
				catalog={pluginCatalog}
				initial={globalPlugins}
				onsave={(items) => globals.setPlugins(items)}
			/>
			<GlobalAssignmentSection
				label="Global configs"
				catalog={configCatalog}
				initial={globalConfigs}
				onsave={(items) => globals.setConfigs(items)}
			/>
			<GsltSection {tokens} onchanged={reloadTokens} />
		{/key}
	{/if}
</div>
