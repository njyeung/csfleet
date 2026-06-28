<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';
	import { sse } from '$lib/api/sse.svelte';
	import { setUnauthorizedHandler } from '$lib/api/client';
	import { auth } from '$lib/stores/auth.svelte';
	import { newResource } from '$lib/stores/newresource.svelte';
	import NewModal from '$lib/components/NewModal.svelte';

	let { children } = $props();

	// Resolve the session cookie once, and route any later 401 back to /login.
	onMount(() => {
		setUnauthorizedHandler(() => {
			auth.clear();
			goto('/login');
		});
		auth.init();
	});

	// The live status stream only runs while authenticated — opened once we have a
	// user, torn down on logout. connect() is idempotent across re-runs.
	$effect(() => {
		if (auth.user) sse.connect();
		else sse.disconnect();
	});

	// Push unauthenticated visitors to the login screen (and bounce them off it
	// once signed in). Waits for the initial cookie check so we never redirect
	// before we know the answer.
	$effect(() => {
		if (!auth.ready) return;
		if (!auth.user && page.url.pathname !== '/login') goto('/login');
		if (auth.user && page.url.pathname === '/login') goto('/');
	});

	async function logout() {
		await auth.logout();
		goto('/login');
	}

	const tabs = [
		{ href: '/global', label: 'Global settings' },
		{ href: '/plugins', label: 'Plugins' },
		{ href: '/configs', label: 'Configs' },
		{ href: '/host', label: 'Host' },
		{ href: '/users', label: 'Users' }
	];

	// A tab is active for its exact path or anything nested under it.
	const isActive = (href: string) =>
		page.url.pathname === href || page.url.pathname.startsWith(href + '/');
</script>

<svelte:head><link rel="icon" href={favicon} /></svelte:head>

{#if !auth.ready}
	<div class="flex h-screen items-center justify-center bg-neutral-900 text-sm text-neutral-500">
		Loading…
	</div>
{:else if page.url.pathname === '/login'}
	{@render children()}
{:else if auth.user}
	<div class="flex h-screen flex-col bg-neutral-900 text-neutral-300">
		<header
			class="flex items-center gap-1 border-b border-neutral-700 bg-neutral-800 px-3 py-2 text-sm"
		>
			<a href="/" class="mr-3 font-semibold tracking-tight text-neutral-100 hover:text-white">
				CSFleet
			</a>

			<button
				type="button"
				onclick={() => newResource.openChooser()}
				class="mr-2 rounded border border-neutral-600 px-2 py-1 text-neutral-200 hover:bg-neutral-700/50"
			>
				+ New
			</button>

			<nav class="flex items-center gap-1">
				{#each tabs as tab (tab.href)}
					<a
						href={tab.href}
						class="rounded px-2.5 py-1 hover:bg-neutral-700/50 {isActive(tab.href)
							? 'bg-neutral-700 text-neutral-100'
							: 'text-neutral-400'}"
					>
						{tab.label}
					</a>
				{/each}
			</nav>

			<div class="ml-auto flex items-center gap-3 text-xs text-neutral-500">
				<span class="flex items-center gap-2">
					<span
						class="inline-block h-2 w-2 rounded-full {sse.connected
							? 'bg-green-500'
							: 'bg-red-500'}"
					></span>
					{sse.connected ? 'Connected' : 'Disconnected'}
				</span>
				<span class="text-neutral-400">{auth.user.username}</span>
				<button
					type="button"
					onclick={logout}
					class="rounded border border-neutral-600 px-2 py-1 text-neutral-300 hover:bg-neutral-700/50"
				>
					Logout
				</button>
			</div>
		</header>

		<div class="min-h-0 flex-1">
			{@render children()}
		</div>
	</div>

	<NewModal />
{:else}
	<div class="flex h-screen items-center justify-center bg-neutral-900 text-sm text-neutral-500">
		Redirecting…
	</div>
{/if}
