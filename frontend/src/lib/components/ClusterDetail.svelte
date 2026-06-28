<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { sse } from '$lib/api/sse.svelte';
	import { fleet, type Aggregate } from '$lib/stores/fleet.svelte';
	import { servers, clusters as clustersApi } from '$lib/api/services';
	import { orchestratorinfo } from '$lib/api/services/orchestratorinfo';
	import type { AssignmentSet, EnvVar } from '$lib/api/types';
	import StateBadge from '$lib/components/ui/StateBadge.svelte';
	import ConfirmDialog from '$lib/components/ui/ConfirmDialog.svelte';
	import AssignmentList from '$lib/components/AssignmentList.svelte';
	import EnvList from '$lib/components/EnvList.svelte';
	import ClusterConfigForm from '$lib/components/ClusterConfigForm.svelte';
	import ConnectBox from '$lib/components/ui/ConnectBox.svelte';
	import { Play, Square, Trash2, type LucideIcon } from '@lucide/svelte';

	type Sets = { plugins: AssignmentSet | null; configs: AssignmentSet | null; env: EnvVar[] | null };

	let { name, sets }: { name: string; sets: Promise<Sets> } = $props();

	// The tree node carries the cluster row + its live members + aggregate.
	const node = $derived(fleet.tree.clusters.find((n) => n.cluster.name === name));
	// Distinguish "first SSE frame hasn't arrived" from "this cluster doesn't exist".
	const connecting = $derived(!sse.connected && sse.clusters.length === 0);

	const AGG_TEXT: Record<Aggregate, string> = {
		all: 'text-green-400',
		partial: 'text-amber-400',
		none: 'text-neutral-400'
	};

	// A cluster has no state of its own — lifecycle fans out across its members.
	const canStart = $derived(!!node && node.total > 0 && node.running < node.total);
	const canStop = $derived(!!node && node.running > 0);

	let busy = $state<null | 'start' | 'stop' | 'delete'>(null);
	let actionError = $state<string | null>(null);
	let confirmDelete = $state(false);

	// Orchestrator host IPs — same for every cluster, fetched once. Public is the
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

	async function startAll() {
		if (!node) return;
		busy = 'start';
		actionError = null;
		const results = await Promise.all(node.members.map((m) => servers.start(m.name)));
		busy = null;
		const failed = results.filter((r) => !r.ok).length;
		if (failed) actionError = `${failed} of ${results.length} servers failed to start.`;
	}
	async function stopAll() {
		if (!node) return;
		busy = 'stop';
		actionError = null;
		const results = await Promise.all(node.members.map((m) => servers.stop(m.name)));
		busy = null;
		const failed = results.filter((r) => !r.ok).length;
		if (failed) actionError = `${failed} of ${results.length} servers failed to stop.`;
	}
	async function doDelete() {
		if (!node) return;
		busy = 'delete';
		actionError = null;
		// The backend blocks deleting a non-empty cluster, so delete members first.
		const memberResults = await Promise.all(node.members.map((m) => servers.remove(m.name)));
		const memberFailed = memberResults.filter((r) => !r.ok).length;
		if (memberFailed) {
			busy = null;
			confirmDelete = false;
			actionError = `Could not delete ${memberFailed} member(s); cluster not removed.`;
			return;
		}
		const res = await clustersApi.remove(node.cluster.name);
		if (!res.ok) {
			busy = null;
			confirmDelete = false;
			actionError = `Cluster delete failed (HTTP ${res.status}).`;
			return;
		}
		// The SSE state frame will drop the deleted cluster + members on its own.
		busy = null;
		confirmDelete = false;
		goto('/');
	}

	const fmtDate = (s: string) => new Date(s).toLocaleString();
</script>

{#snippet field(label: string, value: string)}
	<div class="flex justify-between gap-4 py-1.5 text-sm">
		<span class="shrink-0 text-neutral-500">{label}</span>
		<span class="truncate text-right text-neutral-300">{value}</span>
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
	{#if node}
		{@const c = node.cluster}
		<header class="mb-4 flex flex-wrap items-center gap-3 border-b border-neutral-700 pb-4">
			<div class="min-w-0">
				<h1 class="truncate text-lg font-semibold text-neutral-100">{c.name}</h1>
				<span class="text-xs text-neutral-500">
					cluster · {node.total} member{node.total === 1 ? '' : 's'}
				</span>
			</div>
			<div class="ml-auto flex items-center gap-3">
				<span class="text-sm {AGG_TEXT[node.agg]}">{node.running}/{node.total} running</span>
				{@render actionBtn('Start', Play, 'green', startAll, !canStart || busy !== null)}
				{@render actionBtn('Stop', Square, 'amber', stopAll, !canStop || busy !== null)}
				{@render actionBtn('Delete', Trash2, 'red', () => (confirmDelete = true), busy !== null)}
			</div>
		</header>

		{#if actionError}
			<p class="mb-4 rounded border border-red-500/30 bg-red-500/10 px-3 py-2 text-sm text-red-400">
				{actionError}
			</p>
		{/if}

		<div class="grid gap-4 md:grid-cols-2">
			<div class="rounded border border-neutral-700 bg-neutral-800 p-4">
				<h2 class="mb-2 text-xs font-semibold uppercase tracking-wide text-neutral-500">Info</h2>
				<div class="divide-y divide-neutral-800">
					{@render field('Name', c.name)}
					{@render field('Members', String(node.total))}
					{@render field('Updated', fmtDate(c.updated_at))}
				</div>
				<div class="mt-3 space-y-2 border-t border-neutral-700 pt-3">
					<ConnectBox label="Public" ip={publicIp} port={c.port} />
					<ConnectBox label="Private" ip={localIp} port={c.port} />
				</div>
				<p class="mt-3 text-xs text-neutral-600">
					Name, plugins, configs and env are fixed at creation.
				</p>
			</div>

			{#key c.name}
				<ClusterConfigForm cluster={c} />
			{/key}
		</div>

		<div class="mt-4 rounded border border-neutral-700 bg-neutral-800 p-4">
			<h2 class="mb-2 text-xs font-semibold uppercase tracking-wide text-neutral-500">Members</h2>
			{#if node.members.length === 0}
				<p class="text-sm text-neutral-500">No members yet.</p>
			{:else}
				<ul class="divide-y divide-neutral-800">
					{#each node.members as m (m.name)}
						<li>
							<a
								href="/servers/{encodeURIComponent(m.name)}"
								class="flex items-center gap-2 py-1.5 text-sm hover:text-neutral-100"
							>
								<StateBadge state={m.actual_state} showLabel={false} size="sm" />
								<span class="truncate">{m.name}</span>
								<span class="ml-auto text-xs text-neutral-500">{m.actual_state}</span>
							</a>
						</li>
					{/each}
				</ul>
			{/if}
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
			Cluster <code class="text-neutral-300">{name}</code> not found.
		</p>
	{/if}
</div>

<ConfirmDialog
	open={confirmDelete}
	title="Delete cluster"
	message={node
		? `This removes ${node.total} server${node.total === 1 ? '' : 's'} and the cluster “${name}”. Are you sure?`
		: ''}
	confirmLabel="Delete all"
	tone="red"
	busy={busy === 'delete'}
	onconfirm={doDelete}
	oncancel={() => (confirmDelete = false)}
/>
