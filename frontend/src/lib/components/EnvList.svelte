<script lang="ts">
	import type { EnvVar } from '$lib/api/types';

	// Read-only resolved env for a server/cluster (GET .../env): the overlay of
	// global ◂ cluster ◂ server, one row per resolved key with its source scope.
	// Per-instance env is create-only, so this is display-only here.
	let { vars, loading = false }: { vars: EnvVar[] | null; loading?: boolean } = $props();

	// Credentials seeded into global env. Match on the bare key.
	const SECRET = /(^|\.)(pass|rootpass|password|secret|token)$/i;
	const isSecret = (key: string) => SECRET.test(key);

	const scopeLabel = (v: EnvVar) => (v.scope_name ? `${v.scope}:${v.scope_name}` : v.scope);
</script>

<section>
	<h3 class="mb-2 text-xs font-semibold uppercase tracking-wide text-neutral-500">Environment</h3>

	{#if loading}
		<p class="text-sm text-neutral-500">Loading…</p>
	{:else if !vars}
		<p class="text-sm text-neutral-500">—</p>
	{:else if vars.length === 0}
		<p class="text-sm text-neutral-500">none</p>
	{:else}
		<div class="divide-y divide-neutral-800 rounded border border-neutral-800">
			{#each vars as v (v.scope + ':' + v.scope_name + ':' + v.key)}
				<div class="flex items-center gap-3 px-2 py-1.5 text-sm">
					<code class="shrink-0 text-neutral-300">{v.key}</code>
					<span class="min-w-0 flex-1 truncate font-mono text-xs text-neutral-400">
						{isSecret(v.key) ? '••••••••' : v.value}
					</span>
					<span class="shrink-0 rounded bg-neutral-800 px-1.5 py-0.5 text-[10px] uppercase text-neutral-500">
						{scopeLabel(v)}
					</span>
				</div>
			{/each}
		</div>
	{/if}
</section>
