<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { fleet } from '$lib/stores/fleet.svelte';
	import { sse } from '$lib/api/sse.svelte';
	import { servers } from '$lib/api/services';
	import { orchestratorinfo } from '$lib/api/services/orchestratorinfo';
	import type { AssignmentSet, EnvVar } from '$lib/api/types';
	import StateBadge from '$lib/components/ui/StateBadge.svelte';
	import ConfirmDialog from '$lib/components/ui/ConfirmDialog.svelte';
	import AssignmentList from '$lib/components/AssignmentList.svelte';
	import EnvList from '$lib/components/EnvList.svelte';
	import ServerConfigForm from '$lib/components/ServerConfigForm.svelte';
	import ConnectBox from '$lib/components/ui/ConnectBox.svelte';
	import { Play, Square, Trash2, type LucideIcon } from '@lucide/svelte';

	type Sets = { plugins: AssignmentSet | null; configs: AssignmentSet | null; env: EnvVar[] | null };

	let { name, sets }: { name: string; sets: Promise<Sets> } = $props();

	// Live row from the SSE snapshot (reactive).
	const server = $derived(fleet.server(name));
	// Distinguish "snapshot hasn't arrived" from "this server doesn't exist".
	const connecting = $derived(!sse.connected && sse.snapshot.length === 0);
	
	// Start/Stop/Delete are immediate and independent of the staged-save form.
	const canStart = $derived(
		!!server && !['running', 'starting', 'pending'].includes(server.actual_state)
	);
	const canStop = $derived(!!server && !['stopped', 'stopping'].includes(server.actual_state));

	let busy = $state<null | 'start' | 'stop' | 'delete'>(null);
	let actionError = $state<string | null>(null);
	let confirmDelete = $state(false);

	// Orchestrator host IPs — same for every server, fetched once. Public is the
	// internet-facing address players connect to; local is the LAN/private one.
	let publicIp = $state<string | null>(null);
	let localIp = $state<string | null>(null);
	onMount(async () => {
		const res = await orchestratorinfo.get();
		if (res.ok) {
			publicIp = res.data.public_ip;
			localIp = res.data.local_ip;
		}
	});

	async function start() {
		if (!server) return;
		busy = 'start';
		actionError = null;
		const res = await servers.start(server.name);
		busy = null;
		if (!res.ok) actionError = `Start failed (HTTP ${res.status})`;
	}
	async function stop() {
		if (!server) return;
		busy = 'stop';
		actionError = null;
		const res = await servers.stop(server.name);
		busy = null;
		if (!res.ok) actionError = `Stop failed (HTTP ${res.status})`;
	}
	async function doDelete() {
		if (!server) return;
		busy = 'delete';
		actionError = null;
		const res = await servers.remove(server.name);
		if (!res.ok) {
			busy = null;
			confirmDelete = false;
			actionError = `Delete failed (HTTP ${res.status})`;
			return;
		}
		busy = null;
		confirmDelete = false;
		goto('/'); // back to the fleet index, which lands on the first server
	}

	const fmtDate = (s: string) => new Date(s).toLocaleString();
</script>

