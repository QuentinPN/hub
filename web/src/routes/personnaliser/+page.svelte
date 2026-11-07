<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { ApiError, resetHomeLayout, saveHomeLayout } from '#lib/api/client.ts';
	import type { HomeLayout, LayoutRow } from '#lib/api/types.ts';
	import ErrorPanel from '#lib/components/ErrorPanel.svelte';
	import Skeleton from '#lib/components/Skeleton.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const ROWS: Record<string, { name: string; note: string; emptyChoices: string }> = {
		playing: {
			name: 'En lecture',
			note: 'Grand bandeau du haut',
			emptyChoices: 'Aucune connexion ne lit de sessions.'
		},
		favorites: {
			name: 'Favoris',
			note: 'Tes services étoilés',
			emptyChoices: 'Géré par les étoiles.'
		},
		latest: {
			name: 'Ajoutés récemment',
			note: 'Dernières entrées de la bibliothèque',
			emptyChoices: 'Aucune bibliothèque connectée.'
		},
		upcoming: {
			name: 'À venir',
			note: 'Calendrier commun',
			emptyChoices: 'Aucun calendrier connecté.'
		},
		downloads: {
			name: 'Téléchargements',
			note: 'Une carte par widget',
			emptyChoices: 'Aucune carte connectée.'
		},
		hosts: { name: 'Infrastructure', note: 'Hôtes et agents', emptyChoices: 'Aucun hôte connecté.' }
	};

	// La dernière réponse du serveur : chaque changement s'enregistre aussitôt.
	let fresh = $state<HomeLayout | null>(null);
	let error = $state('');
	let announce = $state('');
	let busy = $state(false);

	async function save(layout: HomeLayout, rows: LayoutRow[], message: string) {
		const previous = fresh;
		fresh = { ...layout, rows };
		error = '';
		busy = true;
		try {
			fresh = await saveHomeLayout(fetch, rows);
			announce = message;
		} catch (err) {
			fresh = previous;
			error = `Non enregistré : ${err instanceof ApiError ? err.message : String(err)}`;
		} finally {
			busy = false;
		}
	}

	function move(layout: HomeLayout, index: number, by: number) {
		const rows = [...layout.rows];
		const [row] = rows.splice(index, 1);
		if (!row) return;
		rows.splice(index + by, 0, row);
		void save(
			layout,
			rows,
			`${ROWS[row.id]?.name ?? row.id} déplacé en position ${index + by + 1}.`
		);
	}

	function setVisible(layout: HomeLayout, row: LayoutRow, visible: boolean) {
		const rows = layout.rows.map((r) => (r.id === row.id ? { ...r, visible } : r));
		void save(layout, rows, `${ROWS[row.id]?.name ?? row.id} ${visible ? 'visible' : 'masqué'}.`);
	}

	function toggle(layout: HomeLayout, row: LayoutRow, source: string, label: string) {
		const out = row.excluded.includes(source);
		const excluded = out ? row.excluded.filter((s) => s !== source) : [...row.excluded, source];
		const rows = layout.rows.map((r) => (r.id === row.id ? { ...r, excluded } : r));
		void save(layout, rows, `${label} ${out ? 'affiché' : 'retiré'}.`);
	}

	async function reset() {
		error = '';
		busy = true;
		try {
			fresh = await resetHomeLayout(fetch);
			announce = 'Ordre rétabli.';
		} catch (err) {
			error = `Non rétabli : ${err instanceof ApiError ? err.message : String(err)}`;
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Personnaliser l’accueil · Hub</title></svelte:head>

<div class="page">
	<header>
		<div>
			<h1>Personnaliser l’accueil</h1>
			<p class="muted">
				Enregistré à chaque changement. Un nouveau scan ne touche pas à cet ordre.
			</p>
		</div>
		<div class="actions">
			<button type="button" class="btn" disabled={busy} onclick={reset}>Rétablir l’ordre</button>
			<a class="btn primary" href={resolve('/')}>Terminé</a>
		</div>
	</header>

	<p class="error" role="alert">{error}</p>
	<p class="visually-hidden" aria-live="polite">{announce}</p>

	{#await data.layout}
		<div class="rows" aria-busy="true">
			<Skeleton height="80px" radius="18px" />
			<Skeleton height="80px" radius="18px" />
			<Skeleton height="80px" radius="18px" />
		</div>
	{:then loaded}
		{@const layout = fresh ?? loaded}
		<ol class="rows">
			{#each layout.rows as row, i (row.id)}
				{@const info = ROWS[row.id]}
				{@const choices = layout.choices[row.id] ?? []}
				<li class:off={!row.visible}>
					<span class="what">
						<strong>{info?.name ?? row.id}</strong>
						<span class="muted small">{info?.note}</span>
					</span>
					<span class="chips">
						{#each choices as c (c.id)}
							{@const on = !row.excluded.includes(c.id)}
							<button
								type="button"
								class="chip"
								class:on
								aria-pressed={on}
								onclick={() => toggle(layout, row, c.id, c.label)}>{c.label}</button
							>
						{:else}
							<span class="muted small">{info?.emptyChoices}</span>
						{/each}
					</span>
					<span class="moves">
						{#if row.id !== 'playing'}
							<button
								type="button"
								class="move"
								aria-label="Monter {info?.name}"
								disabled={i <= 1}
								onclick={() => move(layout, i, -1)}>↑</button
							>
							<button
								type="button"
								class="move"
								aria-label="Descendre {info?.name}"
								disabled={i === layout.rows.length - 1}
								onclick={() => move(layout, i, 1)}>↓</button
							>
						{:else}
							<span class="muted small fixed">Toujours en haut</span>
						{/if}
					</span>
					<label class="visible">
						<input
							type="checkbox"
							checked={row.visible}
							onchange={(e) => setVisible(layout, row, e.currentTarget.checked)}
						/>
						Visible<span class="visually-hidden"> : {info?.name}</span>
					</label>
				</li>
			{/each}
		</ol>
	{:catch err}
		<ErrorPanel
			title="Impossible de charger la disposition"
			detail={err instanceof Error ? err.message : String(err)}
			onretry={() => invalidateAll()}
		/>
	{/await}
</div>

<style>
	.page {
		display: flex;
		flex-direction: column;
		gap: 20px;
		padding: calc(var(--shell-h) + 36px) var(--space-page) 64px;
	}
	header {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 16px 24px;
	}
	h1 {
		margin: 0;
		font-size: clamp(2rem, 4vw, 2.5rem);
		font-weight: 700;
		letter-spacing: -0.02em;
	}
	header p {
		margin: 4px 0 0;
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: 10px;
	}
	.btn {
		display: inline-flex;
		align-items: center;
		min-height: 48px;
		padding: 0 18px;
		border: 1px solid var(--border-2);
		border-radius: var(--r-button);
		background: var(--surface-2);
		color: var(--text);
		font-size: 15px;
		font-weight: 600;
		cursor: pointer;
	}
	.primary {
		border-color: transparent;
		background: var(--text);
		color: var(--text-inverse);
	}
	.btn:disabled {
		opacity: 0.6;
		cursor: default;
	}
	.error {
		margin: 0;
		color: #ff8a80;
		font-size: 14px;
	}
	.error:empty {
		display: none;
	}
	.rows {
		display: flex;
		flex-direction: column;
		gap: 12px;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	li {
		display: grid;
		grid-template-columns: 200px minmax(0, 1fr) 100px 90px;
		align-items: center;
		gap: 16px;
		padding: 12px 18px;
		border: 1px solid var(--border-1);
		border-radius: 18px;
		background: var(--surface-1);
	}
	.off {
		background: rgba(255, 255, 255, 0.02);
	}
	.off .what strong {
		color: var(--text-2);
	}
	.what {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.what strong {
		font-size: 16px;
		font-weight: 600;
	}
	.small {
		font-size: 13px;
	}
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
		min-width: 0;
	}
	.chip {
		min-height: 44px;
		padding: 0 14px;
		border: 1px solid var(--border-2);
		border-radius: var(--r-pill);
		background: transparent;
		color: var(--text-2);
		font-size: 14px;
		font-weight: 500;
		cursor: pointer;
	}
	.chip.on {
		border-color: rgba(232, 201, 166, 0.4);
		background: rgba(232, 201, 166, 0.12);
		color: var(--accent-play);
	}
	.moves {
		display: flex;
		gap: 6px;
	}
	.move {
		width: 44px;
		height: 44px;
		border: 1px solid var(--border-2);
		border-radius: 12px;
		background: var(--surface-2);
		color: var(--text);
		font-size: 16px;
		cursor: pointer;
	}
	.move:disabled {
		opacity: 0.35;
		cursor: default;
	}
	.visible {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		gap: 8px;
		min-height: 44px;
		font-size: 14px;
	}
	.visible input {
		width: 20px;
		height: 20px;
		accent-color: var(--accent-play);
	}
	@media (max-width: 860px) {
		li {
			grid-template-columns: minmax(0, 1fr) auto;
			gap: 10px 12px;
			padding: 14px;
		}
		.chips {
			grid-column: 1 / -1;
			grid-row: 2;
		}
		.moves {
			grid-row: 3;
		}
		.visible {
			grid-row: 3;
		}
	}
</style>
