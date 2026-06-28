<script lang="ts">
	import Switch from './Switch.svelte';

	// On/off with an optional "inherit from cluster" state for member servers.
	// Fully controlled off the wire value so Reset just works — mirrors HoursField:
	//   null  → inherit (members; resolves to the cluster's value at runtime)
	//           / default (standalone, where the backend default applies)
	//   true  → explicitly on
	//   false → explicitly off
	// See resolveBool (orchestration/database/resolve.go): nil inherits, a concrete
	// bool overrides.
	let {
		value = $bindable(),
		allowInherit = false,
		label
	}: {
		value: boolean | null;
		allowInherit?: boolean;
		label?: string;
	} = $props();

	const inherit = $derived(allowInherit && value === null);
	// When not inheriting, show the explicit value; a standalone null falls back to
	// the on default so the toggle isn't misleadingly off.
	const checked = $derived(value === true || (value === null && !allowInherit));
</script>

<div class="flex flex-wrap items-center justify-end gap-x-4 gap-y-1.5">
	<Switch {checked} disabled={inherit} {label} onchange={(c) => (value = c)} />
	{#if allowInherit}
		<label class="flex items-center gap-1.5 text-xs text-neutral-400">
			<input
				type="checkbox"
				checked={inherit}
				onchange={(e) => (value = e.currentTarget.checked ? null : true)}
				class="h-3.5 w-3.5 rounded border-neutral-600 bg-neutral-900 text-neutral-400 focus:ring-0 focus:ring-offset-0"
			/>
			Inherit from cluster
		</label>
	{/if}
</div>
