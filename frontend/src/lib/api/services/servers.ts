import { request, encodePath } from '../client';
import type {
	Server,
	CreateServerRequest,
	UpdateServerRequest,
	AssignmentSet,
	EnvVar
} from '../types';

// Servers. Plugins/configs/env are baked in at creation, so those endpoints are
// read-only here; change them by recreating the server or editing its cluster.
export const servers = {
	list: () => request<Server[]>('GET', '/api/servers'),
	get: (name: string) => request<Server>('GET', `/api/servers/${encodePath(name)}`),
	create: (body: CreateServerRequest) => request<Server>('POST', '/api/servers', { body }),
	update: (name: string, body: UpdateServerRequest) =>
		request<Server>('PUT', `/api/servers/${encodePath(name)}`, { body }),
	remove: (name: string) => request<null>('DELETE', `/api/servers/${encodePath(name)}`),
	start: (name: string) => request<null>('POST', `/api/servers/${encodePath(name)}/start`),
	stop: (name: string) => request<null>('POST', `/api/servers/${encodePath(name)}/stop`),
	plugins: (name: string) =>
		request<AssignmentSet>('GET', `/api/servers/${encodePath(name)}/plugins`),
	configs: (name: string) =>
		request<AssignmentSet>('GET', `/api/servers/${encodePath(name)}/configs`),
	env: (name: string) => request<EnvVar[]>('GET', `/api/servers/${encodePath(name)}/env`)
};
