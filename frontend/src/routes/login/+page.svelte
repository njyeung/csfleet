<script lang="ts">
	import { goto } from '$app/navigation';
	import { auth } from '$lib/stores/auth.svelte';

	let username = $state('');
	let password = $state('');
	let submitting = $state(false);
	let error = $state<string | null>(null);

	const canSubmit = $derived(!submitting && username.trim() !== '' && password !== '');

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!canSubmit) return;
		submitting = true;
		error = null;

		const res = await auth.login(username.trim(), password);
		submitting = false;
		if (res.ok) {
			goto('/');
		} else {
			error = res.status === 401 ? 'Invalid username or password.' : `Login failed (HTTP ${res.status})`;
			password = '';
		}
	}
</script>

<div class="flex h-screen items-center justify-center bg-neutral-900 text-neutral-300">
	<form
		onsubmit={submit}
		class="w-80 space-y-4 rounded-lg border border-neutral-700 bg-neutral-800 p-6 shadow-xl"
	>
		<h1 class="text-center text-lg font-semibold tracking-tight text-neutral-100">CSFleet</h1>

		<div class="space-y-1">
			<label for="login-user" class="text-xs text-neutral-400">Username</label>
			<input
				id="login-user"
				type="text"
				value={username}
				oninput={(e) => (username = e.currentTarget.value.toLowerCase())}
				autocomplete="username"
				autocapitalize="none"
				spellcheck="false"
				class="w-full rounded border border-neutral-700 bg-neutral-900 px-2 py-1.5 text-sm text-neutral-200 outline-none focus:border-neutral-500"
			/>
		</div>

		<div class="space-y-1">
			<label for="login-pass" class="text-xs text-neutral-400">Password</label>
			<input
				id="login-pass"
				type="password"
				bind:value={password}
				autocomplete="current-password"
				class="w-full rounded border border-neutral-700 bg-neutral-900 px-2 py-1.5 text-sm text-neutral-200 outline-none focus:border-neutral-500"
			/>
		</div>

		{#if error}
			<p class="text-xs text-red-400">{error}</p>
		{/if}

		<button
			type="submit"
			disabled={!canSubmit}
			class="w-full rounded bg-neutral-200 px-3 py-1.5 text-sm font-medium text-neutral-900 hover:bg-white disabled:cursor-not-allowed disabled:bg-neutral-700 disabled:text-neutral-500"
		>
			{submitting ? 'Signing in…' : 'Sign in'}
		</button>
	</form>
</div>