{#snippet field(label: string, value: string, valueClass = 'text-neutral-300')}
	<div class="flex justify-between gap-4 py-1.5 text-sm">
		<span class="shrink-0 text-neutral-500">{label}</span>
		<span class="truncate text-right {valueClass}">{value}</span>
	</div>
{/snippet}

{#snippet actionBtn(label: string, Icon: LucideIcon, tone: 'green' | 'amber' | 'red', onclick: () => void, disabled: boolean)}
	<button
		type="button"
		{onclick}
		{disabled}
		title={label}
		aria-label={label}
		class={[
			'rounded border p-1.5 transition-colors disabled:cursor-not-allowed disabled:opacity-40',
			tone === 'green' && 'border-green-500/40 text-green-400 hover:bg-green-500/10',
			tone === 'amber' && 'border-amber-500/40 text-amber-400 hover:bg-amber-500/10',
			tone === 'red' && 'border-red-500/40 text-red-400 hover:bg-red-500/10'
		]}
	>
		<Icon size={16} />
	</button>
{/snippet}

{#snippet setsBlock(plugins: AssignmentSet | null, configs: AssignmentSet | null, env: EnvVar[] | null, loading: boolean)}
	<AssignmentList title="Plugins" set={plugins} {loading} />
	<AssignmentList title="Configs" set={configs} {loading} />
	<EnvList vars={env} {loading} />
{/snippet}

<div class="mx-auto max-w-5xl p-6">
	{#if server}
		<header class="mb-4 flex flex-wrap items-center gap-3 border-b border-neutral-700 pb-4">
			<div class="min-w-0">
				<h1 class="truncate text-lg font-semibold text-neutral-100">{server.name}</h1>
				{#if server.cluster}
					<a
						href="/clusters/{encodeURIComponent(server.cluster)}"
						class="text-xs text-neutral-500 hover:text-neutral-300"
					>
						member of {server.cluster}
					</a>
				{:else}
					<span class="text-xs text-neutral-500">standalone</span>
				{/if}
			</div>
			<div class="ml-auto flex items-center gap-3">
				<StateBadge state={server.actual_state} />
				{@render actionBtn('Start', Play, 'green', start, !canStart || busy !== null)}
				{@render actionBtn('Stop', Square, 'amber', stop, !canStop || busy !== null)}
				{@render actionBtn('Delete', Trash2, 'red', () => (confirmDelete = true), busy !== null)}
			</div>
		</header>

		{#if actionError}
			<p class="mb-4 rounded border border-red-500/30 bg-red-500/10 px-3 py-2 text-sm text-red-400">
				{actionError}
			</p>
		{/if}
		{#if server.last_error}
			<p class="mb-4 rounded border border-red-500/30 bg-red-500/10 px-3 py-2 text-sm text-red-400">
				{server.last_error}
			</p>
		{/if}

		<div class="grid gap-4 md:grid-cols-2">
			<div class="rounded border border-neutral-700 bg-neutral-800 p-4">
				<h2 class="mb-2 text-xs font-semibold uppercase tracking-wide text-neutral-500">Info</h2>
				<div class="divide-y divide-neutral-800">
					{@render field('Name', server.name)}
					{@render field('IP', server.ip || '—')}
					{@render field('Cluster', server.cluster ?? 'standalone')}
					{@render field('Desired', server.desired_state)}
					{@render field('Actual', server.actual_state)}
					{@render field('Updated', fmtDate(server.updated_at))}
				</div>
				<div class="mt-3 space-y-2 border-t border-neutral-700 pt-3">
					<ConnectBox label="Public" ip={publicIp} port={server.port} />
					<ConnectBox label="Private" ip={localIp} port={server.port} />
				</div>
				<p class="mt-3 text-xs text-neutral-600">
					Name, IP and membership are fixed at creation.
				</p>
			</div>

			{#key server.name}
				<ServerConfigForm {server} />
			{/key}
		</div>

		<div class="mt-4 space-y-4 rounded border border-neutral-700 bg-neutral-800 p-4">
			{#await sets}
				{@render setsBlock(null, null, null, true)}
			{:then s}
				{@render setsBlock(s.plugins, s.configs, s.env, false)}
			{/await}
			<p class="text-xs text-neutral-600">
				Plugins, configs and env are fixed at creation.
			</p>
		</div>
	{:else if connecting}
		<p class="text-neutral-400">Loading…</p>
	{:else}
		<p class="text-neutral-400">
			Server <code class="text-neutral-300">{name}</code> not found.
		</p>
	{/if}
</div>

<ConfirmDialog
	open={confirmDelete}
	title="Delete server"
	message={`Permanently delete “${name}”? This cannot be undone.`}
	confirmLabel="Delete"
	tone="red"
	busy={busy === 'delete'}
	onconfirm={doDelete}
	oncancel={() => (confirmDelete = false)}
/>
