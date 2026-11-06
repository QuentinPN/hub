<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import type { ServiceSummary } from '#lib/api/types.ts';
	import AppCard from '#lib/components/AppCard.svelte';
	import ErrorPanel from '#lib/components/ErrorPanel.svelte';
	import Skeleton from '#lib/components/Skeleton.svelte';
	import {
		SEARCH_INPUT_ID,
		catalogueView,
		type GroupBy,
		type StatusFilter
	} from '#lib/catalogue.ts';
	import { statusStyle } from '#lib/status.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	let query = $state('');
	let status = $state<StatusFilter>('all');
	let groupBy = $state<GroupBy>('category');

	// « Arrêté » n'a pas de puce tant qu'aucun service n'est dans cet état.
	const filters: { key: StatusFilter; label: string; always: boolean }[] = [
		{ key: 'all', label: 'Tous', always: true },
		{ key: 'online', label: 'En ligne', always: true },
		{ key: 'degraded', label: 'Dégradé', always: true },
		{ key: 'down', label: 'Arrêté', always: false },
		{ key: 'unknown', label: 'Inconnu', always: true },
		{ key: 'config_required', label: 'Config. requise', always: true }
	];
	const groupings: { key: GroupBy; label: string }[] = [
		{ key: 'category', label: 'Catégorie' },
		{ key: 'host', label: 'Hôte' }
	];

	const hostCount = (list: ServiceSummary[]) => new Set(list.map((s) => s.hostId)).size;
	const plural = (n: number, one: string, many: string) => `${n} ${n > 1 ? many : one}`;
</script>

<svelte:head><title>Tous les services · Hub</title></svelte:head>

