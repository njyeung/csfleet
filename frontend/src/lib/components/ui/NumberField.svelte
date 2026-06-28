<script lang="ts">
	// Styled integer input (e.g. a server/cluster port). Binds a `number | null`;
	// an empty field is `null` so callers can flag "must set".
	let {
		value = $bindable(),
		min,
		max,
		placeholder,
		disabled = false,
		invalid = false,
		id
	}: {
		value: number | null;
		min?: number;
		max?: number;
		placeholder?: string;
		disabled?: boolean;
		invalid?: boolean;
		id?: string;
	} = $props();
</script>

<input
	{id}
	type="number"
	{min}
	{max}
	{placeholder}
	{disabled}
	bind:value
	class={[
		'w-28 rounded border bg-neutral-900 px-2 py-1 text-sm text-neutral-200 outline-none',
		'focus:border-neutral-500 disabled:cursor-not-allowed disabled:opacity-50',
		invalid ? 'border-red-500/60' : 'border-neutral-700'
	]}
/>
