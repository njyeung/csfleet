import type { Server, Cluster, OrchestratorInfo } from './types';

// Live fleet status over Server-Sent Events. The orchestrator emits two event
// types on one connection: `state` — a combined whole-state fleet frame (`servers`,
// the GET /api/servers shape enriched with effective sets, plus `clusters`) pushed
// on every change; and `host` — the orchestrator host's info + live utilization,
// pushed by the background metrics sampler on its own ticker. Both are
// full-replacement (each frame supersedes the last), so consumers always read a
// consistent snapshot. The sidebar/detail views derive from the fleet snapshot;
// the host page reads `host`.
class SSEService {
	snapshot = $state<Server[]>([]); // servers
	clusters = $state<Cluster[]>([]);
	// Reassigned wholesale every sample (never mutated in place), so `raw` avoids
	// needlessly deep-proxying the per-core array each tick.
	host = $state.raw<OrchestratorInfo | null>(null);
	connected = $state(false);
	lastError = $state<string | null>(null);

	#es: EventSource | null = null;
	#path = '/api/events';
	#reconnectTimer: ReturnType<typeof setTimeout> | null = null;

	static readonly RECONNECT_MS = 3000;

	connect(path = '/api/events') {
		this.#path = path;
		if (this.#es) return; // idempotent — survives HMR / double-mount
		const es = new EventSource(path);
		this.#es = es;

		es.addEventListener('open', () => {
			this.connected = true;
			this.lastError = null;
			console.log('[sse] connected to', path);
		});

		es.addEventListener('error', () => {
			// The browser's own EventSource retry is unreliable here: it backs off
			// on its own schedule and gives up entirely if the orchestrator is down
			// at connect time. Drop the socket and poll for a reconnect ourselves.
			this.connected = false;
			this.lastError = 'disconnected (retrying)';
			console.warn(`[sse] connection error — retrying in ${SSEService.RECONNECT_MS}ms`);
			this.#scheduleReconnect();
		});

		es.addEventListener('state', (e) => {
			try {
				const data = JSON.parse((e as MessageEvent).data) as {
					servers: Server[] | null;
					clusters: Cluster[] | null;
				};
				this.snapshot = data.servers ?? [];
				this.clusters = data.clusters ?? [];
			} catch (err) {
				console.error('[sse] failed to parse state frame', err);
			}
		});

		es.addEventListener('host', (e) => {
			try {
				this.host = JSON.parse((e as MessageEvent).data) as OrchestratorInfo;
			} catch (err) {
				console.error('[sse] failed to parse host frame', err);
			}
		});
	}

	#scheduleReconnect() {
		// Tear down the current socket so connect() opens a fresh one, and avoid
		// stacking timers if multiple error events fire.
		this.#es?.close();
		this.#es = null;
		if (this.#reconnectTimer) return;
		this.#reconnectTimer = setTimeout(() => {
			this.#reconnectTimer = null;
			this.connect(this.#path);
		}, SSEService.RECONNECT_MS);
	}

	disconnect() {
		if (this.#reconnectTimer) {
			clearTimeout(this.#reconnectTimer);
			this.#reconnectTimer = null;
		}
		this.#es?.close();
		this.#es = null;
		this.connected = false;
	}
}

// Singleton, imported wherever live status is needed.
export const sse = new SSEService();