<div class="page">
	<div class="head">
		<div>
			<h1>Tous les services</h1>
			{#await data.services}
				<div class="lead"><Skeleton width="260px" height="17px" /></div>
			{:then list}
				<p class="muted lead tabular">
					{plural(list.length, 'service', 'services')} sur {plural(
						hostCount(list),
						'hôte',
						'hôtes'
					)}
				</p>
			{:catch}
				<p class="muted lead">Inventaire indisponible</p>
			{/await}
		</div>
		<label class="search" for={SEARCH_INPUT_ID}>
			<svg
				width="18"
				height="18"
				viewBox="0 0 24 24"
				fill="none"
				stroke="currentColor"
				stroke-width="2"
				stroke-linecap="round"
				aria-hidden="true"
				><circle cx="11" cy="11" r="7"></circle><path d="m20 20-3.5-3.5"></path></svg
			>
			<span class="visually-hidden">Rechercher un service</span>
			<input
				id={SEARCH_INPUT_ID}
				type="search"
				placeholder="Nom, hôte, catégorie…"
				autocomplete="off"
				aria-keyshortcuts="Control+K"
				bind:value={query}
			/>
			<kbd>Ctrl+K</kbd>
		</label>
	</div>

	{#await data.services}
		<div class="grid" aria-busy="true">
			{#each { length: 8 }, i (i)}
				<Skeleton height="148px" radius="var(--r-card)" />
			{/each}
		</div>
	{:then list}
		{@const view = catalogueView(list, query, status, groupBy)}
		{@const toConfigure = list.filter((s) => s.status === 'config_required')}
		{@const shown = view.groups.reduce((n, g) => n + g.services.length, 0)}

		{#if toConfigure.length > 0}
			<button type="button" class="notice" onclick={() => (status = 'config_required')}>
				<span class="notice-dot" aria-hidden="true"></span>
				<span class="notice-text">
					<strong
						>{plural(toConfigure.length, 'service demande', 'services demandent')} une configuration</strong
					>
					<span class="muted">· {toConfigure.map((s) => `${s.name} (${s.note})`).join(', ')}</span>
				</span>
				<span class="notice-action">Afficher</span>
			</button>
		{/if}

		<div class="controls">
			<div class="chips" role="group" aria-label="Filtrer par état">
				{#each filters as f (f.key)}
					{#if f.always || view.counts[f.key] > 0}
						<button
							type="button"
							class="chip"
							class:on={status === f.key}
							aria-pressed={status === f.key}
							onclick={() => (status = f.key)}
						>
							{#if f.key !== 'all'}
								<span class="dot" style:background={statusStyle(f.key).color}></span>
							{/if}
							{f.label} <b class="tabular">{view.counts[f.key]}</b>
						</button>
					{/if}
				{/each}
			</div>
			<div class="group-by">
				<span class="muted" id="group-label">Grouper par</span>
				<div class="seg" role="group" aria-labelledby="group-label">
					{#each groupings as g (g.key)}
						<button
							type="button"
							class:on={groupBy === g.key}
							aria-pressed={groupBy === g.key}
							onclick={() => (groupBy = g.key)}>{g.label}</button
						>
					{/each}
				</div>
			</div>
		</div>

		<p class="visually-hidden" aria-live="polite">
			{plural(shown, 'service affiché', 'services affichés')}
		</p>

		{#if list.length === 0}
			<p class="muted empty">
				Aucun service pour l’instant. Ils apparaîtront ici une fois un hôte connecté.
				<a class="link" href={resolve('/settings#hosts')}>Connecter un hôte</a>
			</p>
		{:else if shown === 0}
			<p class="muted empty">
				Aucun service ne correspond. Essaie un autre filtre ou une autre recherche.
			</p>
		{/if}

		{#each view.groups as group (group.name)}
			<section aria-label={group.name}>
				<h2>{group.name} <span class="muted count tabular">{group.services.length}</span></h2>
				<div class="grid">
					{#each group.services as service (service.slug)}
						<AppCard
							{service}
							showFavorite
							sub={`${groupBy === 'category' ? service.hostId : service.category} · ${service.note}`}
						/>
					{/each}
				</div>
			</section>
		{/each}
	{:catch err}
		<ErrorPanel
			title="Impossible de charger l'inventaire"
			detail={err instanceof Error ? err.message : String(err)}
			onretry={() => invalidateAll()}
		/>
	{/await}
</div>

<style>
	.page {
		display: flex;
		flex-direction: column;
		gap: 28px;
		min-height: 100vh;
		padding: calc(var(--shell-h) + 48px) var(--space-page) 64px;
		background: radial-gradient(90% 40% at 70% 0%, #17222d 0%, rgba(7, 8, 10, 0) 70%), var(--bg);
	}
	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		justify-content: space-between;
		gap: 20px;
	}
	h1 {
		margin: 0;
		font-size: clamp(2.5rem, 5vw, 3.5rem);
		font-weight: 700;
		line-height: 1.02;
		letter-spacing: -0.025em;
	}
	.lead {
		margin: 8px 0 0;
		font-size: 17px;
	}
	.search {
		display: flex;
		flex: 0 1 460px;
		align-items: center;
		gap: 10px;
		height: 52px;
		padding: 0 16px;
		border: 1px solid var(--border-2);
		border-radius: 16px;
		background: var(--surface-2);
		color: var(--text-2);
		backdrop-filter: var(--blur);
		-webkit-backdrop-filter: var(--blur);
	}
	.search:focus-within {
		border-color: var(--border-focus);
	}
	/* Le champ n'a pas d'outline propre : l'anneau entoure toute la pilule. */
	.search:has(input:focus-visible) {
		outline: 2px solid var(--text);
		outline-offset: 3px;
	}
	.search input {
		flex: 1;
		min-width: 0;
		border: 0;
		background: transparent;
		color: var(--text);
		font: inherit;
		font-size: 16px;
		outline: none;
	}
	kbd {
		padding: 3px 7px;
		border: 1px solid var(--border-2);
		border-radius: 6px;
		font-family: inherit;
		font-size: 12px;
	}
	.notice {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 14px;
		width: 100%;
		padding: 16px 20px;
		border: 1px solid rgba(232, 201, 166, 0.25);
		border-radius: 18px;
		background: rgba(232, 201, 166, 0.08);
		text-align: left;
		cursor: pointer;
	}
	.notice-dot {
		flex: none;
		width: 10px;
		height: 10px;
		border-radius: 99px;
		background: var(--accent-play);
	}
	.notice-text {
		flex: 1;
		min-width: 240px;
		font-size: 15px;
	}
	.notice-action {
		padding: 9px 14px;
		border-radius: 10px;
		background: var(--surface-3);
		font-size: 14px;
		font-weight: 600;
	}
	.controls {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 14px;
	}
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
	}
	.chip {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		min-height: 44px;
		padding: 0 14px;
		border: 1px solid rgba(255, 255, 255, 0.1);
		border-radius: var(--r-pill);
		background: var(--surface-1);
		font-size: 14px;
		cursor: pointer;
	}
	.chip:hover {
		background: rgba(255, 255, 255, 0.1);
	}
	.chip b {
		font-weight: 600;
		opacity: 0.7;
	}
	.chip.on,
	.seg .on {
		border-color: transparent;
		background: rgba(255, 255, 255, 0.92);
		color: var(--text-inverse);
	}
	.dot {
		flex: none;
		width: 9px;
		height: 9px;
		border-radius: 99px;
	}
	.group-by {
		display: flex;
		align-items: center;
		gap: 10px;
		font-size: 14px;
	}
	.seg {
		display: inline-flex;
		gap: 2px;
		padding: 4px;
		border-radius: var(--r-pill);
		background: rgba(255, 255, 255, 0.06);
	}
	.seg button {
		min-height: 36px;
		padding: 0 14px;
		border: 0;
		border-radius: var(--r-pill);
		background: transparent;
		color: var(--text-2);
		font-size: 14px;
		font-weight: 500;
		cursor: pointer;
	}
	h2 {
		display: flex;
		align-items: baseline;
		gap: 10px;
		margin: 0;
		font-size: var(--fs-h2);
		font-weight: 600;
		letter-spacing: -0.01em;
	}
	.count {
		font-size: 15px;
		font-weight: 500;
	}
	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(232px, 1fr));
		gap: 22px;
		padding: 16px 4px 8px;
	}
	.link {
		color: var(--text);
		text-decoration: underline;
	}
	.empty {
		margin: 40px 0;
		font-size: 17px;
		text-align: center;
	}
	@media (max-width: 720px) {
		.page {
			gap: 20px;
			padding-top: calc(var(--shell-h) + 24px);
		}
		.search {
			flex-basis: 100%;
		}
		kbd {
			display: none;
		}
		.chips {
			flex-wrap: nowrap;
			max-width: 100%;
			overflow-x: auto;
			scrollbar-width: none;
		}
		.chip {
			flex: none;
		}
		.grid {
			grid-template-columns: repeat(2, minmax(0, 1fr));
			gap: 12px;
			padding: 12px 0 4px;
		}
	}
</style>
