import { request, encodePath } from '../client';
import type {
	Cluster,
	CreateClusterRequest,
	UpdateClusterRequest,
	AssignmentSet,
	EnvVar
} from '../types';

// Clusters. Like servers, plugins/configs/env are fixed at creation and read-only
// here; members inherit them unless they override at their own creation.
export const clusters = {
	list: () => request<Cluster[]>('GET', '/api/clusters'),
	get: (name: string) => request<Cluster>('GET', `/api/clusters/${encodePath(name)}`),
	create: (body: CreateClusterRequest) => request<Cluster>('POST', '/api/clusters', { body }),
	update: (name: string, body: UpdateClusterRequest) =>
		request<Cluster>('PUT', `/api/clusters/${encodePath(name)}`, { body }),
	remove: (name: string) => request<null>('DELETE', `/api/clusters/${encodePath(name)}`),
	plugins: (name: string) =>
		request<AssignmentSet>('GET', `/api/clusters/${encodePath(name)}/plugins`),
	configs: (name: string) =>
		request<AssignmentSet>('GET', `/api/clusters/${encodePath(name)}/configs`),
	env: (name: string) => request<EnvVar[]>('GET', `/api/clusters/${encodePath(name)}/env`)
};
