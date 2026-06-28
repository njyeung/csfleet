<script lang="ts" module>
	// actual_state collapses to pending|starting|running|stopping|stopped|crashed: running=green, crashed=red,
	// everything transitional/stopped=amber.
	type Tone = 'green' | 'amber' | 'red';

	export function stateTone(state: string): Tone {
		if (state === 'running') return 'green';
		if (state === 'crashed') return 'red';
		return 'amber';
	}

	const DOT: Record<Tone, string> = {
		green: 'bg-green-500',
		amber: 'bg-amber-500',
		red: 'bg-red-500'
	};
	const TEXT: Record<Tone, string> = {
		green: 'text-green-400',
		amber: 'text-amber-400',
		red: 'text-red-400'
	};
</script>

<script lang="ts">
	let {
		state,
		showLabel = true,
		size = 'md'
	}: { state: string; showLabel?: boolean; size?: 'sm' | 'md' } = $props();

	const tone = $derived(stateTone(state));
</script>

<span class="inline-flex items-center gap-1.5">
	<span
		class="inline-block shrink-0 rounded-full {size === 'sm' ? 'h-2 w-2' : 'h-2.5 w-2.5'} {DOT[
			tone
		]}"
	></span>
	{#if showLabel}
		<span class="text-sm {TEXT[tone]}">{state}</span>
	{/if}
</span>
