import { request } from '../client';

// Web UI account as returned by the orchestrator. `seed` marks the in-memory
// admin (from .env), which can't be deleted or have its password reset here.
export interface User {
	username: string;
	created_at: string;
	seed: boolean;
}

// Session auth + user management. The JWT rides an HttpOnly cookie set by the
// API, so there's nothing to store client-side — `me()` reports who, if anyone,
// the current cookie authenticates as.
export const auth = {
	login: (username: string, password: string) =>
		request<{ username: string }>('POST', '/api/auth/login', { body: { username, password } }),
	logout: () => request<null>('POST', '/api/auth/logout'),
	me: () => request<{ username: string }>('GET', '/api/auth/me'),

	listUsers: () => request<User[]>('GET', '/api/users'),
	createUser: (username: string, password: string) =>
		request<null>('POST', '/api/users', { body: { username, password } }),
	deleteUser: (username: string) =>
		request<null>('DELETE', `/api/users/${encodeURIComponent(username)}`),
	setPassword: (username: string, password: string) =>
		request<null>('PUT', `/api/users/${encodeURIComponent(username)}/password`, {
			body: { password }
		})
};
