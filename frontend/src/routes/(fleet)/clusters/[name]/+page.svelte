<script lang="ts">
	import ClusterDetail from '$lib/components/ClusterDetail.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<ClusterDetail name={data.name} sets={data.sets} />
