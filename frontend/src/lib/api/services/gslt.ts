import { request } from '../client';

// GSLT token pool. Tokens are returned as a flat list of strings.
export const gslt = {
	list: () => request<string[]>('GET', '/api/gslt-tokens'),
	add: (token: string) => request<null>('POST', '/api/gslt-tokens', { body: { token } }),
	remove: (token: string) => request<null>('DELETE', '/api/gslt-tokens', { query: { token } })
};
