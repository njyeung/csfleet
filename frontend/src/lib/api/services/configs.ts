import { request, encodePath } from '../client';
import type { ConfigFile } from '../types';

// Config file catalog. A config is a (name, filename, content) tuple: `name` is
// the catalog identifier (the PK assignments reference; may contain slashes, so
// encodePath preserves them in the route), `filename` is the file's path under
// game/csgo/cfg/ that the content is written to.
export const configs = {
	list: () => request<ConfigFile[]>('GET', '/api/configs'),
	get: (name: string) => request<ConfigFile>('GET', `/api/configs/${encodePath(name)}`),
	put: (name: string, filename: string, content: string) =>
		request<null>('PUT', `/api/configs/${encodePath(name)}`, { body: { filename, content } }),
	remove: (name: string) => request<null>('DELETE', `/api/configs/${encodePath(name)}`)
};
