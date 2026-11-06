<script lang="ts">
	import { resolve } from '$app/paths';
	import { getWidgetRows } from '#lib/api/client.ts';
	import type { WidgetRow } from '#lib/api/types.ts';
	import { ago } from '#lib/agents.ts';
	import { onWidgetNews } from '#lib/live.ts';
	import { widgetStateView } from '#lib/widgets.ts';
	import ServiceIcon from './ServiceIcon.svelte';
	import Skeleton from './Skeleton.svelte';

	let rows = $state<WidgetRow[] | null>(null);
	let error = $state('');
	let now = $state(Date.now());

	async function load() {
		try {
			rows = await getWidgetRows(fetch);
			now = Date.now();
			error = '';
		} catch (err) {
			error = `Impossible de charger les connexions : ${err instanceof Error ? err.message : String(err)}`;
		}
	}

	$effect(() => {
		void load();
		const poll = setInterval(load, 30_000);
		const stop = onWidgetNews(() => void load());
		return () => {
			clearInterval(poll);
			stop();
		};
	});

	function stateText(r: WidgetRow): string {
		if (r.state === 'ok') return r.dataAt ? `Connecté · à jour ${ago(r.dataAt, now)}` : 'Connecté';
		if (r.state === 'unreachable' && r.dataAt) return `Injoignable, données ${ago(r.dataAt, now)}`;
		if (r.state) return widgetStateView(r.state).label;
		if (r.support === 'planned') return 'Widget à venir';
		if (r.support === 'link') return 'Lien seulement : pas d’API confirmée';
		return r.hasUrl ? 'Pas encore connecté' : 'Adresse à donner d’abord';
	}

	const stateColor = (r: WidgetRow) =>
		r.state ? widgetStateView(r.state).color : 'var(--status-unknown)';

	function action(r: WidgetRow): string {
		if (r.state) return widgetStateView(r.state).tone === 'bad' ? 'Corriger' : 'Gérer';
		if (r.support !== 'ready') return '';
		return r.hasUrl ? 'Connecter' : 'Ouvrir';
	}
</script>

<h2 id="widgets-title">Widgets et connexions</h2>
<p class="muted intro">
	Hub propose un widget pour chaque service reconnu. Rien ne se connecte sans toi.
</p>

{#if error}
	<p class="error" role="alert">{error}</p>
{/if}
{#if rows === null && !error}
	<div aria-busy="true" class="rows">
		<Skeleton height="66px" radius="14px" />
		<Skeleton height="66px" radius="14px" />
	</div>
{:else if rows && rows.length === 0}
	<p class="muted">Aucun service reconnu pour l’instant.</p>
{:else if rows}
	<ul class="rows">
		{#each rows as r (r.slug)}
			{@const label = action(r)}
			<li>
				<ServiceIcon icon={r.icon} monogram={r.monogram} color={r.color} size={40} />
				<span class="who"><strong>{r.name}</strong><span class="muted small">{r.hostId}</span></span
				>
				<span class="state"
					><span class="dot" style:background={stateColor(r)}></span>{stateText(r)}</span
				>
				{#if label}
					<a
						class="btn"
						href="{resolve('/services/[slug]', { slug: r.slug })}#widgets"
						aria-label="{label} {r.name}">{label}</a
					>
				{/if}
			</li>
		{/each}
	</ul>
{/if}

<style>
	h2 {
		margin: 0;
		font-size: var(--fs-h2);
		font-weight: 600;
	}
	.intro {
		margin: 0 0 10px;
		font-size: 14px;
	}
	.error {
		margin: 0;
		color: #ff8a80;
		font-size: 14px;
	}
	.rows {
		display: flex;
		flex-direction: column;
		gap: 6px;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	li {
		display: grid;
		grid-template-columns: 44px minmax(0, 1fr) minmax(0, 1.5fr) 120px;
		align-items: center;
		gap: 14px;
		min-height: 66px;
		border-top: 1px solid var(--border-1);
	}
	.who {
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}
	.who strong {
		font-weight: 600;
	}
	.small {
		font-size: 13px;
	}
	.state {
		display: flex;
		align-items: center;
		gap: 9px;
		min-width: 0;
		font-size: 15px;
	}
	.dot {
		flex: none;
		width: 9px;
		height: 9px;
		border-radius: 99px;
	}
	.btn {
		display: flex;
		grid-column: 4;
		align-items: center;
		justify-content: center;
		min-height: 44px;
		border: 1px solid var(--border-2);
		border-radius: 12px;
		background: var(--surface-2);
		font-size: 15px;
		font-weight: 600;
	}
	@media (max-width: 720px) {
		li {
			grid-template-columns: 44px minmax(0, 1fr) auto;
			gap: 4px 12px;
			padding: 10px 0;
		}
		.state {
			grid-row: 2;
			grid-column: 2 / -1;
			font-size: 14px;
		}
		.btn {
			grid-column: 3;
			grid-row: 1;
			padding: 0 14px;
		}
	}
</style>
