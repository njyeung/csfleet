// The sidebar tree: live servers grouped under their clusters, plus standalone
// servers. Both come from the same SSE state frame, so this is a pure derivation
// that fetches nothing of its own.
import { sse } from '$lib/api/sse.svelte';
import type { Cluster, Server } from '$lib/api/types';

export type Aggregate = 'all' | 'partial' | 'none';

export interface ClusterNode {
	cluster: Cluster;
	members: Server[];
	running: number;
	total: number;
	agg: Aggregate;
}

export interface FleetTree {
	clusters: ClusterNode[];
	standalone: Server[];
}

const byName = (a: { name: string }, b: { name: string }) => a.name.localeCompare(b.name);

class FleetStore {
	tree = $derived.by<FleetTree>(() => {
		const byCluster = new Map<string, Server[]>();
		const standalone: Server[] = [];
		for (const s of sse.snapshot) {
			if (s.cluster) {
				const arr = byCluster.get(s.cluster);
				if (arr) arr.push(s);
				else byCluster.set(s.cluster, [s]);
			} else {
				standalone.push(s);
			}
		}

		const clusters = sse.clusters
			.map((cluster): ClusterNode => {
				const members = (byCluster.get(cluster.name) ?? []).slice().sort(byName);
				const total = members.length;
				const running = members.filter((m) => m.actual_state === 'running').length;
				// Gray when nothing's up (incl. empty), green when all, amber otherwise.
				const agg: Aggregate = running === 0 ? 'none' : running === total ? 'all' : 'partial';
				return { cluster, members, running, total, agg };
			})
			.sort((a, b) => byName(a.cluster, b.cluster));

		return { clusters, standalone: standalone.slice().sort(byName) };
	});

	// The live snapshot for one server, or undefined once it's gone.
	server(name: string): Server | undefined {
		return sse.snapshot.find((s) => s.name === name);
	}
}

export const fleet = new FleetStore();
