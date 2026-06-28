import { request } from '../client';
import type { EnvVar, SetEnvRequest, EnvScope } from '../types';

// Env variables. GET reads any scope for inspection; writes are restricted to
// global scope (cluster/server env is set at the resource's creation).
export const env = {
	list: (scope: EnvScope = 'global', scopeName = '') =>
		request<EnvVar[]>('GET', '/api/env', { query: { scope, scope_name: scopeName } }),
	set: (body: SetEnvRequest) => request<null>('PUT', '/api/env', { body }),
	remove: (key: string, scope: EnvScope = 'global', scopeName = '') =>
		request<null>('DELETE', '/api/env', { query: { key, scope, scope_name: scopeName } })
};
