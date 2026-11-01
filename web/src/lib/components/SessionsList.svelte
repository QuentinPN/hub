<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { getSessions, logout, revokeSession } from '#lib/api/client.ts';
	import type { SessionInfo } from '#lib/api/types.ts';
	import Skeleton from './Skeleton.svelte';

	let sessions = $state<SessionInfo[] | null>(null);
	let busy = $state<number | 'logout' | null>(null);
	let message = $state('');

	const dateFmt = new Intl.DateTimeFormat('fr-FR', { dateStyle: 'medium', timeStyle: 'short' });
	const formatDate = (iso: string) => dateFmt.format(new Date(iso));

	// Résumé lisible du user agent : navigateur et système, sans bibliothèque de parsing. L'ordre
	// compte : Edge annonce aussi Chrome, et Chrome annonce aussi Safari.
	const browsers: [string, string][] = [
		['Edg/', 'Edge'],
		['Firefox/', 'Firefox'],
		['Chrome/', 'Chrome'],
		['Safari/', 'Safari']
	];
	const systems: [string, string][] = [
		['Windows', 'Windows'],
		['Android', 'Android'],
		['iPhone', 'iPhone'],
		['iPad', 'iPad'],
		['Mac OS X', 'macOS'],
		['Linux', 'Linux']
	];
	const match = (ua: string, table: [string, string][]) =>
		table.find(([needle]) => ua.includes(needle))?.[1];

	function device(ua: string): string {
		if (!ua) return 'Appareil inconnu';
		const browser = match(ua, browsers) ?? 'Navigateur';
		const os = match(ua, systems);
		return os ? `${browser} · ${os}` : browser;
	}

	async function load() {
		try {
			// La session courante d'abord, puis les plus récemment utilisées.
			sessions = (await getSessions(fetch)).sort(
				(a, b) => Number(b.current) - Number(a.current) || b.lastSeenAt.localeCompare(a.lastSeenAt)
			);
		} catch (err) {
			message = `Impossible de charger les sessions : ${err instanceof Error ? err.message : String(err)}`;
		}
	}
	void load();

	async function revoke(s: SessionInfo) {
		busy = s.id;
		message = '';
		try {
			await revokeSession(fetch, s.id);
			sessions = sessions?.filter((x) => x.id !== s.id) ?? null;
			message = `Session ${device(s.userAgent)} révoquée.`;
		} catch (err) {
			message = `Échec de la révocation : ${err instanceof Error ? err.message : String(err)}`;
		} finally {
			busy = null;
		}
	}

	async function signOut() {
		busy = 'logout';
		message = '';
		try {
			await logout(fetch);
			await goto(resolve('/login'), { invalidateAll: true });
		} catch (err) {
			message = `Échec de la déconnexion : ${err instanceof Error ? err.message : String(err)}`;
			busy = null;
		}
	}
</script>

{#if sessions === null && !message}
	<div aria-busy="true">
		<Skeleton height="64px" radius="14px" />
	</div>
{:else if sessions}
	<ul class="sessions">
		{#each sessions as s (s.id)}
			<li class="session">
				<div class="who">
					<div class="name">
						{device(s.userAgent)}
						{#if s.current}<span class="badge">Cette session</span>{/if}
					</div>
					<div class="muted small">
						<span class="mono">{s.ip}</span> · ouverte le {formatDate(s.createdAt)} · vue le
						{formatDate(s.lastSeenAt)}
					</div>
				</div>
				{#if !s.current}
					<button
						type="button"
						class="btn ghost"
						disabled={busy !== null}
						aria-label="Révoquer la session {device(s.userAgent)} ({s.ip})"
						onclick={() => revoke(s)}>{busy === s.id ? 'Révocation…' : 'Révoquer'}</button
					>
				{/if}
			</li>
		{/each}
	</ul>
{/if}

<div class="actions">
	<p class="status" role="status">{message}</p>
	<button type="button" class="btn primary" disabled={busy !== null} onclick={signOut}
		>{busy === 'logout' ? 'Déconnexion…' : 'Se déconnecter'}</button
	>
</div>

<style>
	.sessions {
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.session {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
		padding: 16px 0;
		border-top: 1px solid var(--border-1);
	}
	.session:first-child {
		border-top: 0;
	}
	.who {
		min-width: 0;
	}
	.name {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 10px;
		margin-bottom: 3px;
		font-size: 16px;
		font-weight: 600;
	}
	.badge {
		padding: 2px 10px;
		border-radius: 99px;
		background: rgba(255, 255, 255, 0.12);
		font-size: 12px;
		font-weight: 600;
	}
	.small {
		font-size: var(--fs-small);
		line-height: 1.5;
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: flex-end;
		gap: 10px;
		padding-top: 16px;
		border-top: 1px solid var(--border-1);
	}
	.status {
		flex: 1;
		min-width: 200px;
		margin: 0;
		color: var(--text-2);
		font-size: 14px;
	}
	.btn {
		flex: none;
		min-height: 44px;
		padding: 0 18px;
		border: 0;
		border-radius: 12px;
		font-size: 15px;
		font-weight: 600;
		cursor: pointer;
	}
	.btn:disabled {
		opacity: 0.45;
		cursor: default;
	}
	.primary {
		background: var(--text);
		color: var(--text-inverse);
	}
	.ghost {
		border: 1px solid var(--border-2);
		background: transparent;
		color: var(--text);
	}
	@media (max-width: 720px) {
		.session {
			flex-direction: column;
			align-items: flex-start;
			gap: 10px;
		}
	}
</style>
