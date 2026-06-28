<script lang="ts">
	import type { OrchestratorInfo } from '$lib/api/types';

	let { info }: { info: OrchestratorInfo } = $props();

	const h = $derived(info.host_stats);
	const memPct = $derived(info.mem_total_bytes ? (h.mem_used_bytes / info.mem_total_bytes) * 100 : 0);
	const diskPct = $derived(h.disk_total_bytes ? (h.disk_used_bytes / h.disk_total_bytes) * 100 : 0);

	// SampledAt is a zero time until the first sample lands; show nothing then.
	const updatedAt = $derived.by(() => {
		const t = new Date(h.sampled_at);
		return t.getFullYear() > 1 ? t.toLocaleTimeString() : null;
	});

	function fmtBytes(n: number): string {
		if (!n) return '0 B';
		const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
		let v = n;
		let i = 0;
		while (v >= 1024 && i < units.length - 1) {
			v /= 1024;
			i++;
		}
		return `${v.toFixed(i === 0 || v >= 100 ? 0 : 1)} ${units[i]}`;
	}

	function fmtUptime(s: number): string {
		const d = Math.floor(s / 86400);
		const hrs = Math.floor((s % 86400) / 3600);
		const m = Math.floor((s % 3600) / 60);
		const parts: string[] = [];
		if (d) parts.push(`${d}d`);
		if (d || hrs) parts.push(`${hrs}h`);
		parts.push(`${m}m`);
		return parts.join(' ');
	}

	const clamp = (pct: number) => Math.min(100, Math.max(0, pct));
</script>

{#snippet meter(pct: number)}
	<div class="h-2 w-full overflow-hidden rounded-full bg-neutral-800">
		<div class="h-full rounded-full bg-neutral-300 transition-[width]" style="width: {clamp(pct)}%"></div>
	</div>
{/snippet}

{#snippet fact(label: string, value: string)}
	<div class="flex justify-between gap-4 py-1.5 text-sm">
		<span class="shrink-0 text-neutral-500">{label}</span>
		<span class="truncate text-right text-neutral-300">{value}</span>
	</div>
{/snippet}

<div class="h-full overflow-y-auto">
	<div class="mx-auto max-w-5xl space-y-4 p-6">
		<header class="flex flex-wrap items-end justify-between gap-2 border-b border-neutral-700 pb-4">
			<div class="min-w-0">
				<h1 class="truncate text-lg font-semibold text-neutral-100">{info.hostname || 'Host'}</h1>
				<p class="truncate text-xs text-neutral-500">{info.cpu_model || 'Orchestrator host'}</p>
			</div>
			<span class="text-xs text-neutral-500">
				{#if updatedAt}Updated {updatedAt}{:else}Awaiting first sample…{/if}
			</span>
		</header>

		<!-- Identity / hardware (static) -->
		<div class="rounded border border-neutral-700 bg-neutral-800 p-4">
			<h2 class="mb-2 text-xs font-semibold uppercase tracking-wide text-neutral-500">System</h2>
			<div class="grid gap-x-8 sm:grid-cols-2">
				<div class="divide-y divide-neutral-800">
					{@render fact('Hostname', info.hostname || '—')}
					{@render fact('CPU', info.cpu_model || '—')}
					{@render fact('Cores', String(info.cpu_cores))}
				</div>
				<div class="divide-y divide-neutral-800">
					{@render fact('Memory', fmtBytes(info.mem_total_bytes))}
					{@render fact('Local IP', info.local_ip || '—')}
					{@render fact('Public IP', info.public_ip || '—')}
				</div>
			</div>
		</div>

		<!-- CPU (live) -->
		<div class="rounded border border-neutral-700 bg-neutral-800 p-4">
			<div class="mb-3 flex items-baseline justify-between">
				<h2 class="text-xs font-semibold uppercase tracking-wide text-neutral-500">CPU</h2>
				<div class="flex items-baseline gap-3 text-xs text-neutral-500">
					<span>load {h.load_avg.map((l) => l.toFixed(2)).join(' · ')}</span>
					<span>up {fmtUptime(h.uptime_seconds)}</span>
				</div>
			</div>

			<div class="mb-4 flex items-center gap-3">
				<span class="w-14 shrink-0 text-2xl font-semibold tabular-nums text-neutral-100">
					{h.cpu_percent.toFixed(0)}%
				</span>
				<div class="flex-1">{@render meter(h.cpu_percent)}</div>
			</div>

			{#if h.per_core_percent.length}
				<div class="grid grid-cols-2 gap-x-4 gap-y-2 sm:grid-cols-4 lg:grid-cols-8">
					{#each h.per_core_percent as pct, i (i)}
						<div>
							<div class="mb-0.5 flex justify-between text-[10px] tabular-nums text-neutral-500">
								<span>c{i}</span><span>{pct.toFixed(0)}</span>
							</div>
							{@render meter(pct)}
						</div>
					{/each}
				</div>
			{/if}
		</div>

		<!-- Memory + Disk (live) -->
		<div class="grid gap-4 md:grid-cols-2">
			<div class="rounded border border-neutral-700 bg-neutral-800 p-4">
				<div class="mb-3 flex items-baseline justify-between">
					<h2 class="text-xs font-semibold uppercase tracking-wide text-neutral-500">Memory</h2>
					<span class="text-sm tabular-nums text-neutral-300">{memPct.toFixed(0)}%</span>
				</div>
				{@render meter(memPct)}
				<div class="mt-2 divide-y divide-neutral-800">
					{@render fact('Used', `${fmtBytes(h.mem_used_bytes)} of ${fmtBytes(info.mem_total_bytes)}`)}
					{@render fact('Available', fmtBytes(h.mem_available_bytes))}
					{@render fact('Swap used', fmtBytes(h.swap_used_bytes))}
				</div>
			</div>

			<div class="rounded border border-neutral-700 bg-neutral-800 p-4">
				<div class="mb-3 flex items-baseline justify-between">
					<h2 class="text-xs font-semibold uppercase tracking-wide text-neutral-500">Disk</h2>
					<span class="text-sm tabular-nums text-neutral-300">{diskPct.toFixed(0)}%</span>
				</div>
				{@render meter(diskPct)}
				<div class="mt-2 divide-y divide-neutral-800">
					{@render fact('Used', `${fmtBytes(h.disk_used_bytes)} of ${fmtBytes(h.disk_total_bytes)}`)}
					{@render fact('Free', fmtBytes(Math.max(0, h.disk_total_bytes - h.disk_used_bytes)))}
				</div>
			</div>
		</div>
	</div>
</div>
