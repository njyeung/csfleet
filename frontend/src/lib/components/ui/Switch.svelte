<script lang="ts">
	// Neutral on/off toggle. Per DESIGN only state/lifecycle gets color, so the
	// switch stays grayscale — the knob position is the signal.
	let {
		checked = $bindable(false),
		disabled = false,
		label,
		title,
		onchange
	}: {
		checked?: boolean;
		disabled?: boolean;
		label?: string;
		title?: string;
		// Controlled use: when provided, the toggle reports the next value instead of
		// mutating `checked` itself (so a parent can own a richer value, e.g. tri-state).
		onchange?: (checked: boolean) => void;
	} = $props();

	function toggle() {
		if (onchange) onchange(!checked);
		else checked = !checked;
	}
</script>

<button
	type="button"
	role="switch"
	aria-checked={checked}
	aria-label={label}
	{title}
	{disabled}
	onclick={toggle}
	class={[
		'relative inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full transition-colors',
		'disabled:cursor-not-allowed disabled:opacity-40',
		checked ? 'bg-neutral-500' : 'bg-neutral-700'
	]}
>
	<span
		class={[
			'inline-block h-3.5 w-3.5 rounded-full bg-neutral-100 transition-transform',
			checked ? 'translate-x-5' : 'translate-x-0.5'
		]}
	></span>
</button>
