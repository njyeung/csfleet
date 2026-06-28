import type { PageLoad } from './$types';
import { clusters } from '$lib/api/services';

// Read-only assignment/env sets for the cluster panel, streamed like
// the server route. The cluster row + live members come from the fleet/clusters
// stores; only these create-only sets are fetched here.
export const load: PageLoad = ({ params }) => {
	const name = params.name;
	return {
		name,
		sets: Promise.all([clusters.plugins(name), clusters.configs(name), clusters.env(name)]).then(
			([p, c, e]) => ({
				plugins: p.ok ? p.data : null,
				configs: c.ok ? c.data : null,
				env: e.ok ? e.data : null
			})
		)
	};
};
