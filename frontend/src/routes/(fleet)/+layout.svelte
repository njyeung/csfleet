<script lang="ts">
	import Sidebar from '$lib/components/Sidebar.svelte';

	let { children } = $props();
</script>

<div class="flex h-full min-h-0">
	<Sidebar />
	<main class="min-w-0 flex-1 overflow-y-auto">
		{@render children()}
	</main>
</div>
