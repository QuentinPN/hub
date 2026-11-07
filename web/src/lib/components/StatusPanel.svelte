<script lang="ts">
	import { resolve } from '$app/paths';
	import { getStatus } from '#lib/api/client.ts';
	import type { HubStatus, StatusCheck } from '#lib/api/types.ts';
	import { ago, formatDay } from '#lib/agents.ts';
	import Skeleton from './Skeleton.svelte';

	let { onproblems }: { onproblems?: (count: number) => void } = $props();

	let status = $state<HubStatus | null>(null);
	let error = $state('');
	let now = $state(Date.now());

	async function load() {
		try {
			status = await getStatus(fetch);
			error = '';
			onproblems?.(status.problems);
		} catch (err) {
			error = `Le serveur Hub ne répond pas : ${err instanceof Error ? err.message : String(err)}`;
			onproblems?.(1);
		}
		now = Date.now();
	}

	$effect(() => {
		void load();
		const poll = setInterval(load, 15_000);
		return () => clearInterval(poll);
	});

	/** « 2 j 4 h », « 14 min ». */
	function uptime(iso: string): string {
		const minutes = Math.max(0, Math.floor((now - Date.parse(iso)) / 60_000));
		if (minutes < 60) return `${minutes} min`;
		const hours = Math.floor(minutes / 60);
		if (hours < 24) return `${hours} h ${String(minutes % 60).padStart(2, '0')}`;
		return `${Math.floor(hours / 24)} j ${hours % 24} h`;
	}

	const COLORS: Record<string, string> = {
		ok: 'var(--status-online)',
		warn: 'var(--status-degraded)',
		bad: 'var(--status-down)'
	};

	function link(c: StatusCheck): { href: string; label: string } | null {
		if (c.level === 'ok') return null;
		if (c.link === 'hosts') return { href: '#hosts', label: 'Hôtes' };
		if (c.link === 'widgets') return { href: '#widgets', label: 'Widgets' };
		if (c.link === 'backup') return { href: '#backup', label: 'Sauvegarde' };
		if (c.link === 'notifications') return { href: '#notifications', label: 'Notifications' };
		if (c.link === 'services') return { href: resolve('/services'), label: 'Services' };
		return null;
	}
</script>

