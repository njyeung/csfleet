<script lang="ts">
	import { page } from '$app/state';
	import { fleet, type Aggregate } from '$lib/stores/fleet.svelte';
	import { sse } from '$lib/api/sse.svelte';
	import { newResource } from '$lib/stores/newresource.svelte';
	import StateBadge from '$lib/components/ui/StateBadge.svelte';

	const tree = $derived(fleet.tree);
	const empty = $derived(tree.clusters.length === 0 && tree.standalone.length === 0);
	// Still waiting on the first SSE frame vs. genuinely-empty fleet.
	const connecting = $derived(!sse.connected && sse.snapshot.length === 0 && sse.clusters.length === 0);

	const serverActive = (name: string) => page.url.pathname === `/servers/${encodeURIComponent(name)}`;
	const clusterActive = (name: string) => page.url.pathname === `/clusters/${encodeURIComponent(name)}`;

	const AGG_DOT: Record<Aggregate, string> = {
		all: 'bg-green-500',
		partial: 'bg-amber-500',
		none: 'bg-neutral-600'
	};

	const rowBase = 'flex items-center gap-2 border-l-2 px-3 py-1.5 text-sm hover:bg-neutral-700/50';
	const sel = (active: boolean) =>
		active ? 'border-neutral-400 bg-neutral-700 text-neutral-100' : 'border-transparent text-neutral-300';
</script>

<aside class="flex w-64 shrink-0 flex-col border-r border-neutral-700 bg-neutral-800">
	<div class="px-3 py-2 text-xs font-semibold uppercase tracking-wide text-neutral-500">Fleet</div>

	<nav class="min-h-0 flex-1 overflow-y-auto pb-4">
		{#if empty}
			<p class="px-3 py-2 text-sm text-neutral-500">
				{connecting ? 'Loading…' : 'No servers yet.'}
			</p>
		{/if}

		<!-- Clusters, each with its members nested beneath -->
		{#each tree.clusters as node (node.cluster.name)}
			<div class="group flex items-center {sel(clusterActive(node.cluster.name))} border-l-2 hover:bg-neutral-700/50">
				<a
					href="/clusters/{encodeURIComponent(node.cluster.name)}"
					class="flex min-w-0 flex-1 items-center gap-2 px-3 py-1.5 text-sm"
				>
					<span class="font-mono text-xs text-neutral-500">:{node.cluster.port}</span>
					<span class="truncate">{node.cluster.name}</span>
					<span class="ml-auto flex items-center gap-1 text-xs text-neutral-500">
						{node.running}/{node.total}
						<span class="inline-block h-2 w-2 rounded-full {AGG_DOT[node.agg]}"></span>
					</span>
				</a>
				<button
					type="button"
					title="Add a server to this cluster"
					aria-label="Add a server to {node.cluster.name}"
					onclick={() => newResource.openServerForCluster(node.cluster.name)}
					class="mr-1 shrink-0 rounded px-1.5 text-neutral-500 hover:bg-neutral-600/50 hover:text-neutral-200"
				>
					+
				</button>
			</div>

			{#each node.members as m (m.name)}
				<a href="/servers/{encodeURIComponent(m.name)}" class="{rowBase} pl-7 {sel(serverActive(m.name))}">
					<StateBadge state={m.actual_state} showLabel={false} size="sm" />
					<span class="truncate">{m.name}</span>
				</a>
			{/each}
		{/each}

		<!-- Standalone servers (own external port shown; cluster members don't) -->
		{#each tree.standalone as s (s.name)}
			<a href="/servers/{encodeURIComponent(s.name)}" class="{rowBase} {sel(serverActive(s.name))}">
				<StateBadge state={s.actual_state} showLabel={false} size="sm" />
				<span class="font-mono text-xs text-neutral-500">{s.port == null ? ':—' : `:${s.port}`}</span>
				<span class="truncate">{s.name}</span>
			</a>
		{/each}
	</nav>
</aside>
