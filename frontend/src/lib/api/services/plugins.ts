import { request, encodePath } from '../client';
import type { Manifest } from '../types';

// Plugin manifest catalog. Editing a manifest doesn't touch running servers; the
// new definition is picked up the next time a server using it starts.
export const plugins = {
	list: () => request<Manifest[]>('GET', '/api/plugins'),
	get: (name: string) => request<Manifest>('GET', `/api/plugins/${encodePath(name)}`),
	put: (name: string, manifest: string) =>
		request<null>('PUT', `/api/plugins/${encodePath(name)}`, { body: { manifest } }),
	remove: (name: string) => request<null>('DELETE', `/api/plugins/${encodePath(name)}`)
};
