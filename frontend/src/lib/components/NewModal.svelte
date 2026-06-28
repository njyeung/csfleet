<script lang="ts">
	import { newResource } from '$lib/stores/newresource.svelte';
	import Modal from './ui/Modal.svelte';
	import NewServerForm from './NewServerForm.svelte';
	import NewClusterForm from './NewClusterForm.svelte';

	const title = $derived(
		newResource.mode === 'server'
			? 'New server'
			: newResource.mode === 'cluster'
				? 'New cluster'
				: 'Create'
	);

	const close = () => newResource.close();
</script>

<Modal open={newResource.open} {title} size="lg" onclose={close}>
	{#if newResource.mode === 'choose'}
		<div class="grid grid-cols-2 gap-3">
			<button
				type="button"
				onclick={() => (newResource.mode = 'server')}
				class="rounded-lg border border-neutral-700 p-4 text-left hover:border-neutral-500 hover:bg-neutral-700/30"
			>
				<div class="text-sm font-medium text-neutral-100">Server</div>
				<p class="mt-1 text-xs text-neutral-500">A standalone server or a member of a cluster.</p>
			</button>
			<button
				type="button"
				onclick={() => (newResource.mode = 'cluster')}
				class="rounded-lg border border-neutral-700 p-4 text-left hover:border-neutral-500 hover:bg-neutral-700/30"
			>
				<div class="text-sm font-medium text-neutral-100">Cluster</div>
				<p class="mt-1 text-xs text-neutral-500">A shared ingress port load-balancing its members.</p>
			</button>
		</div>
	{:else if newResource.mode === 'server'}
		<NewServerForm lockedCluster={newResource.lockedCluster} onclose={close} />
	{:else}
		<NewClusterForm onclose={close} />
	{/if}
</Modal>
