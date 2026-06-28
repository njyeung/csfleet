import { auth as authApi } from '$lib/api/services/auth';

// Session state for the whole app. `ready` flips true once the initial cookie
// check (`me()`) resolves — the root layout shows a splash until then so guarded
// routes never flash before we know who's logged in.
class AuthStore {
	user = $state<{ username: string } | null>(null);
	ready = $state(false);

	// Resolve the current cookie into a user (or null). Runs once on app start;
	// idempotent so repeated mounts don't re-fetch.
	async init() {
		if (this.ready) return;
		const res = await authApi.me();
		this.user = res.ok ? { username: res.data.username } : null;
		this.ready = true;
	}

	async login(username: string, password: string) {
		const res = await authApi.login(username, password);
		if (res.ok) this.user = { username: res.data.username };
		return res;
	}

	async logout() {
		await authApi.logout();
		this.user = null;
	}

	// Drop the session locally without a round-trip — used when the API rejects a
	// request with 401 (expired/invalid cookie).
	clear() {
		this.user = null;
	}
}

export const auth = new AuthStore();
