<script lang="ts">
	import { sse } from '$lib/api/sse.svelte';
	import HostDashboard from '$lib/components/HostDashboard.svelte';

	// The host frame rides the app-wide SSE connection (opened in the root layout
	// and pushed immediately on connect), so there's nothing to fetch here — just
	// render once the first sample has arrived.
</script>

{#if sse.host}
	<HostDashboard info={sse.host} />
{:else}
	<p class="p-6 text-sm text-neutral-500">
		{sse.connected ? 'Loading host metrics…' : 'Disconnected — waiting for the orchestrator…'}
	</p>
{/if}
