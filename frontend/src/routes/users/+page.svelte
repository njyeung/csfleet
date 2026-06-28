<script lang="ts">
	import { onMount } from 'svelte';
	import { auth } from '$lib/api/services';
	import type { User } from '$lib/api/services/auth';
	import Modal from '$lib/components/ui/Modal.svelte';
	import ConfirmDialog from '$lib/components/ui/ConfirmDialog.svelte';

	// Web UI accounts. The seed admin (from .env) is listed but can't be deleted or
	// reset here — its actions are hidden and the API rejects them defensively.
	let users = $state<User[]>([]);
	let loaded = $state(false);
	let loadError = $state<string | null>(null);

	async function load() {
		const res = await auth.listUsers();
		if (res.ok) {
			users = res.data;
			loadError = null;
		} else {
			loadError = `Failed to load users (HTTP ${res.status})`;
		}
		loaded = true;
	}

	onMount(load);

	// --- create ---
	let newUser = $state('');
	let newPass = $state('');
	let creating = $state(false);
	let createError = $state<string | null>(null);

	const canCreate = $derived(!creating && newUser.trim() !== '' && newPass.length >= 8);

	async function create() {
		if (!canCreate) return;
		creating = true;
		createError = null;
		const res = await auth.createUser(newUser.trim(), newPass);
		creating = false;
		if (res.ok) {
			newUser = '';
			newPass = '';
			await load();
		} else {
			createError =
				res.status === 409 ? 'That username is taken.' : `Create failed (HTTP ${res.status})`;
		}
	}

	// --- delete ---
	let deleteTarget = $state<string | null>(null);
	let deleting = $state(false);

	async function confirmDelete() {
		if (!deleteTarget) return;
		deleting = true;
		const res = await auth.deleteUser(deleteTarget);
		deleting = false;
		if (res.ok) {
			deleteTarget = null;
			await load();
		}
	}

	// --- reset password ---
	let resetTarget = $state<string | null>(null);
	let resetPass = $state('');
	let resetting = $state(false);
	let resetError = $state<string | null>(null);

	function openReset(username: string) {
		resetTarget = username;
		resetPass = '';
		resetError = null;
	}

	const canReset = $derived(!resetting && resetPass.length >= 8);

	async function confirmReset() {
		if (!resetTarget || !canReset) return;
		resetting = true;
		resetError = null;
		const res = await auth.setPassword(resetTarget, resetPass);
		resetting = false;
		if (res.ok) {
			resetTarget = null;
		} else {
			resetError = `Reset failed (HTTP ${res.status})`;
		}
	}

	function fmtDate(iso: string) {
		if (!iso) return '—';
		const d = new Date(iso);
		return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString();
	}
</script>

