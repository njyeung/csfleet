<script lang="ts">
	// Hour-limit control (restart_after_hrs / stop_after_hrs). Fully controlled off
	// the wire value so Reset just works — no internal state to resync:
	//   value >= 0  → that many hours
	//   value < 0   → "no limit"
	//   value null  → "inherit from cluster" (members) / "no limit" (standalone+cluster)
	// See DESIGN: NULL = inherit, < 0 = no limit.
	let {
		value = $bindable(),
		allowInherit = false
	}: {
		value: number | null;
		allowInherit?: boolean;
	} = $props();

	type Mode = 'inherit' | 'nolimit' | 'value';
	const mode = $derived<Mode>(
		value == null ? (allowInherit ? 'inherit' : 'nolimit') : value < 0 ? 'nolimit' : 'value'
	);

	function onNum(e: Event & { currentTarget: HTMLInputElement }) {
		const n = e.currentTarget.valueAsNumber;
		value = Number.isNaN(n) ? 0 : Math.max(0, Math.trunc(n));
	}
</script>

<div class="flex flex-wrap items-center justify-end gap-x-4 gap-y-1.5">
	<div class="flex items-center gap-1.5">
		<input
			type="number"
			min="0"
			disabled={mode !== 'value'}
			value={mode === 'value' ? value : ''}
			oninput={onNum}
			class={[
				'w-20 rounded border border-neutral-700 bg-neutral-900 px-2 py-1 text-sm text-neutral-200 outline-none',
				'focus:border-neutral-500 disabled:cursor-not-allowed disabled:opacity-50'
			]}
		/>
		<span class="text-xs text-neutral-500">hours</span>
	</div>

	<label class="flex items-center gap-1.5 text-xs text-neutral-400">
		<input
			type="checkbox"
			checked={mode === 'nolimit'}
			disabled={mode === 'inherit'}
			onchange={(e) => (value = e.currentTarget.checked ? -1 : 0)}
			class="h-3.5 w-3.5 rounded border-neutral-600 bg-neutral-900 text-neutral-400 focus:ring-0 focus:ring-offset-0 disabled:opacity-50"
		/>
		No limit
	</label>

	{#if allowInherit}
		<label class="flex items-center gap-1.5 text-xs text-neutral-400">
			<input
				type="checkbox"
				checked={mode === 'inherit'}
				onchange={(e) => (value = e.currentTarget.checked ? null : -1)}
				class="h-3.5 w-3.5 rounded border-neutral-600 bg-neutral-900 text-neutral-400 focus:ring-0 focus:ring-offset-0"
			/>
			Inherit from cluster
		</label>
	{/if}
</div>
