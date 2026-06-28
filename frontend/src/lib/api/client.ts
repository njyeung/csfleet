// Low-level HTTP transport for the orchestrator control plane. Every service
// module builds on request(). Paths are relative ("/api/..."): the dev server
// proxies them to the Go API (vite.config.ts), and in production the same-origin
// host serves both. No base URL needed.
//
// request() never throws on HTTP status — it returns the parsed body alongside
// the status so callers (and the API console) can render 4xx/5xx error payloads.

export interface ApiResponse<T = unknown> {
	status: number;
	ok: boolean; // status < 400
	data: T; // parsed JSON, the raw string for non-JSON, or null for empty (204) bodies
}

export interface RequestOptions {
	query?: Record<string, string | number | boolean | undefined>;
	body?: unknown; // JSON-encoded when present
	signal?: AbortSignal;
}

// encodePath percent-encodes each path segment but preserves slashes, so config
// names like "cfg/server.cfg" survive into the {name...} route intact.
export function encodePath(name: string): string {
	return name.split('/').map(encodeURIComponent).join('/');
}

// A 401 on anything but the auth endpoints means the session cookie expired or
// was never set. The app registers a handler here (clear store + redirect to
// /login) rather than importing it directly, keeping this module dependency-free.
let onUnauthorized: (() => void) | null = null;
export function setUnauthorizedHandler(fn: () => void) {
	onUnauthorized = fn;
}

function withQuery(path: string, query?: RequestOptions['query']): string {
	if (!query) return path;
	const qs = new URLSearchParams();
	for (const [k, v] of Object.entries(query)) {
		if (v !== undefined) qs.set(k, String(v));
	}
	const s = qs.toString();
	return s ? `${path}?${s}` : path;
}

export async function request<T = unknown>(
	method: string,
	path: string,
	opts: RequestOptions = {}
): Promise<ApiResponse<T>> {
	const headers: Record<string, string> = {};
	let payload: string | undefined;
	if (opts.body !== undefined) {
		headers['Content-Type'] = 'application/json';
		payload = JSON.stringify(opts.body);
	}

	const res = await fetch(withQuery(path, opts.query), {
		method,
		headers,
		body: payload,
		signal: opts.signal
	});

	const text = await res.text();
	let data: unknown = null;
	if (text) {
		try {
			data = JSON.parse(text);
		} catch {
			data = text; // non-JSON body (shouldn't happen, but surface it verbatim)
		}
	}

	if (res.status === 401 && !path.startsWith('/api/auth/')) onUnauthorized?.();

	return { status: res.status, ok: res.status < 400, data: data as T };
}
