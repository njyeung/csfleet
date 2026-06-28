<script lang="ts">
	import Switch from './ui/Switch.svelte';

	// GSLT token controls — a server-only pairing of an explicit
	// SRCDS_TOKEN and the auto_token switch:
	//   token set   → auto_token locked OFF (use the pinned token)
	//   token empty + on  → claim a free token from the pool
	//   token empty + off → run tokenless
	// An explicit token always wins (worker.go), so filling it locks the switch.
	let {
		token = $bindable(''),
		autoToken = $bindable(false)
	}: {
		token?: string;
		autoToken?: boolean;
	} = $props();

	const hasToken = $derived(token.trim() !== '');
	// While a token is pinned, the switch reads off regardless of the stored value.
	const switchChecked = $derived(hasToken ? false : autoToken);

	function onToken(e: Event & { currentTarget: HTMLInputElement }) {
		token = e.currentTarget.value;
		// Pinning a token forces auto_token off so the wire value can't contradict it.
		if (token.trim() !== '') autoToken = false;
	}
</script>

<div class="space-y-2">
	<div class="flex items-center justify-between gap-4">
		<span class="text-sm text-neutral-400">Auto token</span>
		<Switch
			checked={switchChecked}
			disabled={hasToken}
			label="Auto token"
			title={hasToken ? 'Locked off while SRCDS_TOKEN is set' : undefined}
			onchange={(c) => (autoToken = c)}
		/>
	</div>

	<div class="flex items-center justify-between gap-4">
		<label for="srcds-token" class="text-sm text-neutral-400">SRCDS_TOKEN</label>
		<input
			id="srcds-token"
			type="text"
			value={token}
			oninput={onToken}
			placeholder="Pin a GSLT (optional)"
			spellcheck="false"
			autocomplete="off"
			class="w-56 rounded border border-neutral-700 bg-neutral-900 px-2 py-1 font-mono text-sm text-neutral-200 outline-none focus:border-neutral-500"
		/>
	</div>
</div>
