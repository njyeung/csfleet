<script lang="ts">
	import { goto } from '$app/navigation';
	import { sse } from '$lib/api/sse.svelte';
	import { fleet } from '$lib/stores/fleet.svelte';

	// The fleet index has no detail of its own
	// defaults to the first server. 
	// Once the live snapshot has one, redirect to it (replaceState so Back
	// Empty fleets get a prompt instead.
	const first = $derived(fleet.tree.standalone[0] ?? fleet.tree.clusters.flatMap((c) => c.members)[0]);
	const firstHref = $derived(first ? `/servers/${encodeURIComponent(first.name)}` : null);

	// the first server only exists once the SSE snapshot streams in (after load).
	$effect(() => {
		if (firstHref) goto(firstHref, { replaceState: true });
	});

</script>

{#if first}
	<!-- redirecting to the first server -->
{:else}
	<div class="flex h-full items-center justify-center p-8 text-center">
		<div>
			{#if !sse.connected && sse.snapshot.length === 0}
				<p class="text-neutral-400">Connecting to the fleet…</p>
			{:else}
				<p class="text-neutral-300">No servers yet.</p>
				<p class="mt-1 text-sm text-neutral-500">
					Create one with <span class="text-neutral-400">+ New</span>
				</p>
			{/if}
		</div>
	</div>
{/if}
