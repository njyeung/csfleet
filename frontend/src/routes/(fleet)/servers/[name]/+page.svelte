<script lang="ts">
	import ServerDetail from '$lib/components/ServerDetail.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<ServerDetail name={data.name} sets={data.sets} />