<div class="head">
	<div>
		<h2 id="status-title">État de Hub</h2>
		<p class="muted intro">Relu toutes les 15 s. Vu par le serveur, pas par ton navigateur.</p>
	</div>
	{#if status}
		<span class="pill" class:warn={status.problems > 0}>
			<span class="dot" style:background={status.problems > 0 ? COLORS.warn : COLORS.ok}></span>
			{status.problems === 0
				? 'Tout fonctionne'
				: status.problems === 1
					? '1 point à regarder'
					: `${status.problems} points à regarder`}
		</span>
	{/if}
</div>

{#if error}
	<p class="error" role="alert">{error}</p>
{:else if !status}
	<div class="tiles" aria-busy="true">
		<Skeleton height="86px" radius="16px" />
		<Skeleton height="86px" radius="16px" />
		<Skeleton height="86px" radius="16px" />
		<Skeleton height="86px" radius="16px" />
	</div>
{:else}
	<div class="tiles">
		<div class="tile">
			<span class="muted small">Version</span>
			<strong>{status.version || 'inconnue'}</strong>
			<span class="muted tiny">image hub-server</span>
		</div>
		<div class="tile">
			<span class="muted small">En marche depuis</span>
			<strong>{status.startedAt ? uptime(status.startedAt) : '—'}</strong>
			<span class="muted tiny"
				>{status.startedAt ? `démarré le ${formatDay(status.startedAt)}` : ''}</span
			>
		</div>
		<div class="tile">
			<span class="muted small">Base de données</span>
			<strong class="tabular">{status.dbLatencyMs} ms</strong>
			<span class="muted tiny"
				>migration {status.migrationsApplied} sur {status.migrationsExpected}</span
			>
		</div>
		<div class="tile">
			<span class="muted small">Pages ouvertes</span>
			<strong class="tabular">{status.pages}</strong>
			<span class="muted tiny">reçoivent les signaux</span>
		</div>
	</div>

	<ul class="checks">
		{#each status.checks as c (c.id)}
			{@const go = link(c)}
			<li>
				<span class="dot" style:background={COLORS[c.level]} aria-hidden="true"></span>
				<strong>{c.name}</strong>
				<span class="detail"
					><span class="visually-hidden"
						>{c.level === 'ok' ? 'Bon' : c.level === 'warn' ? 'À regarder' : 'À corriger'} :
					</span>{c.detail}</span
				>
				<span class="muted when">{c.since ? ago(c.since, now) : ''}</span>
				{#if go}
					<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- ancre de la page ou chemin déjà résolu -->
					<a class="btn" href={go.href}>{go.label}</a>
				{:else}
					<span></span>
				{/if}
			</li>
		{/each}
	</ul>
	<p class="muted small">
		Pour aller plus loin : <code>docker compose logs hub</code> sur l’hôte de Hub. La documentation
		d’exploitation (<code>docs/EXPLOITATION.md</code>) explique quoi chercher.
	</p>
{/if}

<style>
	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-start;
		justify-content: space-between;
		gap: 12px 16px;
	}
	h2 {
		margin: 0;
		font-size: var(--fs-h2);
		font-weight: 600;
	}
	.intro {
		margin: 4px 0 0;
		font-size: 14px;
	}
	.pill {
		display: inline-flex;
		align-items: center;
		gap: 9px;
		padding: 10px 14px;
		border: 1px solid var(--tint-ok-border);
		border-radius: var(--r-pill);
		background: var(--tint-ok-bg);
		font-weight: 600;
	}
	.pill.warn {
		border-color: var(--tint-warn-border);
		background: var(--tint-warn-bg);
	}
	.dot {
		flex: none;
		width: 9px;
		height: 9px;
		border-radius: 99px;
	}
	.error {
		margin: 8px 0 0;
		color: var(--text-danger);
		font-size: 14px;
	}
	.tiles {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(170px, 1fr));
		gap: 14px;
		margin-top: 14px;
	}
	.tile {
		display: flex;
		flex-direction: column;
		gap: 6px;
		min-width: 0;
		padding: 16px;
		border: 1px solid var(--border-1);
		border-radius: 16px;
		background: var(--surface-inset);
	}
	.tile strong {
		font-size: 22px;
		font-weight: 700;
		overflow-wrap: anywhere;
	}
	.small {
		font-size: 14px;
	}
	.tiny {
		font-size: 13px;
	}
	.checks {
		display: flex;
		flex-direction: column;
		margin: 14px 0 0;
		padding: 0;
		list-style: none;
	}
	.checks li {
		display: grid;
		grid-template-columns: 22px 200px minmax(0, 1fr) 130px 110px;
		align-items: center;
		gap: 12px;
		min-height: 56px;
		border-top: 1px solid var(--border-1);
	}
	.checks strong {
		font-weight: 600;
	}
	.detail {
		min-width: 0;
		overflow-wrap: anywhere;
	}
	.when {
		font-size: 14px;
		text-align: right;
	}
	.btn {
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: 44px;
		border: 1px solid var(--border-2);
		border-radius: 12px;
		background: var(--surface-2);
		font-size: 14px;
		font-weight: 600;
	}
	code {
		font-size: 13px;
	}
	p.small {
		margin: 14px 0 0;
	}
	@media (max-width: 860px) {
		.checks li {
			grid-template-columns: 22px minmax(0, 1fr) auto;
			gap: 4px 10px;
			padding: 10px 0;
		}
		.detail {
			grid-column: 2 / -1;
			grid-row: 2;
			font-size: 14px;
		}
		.when {
			display: none;
		}
		.btn {
			grid-column: 3;
			grid-row: 1;
			padding: 0 12px;
		}
	}
</style>
