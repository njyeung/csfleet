import type { PageLoad } from './$types';
import { servers } from '$lib/api/services';

// The read-only assignment/env sets for the panel: 3 GETs per
// selection. Returned as a nested promise so it streams — selecting a server is
// instant and the sets resolve in-panel — and the fetch lives in load, not a
// component $effect. Live server state still comes from the SSE store.
export const load: PageLoad = ({ params }) => {
	const name = params.name;
	return {
		name,
		sets: Promise.all([servers.plugins(name), servers.configs(name), servers.env(name)]).then(
			([p, c, e]) => ({
				plugins: p.ok ? p.data : null,
				configs: c.ok ? c.data : null,
				env: e.ok ? e.data : null
			})
		)
	};
};