<div class="mx-auto max-w-3xl space-y-6 p-6">
	<h1 class="text-base font-semibold text-neutral-100">Users</h1>

	<!-- Add user -->
	<div class="space-y-3 rounded-lg border border-neutral-700 bg-neutral-800 p-4">
		<h2 class="text-sm font-medium text-neutral-200">Add user</h2>
		<div class="flex flex-wrap items-start gap-2">
			<input
				type="text"
				value={newUser}
				oninput={(e) => (newUser = e.currentTarget.value.toLowerCase())}
				placeholder="username"
				spellcheck="false"
				autocomplete="off"
				autocapitalize="none"
				class="w-44 rounded border border-neutral-700 bg-neutral-900 px-2 py-1.5 text-sm text-neutral-200 outline-none focus:border-neutral-500"
			/>
			<div class="flex flex-col gap-1">
				<input
					type="password"
					bind:value={newPass}
					placeholder="password (min 8)"
					autocomplete="new-password"
					class="w-52 rounded border border-neutral-700 bg-neutral-900 px-2 py-1.5 text-sm text-neutral-200 outline-none focus:border-neutral-500"
				/>
				{#if newPass !== '' && newPass.length < 8}
					<span class="text-xs text-neutral-500">At least 8 characters.</span>
				{/if}
			</div>
			<button
				type="button"
				disabled={!canCreate}
				onclick={create}
				class="rounded bg-neutral-200 px-3 py-1.5 text-sm font-medium text-neutral-900 hover:bg-white disabled:cursor-not-allowed disabled:bg-neutral-700 disabled:text-neutral-500"
			>
				{creating ? 'Adding…' : 'Add user'}
			</button>
		</div>
		{#if createError}
			<p class="text-xs text-red-400">{createError}</p>
		{/if}
	</div>

	<!-- User list -->
	{#if !loaded}
		<p class="text-sm text-neutral-500">Loading…</p>
	{:else if loadError}
		<p class="text-sm text-red-400">{loadError}</p>
	{:else}
		<div class="overflow-hidden rounded-lg border border-neutral-700">
			<table class="w-full text-sm">
				<thead class="bg-neutral-800 text-left text-xs text-neutral-500">
					<tr>
						<th class="px-4 py-2 font-medium">Username</th>
						<th class="px-4 py-2 font-medium">Created</th>
						<th class="px-4 py-2 text-right font-medium">Actions</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-neutral-800">
					{#each users as u (u.username)}
						<tr class="bg-neutral-900/40">
							<td class="px-4 py-2 text-neutral-200">
								{u.username}
								{#if u.seed}
									<span
										class="ml-2 rounded border border-neutral-600 px-1.5 py-0.5 text-xs text-neutral-400"
									>
										admin
									</span>
								{/if}
							</td>
							<td class="px-4 py-2 text-neutral-500">{u.seed ? '—' : fmtDate(u.created_at)}</td>
							<td class="px-4 py-2">
								{#if u.seed}
									<div class="text-right text-xs text-neutral-600">managed via .env</div>
								{:else}
									<div class="flex justify-end gap-2">
										<button
											type="button"
											onclick={() => openReset(u.username)}
											class="rounded border border-neutral-600 px-2 py-1 text-xs text-neutral-300 hover:bg-neutral-700/50"
										>
											Reset password
										</button>
										<button
											type="button"
											onclick={() => (deleteTarget = u.username)}
											class="rounded border border-red-500/40 px-2 py-1 text-xs text-red-300 hover:bg-red-500/10"
										>
											Delete
										</button>
									</div>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<ConfirmDialog
	open={deleteTarget !== null}
	title="Delete user"
	message={`Delete user "${deleteTarget}"? This can't be undone.`}
	confirmLabel="Delete"
	busy={deleting}
	onconfirm={confirmDelete}
	oncancel={() => (deleteTarget = null)}
/>

<Modal
	open={resetTarget !== null}
	title={`Reset password — ${resetTarget}`}
	busy={resetting}
	onclose={() => (resetTarget = null)}
>
	<div class="space-y-3">
		<input
			type="password"
			bind:value={resetPass}
			placeholder="new password (min 8)"
			autocomplete="new-password"
			class="w-full rounded border border-neutral-700 bg-neutral-900 px-2 py-1.5 text-sm text-neutral-200 outline-none focus:border-neutral-500"
		/>
		{#if resetError}
			<p class="text-xs text-red-400">{resetError}</p>
		{/if}
		<div class="flex justify-end gap-2">
			<button
				type="button"
				disabled={resetting}
				onclick={() => (resetTarget = null)}
				class="rounded border border-neutral-600 px-3 py-1 text-sm text-neutral-300 hover:bg-neutral-700/50 disabled:opacity-50"
			>
				Cancel
			</button>
			<button
				type="button"
				disabled={!canReset}
				onclick={confirmReset}
				class="rounded bg-neutral-200 px-3 py-1 text-sm font-medium text-neutral-900 hover:bg-white disabled:cursor-not-allowed disabled:bg-neutral-700 disabled:text-neutral-500"
			>
				{resetting ? 'Saving…' : 'Save'}
			</button>
		</div>
	</div>
</Modal>
