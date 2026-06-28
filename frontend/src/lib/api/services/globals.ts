import { request } from '../client';
import type { AssignmentSet } from '../types';

// Global-scope plugin/config assignment — the base of the
// global < cluster < server inheritance chain and the only editable assignment
// tier. An empty items list is an explicit "none" that members can still override.
export const globals = {
	getPlugins: () => request<AssignmentSet>('GET', '/api/global/plugins'),
	setPlugins: (items: string[]) => request<null>('PUT', '/api/global/plugins', { body: { items } }),
	getConfigs: () => request<AssignmentSet>('GET', '/api/global/configs'),
	setConfigs: (items: string[]) => request<null>('PUT', '/api/global/configs', { body: { items } })
};
